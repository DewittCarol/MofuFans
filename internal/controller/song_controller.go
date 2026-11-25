package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type SongController struct{}

// Both bare BV IDs and full Bilibili video links are accepted. Store only the
// canonical ID, never arbitrary user-provided URLs for the public link.
var songBV = regexp.MustCompile(`^(?i:BV)1[0-9A-Za-z]{9}$`)

func normalizeSongVideo(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	page := ""
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Host != "www.bilibili.com" && u.Host != "bilibili.com" && u.Host != "m.bilibili.com") {
			return "", errors.New("请填写 BV 号或歌切的 B 站链接")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "video" {
			return "", errors.New("链接中没有有效的 BV 号")
		}
		value = parts[1]
		page = u.Query().Get("p")
	} else if parts := strings.SplitN(value, "?", 2); len(parts) == 2 {
		value = parts[0]
		q, err := url.ParseQuery(parts[1])
		if err != nil || len(q) != 1 {
			return "", errors.New("分 P 参数无效")
		}
		page = q.Get("p")
	}
	if !songBV.MatchString(value) {
		return "", errors.New("BV 号格式不正确")
	}
	value = "BV" + value[2:]
	if page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 || n > 9999 {
			return "", errors.New("视频分 P 需为 1–9999")
		}
		if n > 1 {
			value += "?p=" + strconv.Itoa(n)
		}
	}
	return value, nil
}

func libraryRequest(c *gin.Context, maxBytes int64) bool {
	c.Header("Cache-Control", "private, no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if origin := c.GetHeader("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != c.Request.Host || (u.Scheme != "http" && u.Scheme != "https") {
			Fail(c, 403, "请从本站重新打开页面")
			return false
		}
	}
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		Fail(c, 403, "请从本站重新打开页面")
		return false
	}
	return true
}
func libraryPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	if page < 1 || page > 1000000 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 24
	}
	return page, size
}
func libraryID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "记录编号无效")
		return 0, false
	}
	return uint(id), true
}
func (*SongController) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, size := libraryPage(c)
	query := database.DB.Model(&database.Song{})
	for _, kind := range []string{"language", "genre"} {
		if raw := c.Query(kind + "_id"); raw != "" {
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || id == 0 {
				Fail(c, 400, "标签筛选无效")
				return
			}
			query = query.Where("EXISTS (SELECT 1 FROM song_tag_links AS links JOIN song_tags AS tags ON tags.id = links.song_tag_id WHERE links.song_id = songs.id AND tags.id = ? AND tags.kind = ?)", id, kind)
		}
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		query = query.Where("title LIKE ? OR artist LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	var total, performed int64
	if query.Session(&gorm.Session{}).Count(&total).Error != nil || query.Session(&gorm.Session{}).Where("performed = ?", true).Count(&performed).Error != nil {
		Fail(c, 500, "歌单加载失败，请重试")
		return
	}
	count := total
	if c.Query("performed") == "true" {
		query = query.Where("performed = ?", true)
		count = performed
	}
	rows := []database.Song{}
	if query.Preload("Tags", func(tx *gorm.DB) *gorm.DB { return tx.Order("id asc") }).Order("id desc").Offset((page-1)*size).Limit(size).Find(&rows).Error != nil {
		Fail(c, 500, "歌单加载失败，请重试")
		return
	}
	updated, err := libraryLastUpdated(database.DB.Model(&database.Song{}))
	if err != nil {
		Fail(c, 500, "歌单加载失败，请重试")
		return
	}
	Success(c, gin.H{"items": rows, "total": count, "page": page, "page_size": size, "last_updated_at": updated, "counts": gin.H{"all": total, "performed": performed}})
}
func (*SongController) Save(c *gin.Context) {
	if !libraryRequest(c, 32<<10) {
		return
	}
	var input struct {
		Title     string   `json:"title"`
		Artist    string   `json:"artist"`
		Performed bool     `json:"performed"`
		Videos    []string `json:"videos"`
		TagIDs    *[]uint  `json:"tag_ids"`
	}
	if c.ShouldBindJSON(&input) != nil {
		Fail(c, 400, "歌曲格式无效")
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Artist = strings.TrimSpace(input.Artist)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 100 || utf8.RuneCountInString(input.Artist) > 100 || len(input.Videos) > 20 {
		Fail(c, 400, "请填写歌名，歌名与歌手最多 100 字，最多关联 20 个视频")
		return
	}
	videos := []string{}
	seen := map[string]bool{}
	for _, raw := range input.Videos {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		bv, err := normalizeSongVideo(raw)
		if err != nil {
			Fail(c, 400, err.Error())
			return
		}
		if !seen[bv] {
			videos = append(videos, bv)
			seen[bv] = true
		}
	}
	row := database.Song{Title: input.Title, Artist: input.Artist, Performed: input.Performed || len(videos) > 0, Videos: videos}
	tagIDs := []uint{}
	if input.TagIDs != nil {
		if len(*input.TagIDs) > 24 {
			Fail(c, 400, "每首歌最多选择 24 个标签")
			return
		}
		seenTags := map[uint]bool{}
		for _, id := range *input.TagIDs {
			if !seenTags[id] {
				tagIDs = append(tagIDs, id)
				seenTags[id] = true
			}
		}
		if len(tagIDs) > 0 {
			var count int64
			if database.DB.Model(&database.SongTag{}).Where("id IN ?", tagIDs).Count(&count).Error != nil {
				Fail(c, 500, "标签读取失败")
				return
			}
			if count != int64(len(tagIDs)) {
				Fail(c, 400, "所选标签不存在，请刷新后重试")
				return
			}
		}
	}
	if c.Param("id") != "" {
		id, ok := libraryID(c)
		if !ok {
			return
		}
		var existing database.Song
		if err := database.DB.First(&existing, id).Error; err != nil {
			Fail(c, 404, "歌曲不存在，请刷新列表")
			return
		}
		row.ID = id
		row.CreatedAt = existing.CreatedAt
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if row.ID == 0 {
			if err := tx.Omit("Tags").Create(&row).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&row).Select("title", "artist", "performed", "videos", "updated_at").Updates(&row).Error; err != nil {
				return err
			}
		}
		// Older clients omit tag_ids; retain their existing associations.
		if input.TagIDs != nil {
			if err := tx.Where("song_id = ?", row.ID).Delete(&database.SongTagLink{}).Error; err != nil {
				return err
			}
			links := []database.SongTagLink{}
			for _, id := range tagIDs {
				links = append(links, database.SongTagLink{SongID: row.ID, SongTagID: id})
			}
			if len(links) > 0 {
				if err := tx.Create(&links).Error; err != nil {
					return err
				}
			}
		}
		return tx.Preload("Tags", func(q *gorm.DB) *gorm.DB { return q.Order("id asc") }).First(&row, row.ID).Error
	})
	if err != nil {
		Fail(c, 500, "歌曲保存失败")
		return
	}
	Success(c, row)
}
func (*SongController) Delete(c *gin.Context) {
	if !libraryRequest(c, 1024) {
		return
	}
	id, ok := libraryID(c)
	if !ok {
		return
	}
	var affected int64
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("song_id = ?", id).Delete(&database.SongTagLink{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&database.Song{}, id)
		affected = result.RowsAffected
		return result.Error
	})
	if err != nil {
		Fail(c, 500, "删除失败")
		return
	}
	if affected == 0 {
		Fail(c, 404, "歌曲不存在")
		return
	}
	Success(c, nil)
}
