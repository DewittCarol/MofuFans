package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"io"
	"mofufans/internal/database"
	"mofufans/internal/moderation"
	"mofufans/internal/storage"
	"mofufans/pkg/util"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mofufans/internal/adminauth"
)

// 1 for emoji op
// 2 for archive op
// 3 for cursor op
const (
	SSEventEmoji = iota + 1
	SSEventArchive
	SSEventCursor
)

type SSEvent struct {
	Type int
	Data any
}

type EmojiRecords struct {
	CardID int            `json:"card_id"`
	Emojis []*EmojiRecord `json:"emojis"`
}

type QuestionController struct {
	eventChan    chan SSEvent
	clients      map[chan SSEvent]bool
	clientsMutex sync.Mutex
	// WebSocket clients
	wsClients      map[*websocket.Conn]*wsPeer
	wsClientsMutex sync.Mutex
	cursorLimit    *rate.Limiter
}

func NewQuestionController() *QuestionController {
	controller := &QuestionController{
		eventChan:   make(chan SSEvent, 256),
		clients:     make(map[chan SSEvent]bool),
		wsClients:   make(map[*websocket.Conn]*wsPeer),
		cursorLimit: rate.NewLimiter(30, 60),
	}
	// Start broadcast goroutine
	go controller.broadcast()
	return controller
}

func (this *QuestionController) broadcast() {
	for event := range this.eventChan {
		// Broadcast to SSE clients
		this.clientsMutex.Lock()
		for client := range this.clients {
			select {
			case client <- event:
			default:
				// Remove client if channel is full
				delete(this.clients, client)
				close(client)
			}
		}
		this.clientsMutex.Unlock()

		// Broadcast to WebSocket clients
		this.wsClientsMutex.Lock()
		eventJson, err := json.Marshal(event)
		if err != nil {
			log.Error("Failed to marshal event for WebSocket:", err)
			this.wsClientsMutex.Unlock()
			continue
		}
		for _, peer := range this.wsClients {
			if len(eventJson) <= 64<<10 {
				peer.offer(eventJson)
			}
		}
		this.wsClientsMutex.Unlock()
	}
}

type QuestionRequest struct {
	LocateID         uint   `form:"locate_id"`
	OrderBy          string `form:"order_by"`
	Order            string `form:"order"`
	Page             int    `form:"page"`
	PageSize         int    `form:"page_size"`
	TagID            int    `form:"tag_id"`
	Search           string `form:"search"`
	Hide             bool   `form:"hide"`
	Rainbow          bool   `form:"rainbow"`
	Archive          bool   `form:"archive"`
	Publish          bool   `form:"publish"`
	Spam             bool   `form:"is_spam"`
	ModerationStatus string `form:"moderation_status"`
}

type QuestionModifyRequest struct {
	TagID     int   `json:"tag_id"`
	IsHide    bool  `json:"is_hide"`
	IsRainbow bool  `json:"is_rainbow"`
	IsArchive bool  `json:"is_archive"`
	IsPublish bool  `json:"is_publish"`
	IsSpam    *bool `json:"is_spam"`
}

// Get questions
func (*QuestionController) Get(c *gin.Context) {
	// Prepare for auth
	if _, ok := adminauth.Current(c); ok {
		c.Set("authed", true)
	}
	var questionList []database.Question
	var total int64
	var request QuestionRequest
	if err := c.Bind(&request); err != nil {
		Fail(c, 400, "请求错误")
		return
	}
	if _, locating := c.GetQuery("locate_id"); locating && request.LocateID == 0 {
		Fail(c, 400, "请输入有效的投稿编号")
		return
	}
	filter := func(tx *gorm.DB) *gorm.DB {
		if c.GetBool("authed") && request.ModerationStatus != "" {
			if request.ModerationStatus == "unreviewed" {
				tx = tx.Where("NOT EXISTS (SELECT 1 FROM question_moderations m WHERE m.question_id=questions.id AND m.status <> ?)", "skipped")
			} else {
				tx = tx.Where("EXISTS (SELECT 1 FROM question_moderations m WHERE m.question_id=questions.id AND m.status = ?)", request.ModerationStatus)
			}
		}
		if _, ok := c.GetQuery("tag_id"); ok {
			if request.TagID > 0 {
				tx = tx.Where("tag_id = ?", request.TagID)
			}
		}
		if _, ok := c.GetQuery("search"); ok {
			tx = tx.Where("content like ?", "%"+request.Search+"%")
		}
		if _, ok := c.GetQuery("hide"); ok {
			tx = tx.Where("is_hide = ?", request.Hide)
		}
		if _, ok := c.GetQuery("rainbow"); ok {
			tx = tx.Where("is_rainbow = ?", request.Rainbow)
		}
		if _, ok := c.GetQuery("archive"); ok {
			tx = tx.Where("is_archive = ?", request.Archive)
		}
		if _, ok := c.GetQuery("is_spam"); ok {
			tx = tx.Where("is_spam = ?", request.Spam)
		}
		if _, ok := c.GetQuery("publish"); ok {
			// Normal users can only see published questions
			log.Debug("publish: ", request.Publish)
			if !c.GetBool("authed") {
				tx = tx.Where("is_publish = ?", true)
			} else {
				tx = tx.Where("is_publish = ?", request.Publish)
			}
		} else {
			// Normal users can only see published questions
			if !c.GetBool("authed") {
				tx = tx.Where("is_publish = ?", true)
			}
		}
		return tx
	}
	page, size := getPage(request.Page), getPageSize(request.PageSize)
	var bounds struct {
		MinID uint
		MaxID uint
	}
	// Use one read snapshot for the target rank, counts and page contents.
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(snapshot *gorm.DB) error {
		base := filter(snapshot.Model(&database.Question{}))
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		if err := base.Session(&gorm.Session{}).Select("COALESCE(MIN(id), 0) AS min_id, COALESCE(MAX(id), 0) AS max_id").Scan(&bounds).Error; err != nil {
			return err
		}
		orderBy, order := getOrderBy(request.OrderBy), getOrder(request.Order)
		if request.LocateID > 0 {
			var target database.Question
			if err := base.Session(&gorm.Session{}).Where("id = ?", request.LocateID).Take(&target).Error; err != nil {
				return err
			}
			condition, values := questionPreceding(orderBy, order, target)
			var preceding int64
			if err := base.Session(&gorm.Session{}).Where(condition, values...).Count(&preceding).Error; err != nil {
				return err
			}
			page = int(preceding)/size + 1
		}
		rows := base.Session(&gorm.Session{}).Order(orderBy + " " + order)
		if orderBy != "is_archive" {
			rows = rows.Order("is_archive asc")
		}
		if orderBy != "id" {
			rows = rows.Order("id " + order)
		}
		return rows.Preload(clause.Associations).Scopes(paginate(page, size)).Find(&questionList).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, 404, "当前列表里没有这条投稿")
			return
		}
		log.Error(err)
		Fail(c, 500, "获取提问失败")
		return
	}
	if c.GetBool("authed") {
		c.Header("Cache-Control", "no-store")
		attachModeration(database.DB, questionList)
	}
	Success(c, gin.H{
		"questions": questionList, "total": total, "page": page, "page_size": size,
		"min_id": bounds.MinID, "max_id": bounds.MaxID,
	})
}

func (this *QuestionController) Put(c *gin.Context) {
	var request QuestionModifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求错误")
		return
	}
	var q database.Question
	database.DB.First(&q, "id = ?", c.Param("id"))
	if q.ID == 0 {
		Fail(c, 404, "提问不存在")
		return
	}
	if request.TagID == 0 {
		Fail(c, 400, "请求错误")
		return
	}
	wasArchived := q.IsArchive
	q.TagID = request.TagID
	q.IsHide = request.IsHide
	q.IsRainbow = request.IsRainbow
	q.IsArchive = request.IsArchive
	q.IsPublish = request.IsPublish
	if request.IsSpam != nil {
		q.IsSpam = *request.IsSpam
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&q).Error; err != nil {
			return err
		}
		return moderation.MarkManual(tx, q.ID)
	})
	if err != nil {
		log.Error(err)
		Fail(c, 500, "修改提问失败")
		return
	}
	// This event means "newly archived" to existing frontend clients. Other
	// moderation edits must not make a live card appear archived.
	if !wasArchived && q.IsArchive {
		this.eventChan <- SSEvent{
			Type: SSEventArchive,
			Data: q.ID,
		}
	}
	Success(c, nil)
}

func (*QuestionController) Post(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		Fail(c, 400, "投稿格式无效或附件总大小超过 32 MB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	var config database.Config
	if err := database.DB.First(&config).Error; err != nil {
		Fail(c, 500, "内部错误")
		return
	}
	member, memberLoggedIn := currentMember(c)
	if config.DisableMemberAccounts {
		memberLoggedIn = false
	}
	if !config.DisableMemberAccounts && config.RequireVerifiedUserToPost && !memberLoggedIn {
		Fail(c, 403, "投稿需要先完成 B 站验证并登录")
		return
	}
	realName := !config.DisableMemberAccounts && c.PostForm("real_name") == "true"
	if realName && !memberLoggedIn {
		Fail(c, 403, "实名投稿需要先完成 B 站验证并登录")
		return
	}
	var tag database.Tag
	tagID, err := strconv.ParseUint(c.PostForm("tag_id"), 10, 32)
	if err != nil || tagID == 0 {
		Fail(c, 400, "请选择一个话题")
		return
	}
	database.DB.First(&tag, tagID)
	if tag.ID == 0 {
		Fail(c, 404, "话题不存在")
		return
	}
	var q database.Question
	if memberLoggedIn {
		q.BilibiliUID = &member.BilibiliUID
	}
	if realName {
		q.IsRealName = true
		q.BilibiliName = member.BilibiliName
		q.BilibiliAvatar = member.BilibiliAvatar
	}
	q.TagID = int(tag.ID)
	q.Content = strings.Trim(c.PostForm("content"), " \r\n\t")
	if q.Content == "" {
		Fail(c, 400, "请填写提问内容")
		return
	}
	q.IsHide = c.PostForm("hide") == "true"
	q.IsRainbow = c.PostForm("rainbow") == "true"
	mp, _ := c.MultipartForm()
	if len(mp.File["files[]"]) > 6 {
		Fail(c, 400, "最多上传 6 张图片")
		return
	}
	for _, v := range mp.File["files[]"] {
		f, err := v.Open()
		if err != nil {
			log.Error(err)
			Fail(c, 405, "文件上传失败")
			return
		}
		fileContent, err := io.ReadAll(io.LimitReader(f, (10<<20)+1))
		f.Close()
		if err != nil {
			log.Error(err)
			Fail(c, 405, "文件上传失败")
			return
		}
		info, format, decodeErr := image.DecodeConfig(bytes.NewReader(fileContent))
		if decodeErr != nil || len(fileContent) > 10<<20 || info.Width > 16384 || info.Height > 16384 || int64(info.Width)*int64(info.Height) > 40000000 {
			Fail(c, 400, "请选择有效图片，每张不超过 10 MB、4000 万像素")
			return
		}
		if format == "jpeg" {
			format = "jpg"
		}
		newFileName := util.Md5v(string(fileContent)) + "." + format
		url, err := storage.Get().Upload(newFileName, bytes.NewReader(fileContent))
		if err != nil {
			log.Error(err)
			Fail(c, 500, "文件上传失败")
			return
		}
		q.ImagesNum++
		if q.Images == "" {
			q.Images = url
		} else {
			q.Images += ";" + url
		}
	}
	err = database.DB.Save(&q).Error
	if err != nil {
		log.Error(err)
		Fail(c, 500, "创建提问失败")
		return
	}
	if _, err := moderation.Queue(database.DB, q.ID, false); err != nil {
		log.Warn("AI queue unavailable; submission kept for manual review")
	}
	Success(c, nil)
}

func (*QuestionController) Delete(c *gin.Context) {
	var q database.Question
	id := c.Param("id")
	err := database.DB.First(&q, "id = ?", id).Error
	if q.ID == 0 {
		log.Error(err)
		Fail(c, 404, "提问不存在")
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("question_id = ?", q.ID).Delete(&database.QuestionModeration{}).Error; err != nil {
			return err
		}
		if err := tx.Where("question_id = ?", q.ID).Delete(&database.LikeRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(&q).Error
	})
	if err != nil {
		log.Error(err)
		Fail(c, 500, "删除提问失败")
		return
	}
	// clean images in storage
	key := "upload-img/"
	images := strings.Split(q.Images, ";")
	filenames := []string{}
	for i := range images {
		cur := images[i]
		parts := strings.SplitN(cur, key, 2)
		if len(parts) < 2 {
			continue
		}
		filenames = append(filenames, parts[1])
	}
	// it's ok to fail deleting
	for _, f := range filenames {
		_ = storage.Get().Delete(f)
	}
	Success(c, nil)
}

type EmojiRecord struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func (this *QuestionController) Emoji(c *gin.Context) {
	emojiToAdd := c.PostForm("emoji")
	// should check emojiToAdd valid or not
	var activeEmojis int64
	if err := database.DB.Model(&database.EmojiAsset{}).Where("kind = ? AND value = ? AND is_deleted = ?", "reaction", emojiToAdd, false).Count(&activeEmojis).Error; err != nil {
		Fail(c, 500, "表情加载失败，请重试")
		return
	}
	if activeEmojis == 0 {
		Fail(c, 400, "这个表情已移除，请重新选择")
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	tx := database.DB.Begin()
	var q database.Question
	err := tx.Where("id = ?", id).First(&q).Error
	if err != nil {
		log.Error(err)
		Fail(c, 500, "评价失败")
		tx.Rollback()
		return
	}
	var reactionPolicy database.Config
	if err := tx.Select("allow_repeated_reactions").Limit(1).Find(&reactionPolicy).Error; err != nil {
		tx.Rollback()
		Fail(c, 500, "互动设置加载失败，请重试")
		return
	}
	// Keep one record even in repeat mode. Turning the setting off restores
	// one reaction per IP/question without discarding any existing counts.
	likeRecord := database.LikeRecord{IP: c.ClientIP(), QuestionID: id}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&likeRecord)
	if result.Error != nil {
		log.Error(result.Error)
		Fail(c, 500, "评价失败")
		tx.Rollback()
		return
	}
	if result.RowsAffected == 0 && !reactionPolicy.AllowRepeatedReactions {
		tx.Rollback()
		Fail(c, 400, "您已经评价过了")
		return
	}
	var emojis []*EmojiRecord
	if len(q.Emojis) > 0 {
		err = json.Unmarshal([]byte(q.Emojis), &emojis)
		if err != nil {
			log.Error((err))
			Fail(c, 500, "解析表情失败")
			tx.Rollback()
			return
		}
	}
	added := false
	for _, e := range emojis {
		if e.Value == emojiToAdd {
			added = true
			e.Count++
			break
		}
	}
	if !added {
		emojis = append(emojis, &EmojiRecord{
			Value: emojiToAdd,
			Count: 1,
		})
	}
	updatedList, err := json.Marshal(emojis)
	if err != nil {
		log.Error(err)
		Fail(c, 500, "评价失败")
		tx.Rollback()
		return
	}
	q.Emojis = string(updatedList)
	err = tx.Save(&q).Error
	if err != nil {
		log.Error(err)
		Fail(c, 500, "评价失败")
		tx.Rollback()
		return
	}
	if tx.Commit().Error != nil {
		log.Error(err)
		Fail(c, 500, "评价失败")
		tx.Rollback()
		return
	}
	this.eventChan <- SSEvent{
		Type: SSEventEmoji,
		Data: EmojiRecords{
			CardID: id,
			Emojis: emojis,
		},
	}
	Success(c, EmojiRecords{CardID: id, Emojis: emojis})
}
