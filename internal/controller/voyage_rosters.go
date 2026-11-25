package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type VoyageRostersController struct{}

var errRosterDuplicate = errors.New("这个 UID 已有相同档位的记录，请直接编辑已有记录")

func rosterEntryKey(uid, tier string) string { return uid + "/" + tier }

func (*VoyageRostersController) List(c *gin.Context) {
	rows := []database.VoyageRoster{}
	if e := database.DB.Order("month desc, id desc").Find(&rows).Error; e != nil {
		Fail(c, 500, "名单读取失败，请重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	Success(c, rows)
}
func rosterRequest(c *gin.Context) (database.VoyageRoster, bool) {
	var r database.VoyageRoster
	id, e := strconv.ParseUint(c.Param("roster"), 10, 32)
	if e != nil || id == 0 || database.DB.First(&r, id).Error != nil {
		Fail(c, 404, "名单不存在")
		return r, false
	}
	return r, true
}
func (*VoyageRostersController) Save(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	var req struct {
		Title    string `json:"title"`
		Category string `json:"category"`
		Month    string `json:"month"`
		Version  int    `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "内容无效")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || utf8.RuneCountInString(req.Title) > 80 || (req.Category != "monthly" && req.Category != "event") {
		Fail(c, 400, "请填写名单名称，并选择月度大航海或活动抽奖类别")
		return
	}
	if req.Category == "monthly" {
		if _, e := time.Parse("2006-01", req.Month); e != nil {
			Fail(c, 400, "请选择所属年份和月份")
			return
		}
	} else if req.Month != "" {
		if _, e := time.Parse("2006-01", req.Month); e != nil {
			Fail(c, 400, "月份格式无效")
			return
		}
	}
	row := database.VoyageRoster{Title: req.Title, Category: req.Category, Month: req.Month, Version: 1}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if c.Param("roster") != "" {
			id, e := strconv.ParseUint(c.Param("roster"), 10, 32)
			if e != nil || id == 0 {
				return errors.New("名单无效")
			}
			if e = tx.First(&row, id).Error; e != nil {
				return e
			}
			if row.Version != req.Version {
				return errDeliveryConflict
			}
			if row.Category != req.Category || row.Month != req.Month {
				return errors.New("名单类别和所属月份创建后固定；如需调整，请新建正确的名单")
			}
			row.Title = req.Title
			row.Version++
			if e = tx.Model(&row).Where("version = ?", req.Version).Select("Title", "Version").Updates(row).Error; e != nil {
				return e
			}
			_, e = syncVoyageRoster(tx, row)
			return e
		}
		if req.Category == "monthly" {
			var n int64
			if e := tx.Model(&database.VoyageRoster{}).Where("category = ? AND month = ?", "monthly", req.Month).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return errors.New("该月份已有大航海名单，请使用现有名单")
			}
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, row)
}
func (*VoyageRostersController) Members(c *gin.Context) {
	roster, ok := rosterRequest(c)
	if !ok {
		return
	}
	rows := []database.VoyageRosterMember{}
	if e := database.DB.Where("roster_id = ?", roster.ID).Order("id").Find(&rows).Error; e != nil {
		Fail(c, 500, "名单读取失败")
		return
	}
	Success(c, rows)
}

type rosterImportItem struct {
	Line   int    `json:"line"`
	UID    string `json:"uid"`
	Tier   string `json:"tier"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func parseRosterImport(text, category string) ([]rosterImportItem, error) {
	records, e := readVoyageImportRecords(strings.TrimSpace(strings.TrimPrefix(text, "\ufeff")))
	if e != nil {
		return nil, e
	}
	rows := []rosterImportItem{}
	uidCol, tierCol, nameCol := 0, 1, 2
	first := true
	seen := map[string]bool{}
	for _, record := range records {
		cells := record.Values
		if len(cells) == 0 {
			continue
		}
		if first && strings.HasPrefix(strings.TrimSpace(cells[0]), "舰队总数") {
			continue
		}
		if first {
			first = false
			header := false
			for i, v := range cells {
				k := strings.ToLower(strings.TrimSpace(v))
				if k == "uid" || k == "用户uid" || k == "b站uid" || k == "buid" {
					uidCol = i
					header = true
				}
			}
			if header {
				tierCol = -1
				nameCol = -1
				for i, v := range cells {
					switch strings.ToLower(strings.TrimSpace(v)) {
					case "档位", "等级", "大航海等级", "舰长类型", "tier", "guard_level":
						tierCol = i
					case "昵称", "用户名", "name", "username":
						nameCol = i
					}
				}
				continue
			}
			// BiliMessenger exports tier,UID,name,time without a header.
			if len(cells) > 1 && voyageTier(cells[0]) != "" {
				uidCol = 1
				tierCol = 0
				nameCol = 2
			} else if category == "event" {
				tierCol = -1
				nameCol = 1
			}
		}
		field := func(i int) string {
			if i >= 0 && i < len(cells) {
				return strings.TrimSpace(cells[i])
			}
			return ""
		}
		_, uid, e := normalizeBilibiliUID(field(uidCol))
		if e != nil || !claimUID.MatchString(uid) {
			return nil, errors.New("第 " + strconv.Itoa(record.Line) + " 行 UID 无效")
		}
		raw := field(tierCol)
		tier := voyageTier(raw)
		if category == "event" {
			tier = ""
		} else if raw != "" && tier == "" {
			return nil, errors.New("第 " + strconv.Itoa(record.Line) + " 行档位无效")
		}
		name := field(nameCol)
		if utf8.RuneCountInString(name) > 80 {
			return nil, errors.New("昵称最多 80 字")
		}
		status := "new"
		key := rosterEntryKey(uid, tier)
		if seen[key] {
			status = "duplicate"
		}
		seen[key] = true
		rows = append(rows, rosterImportItem{record.Line, uid, tier, name, status})
		if len(rows) > 2000 {
			return nil, errors.New("每次最多导入 2000 行")
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("请填写至少一个 UID")
	}
	return rows, nil
}
func (*VoyageRostersController) Import(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	roster, ok := rosterRequest(c)
	if !ok {
		return
	}
	var req struct {
		Text    string `json:"text"`
		Version int    `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "名单内容无效或过大")
		return
	}
	parsed, e := parseRosterImport(req.Text, roster.Category)
	if e != nil {
		Fail(c, 400, e.Error())
		return
	}
	commit := strings.HasSuffix(c.FullPath(), "/import")
	added, skipped := 0, 0
	var syncResult rosterSyncResult
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		var current database.VoyageRoster
		if e := tx.First(&current, roster.ID).Error; e != nil {
			return e
		}
		if current.Version != req.Version {
			return errDeliveryConflict
		}
		var saved []database.VoyageRosterMember
		if e := tx.Where("roster_id = ?", roster.ID).Find(&saved).Error; e != nil {
			return e
		}
		existing := map[string]bool{}
		for _, r := range saved {
			existing[rosterEntryKey(r.UID, r.Tier)] = true
		}
		for i := range parsed {
			r := &parsed[i]
			if r.Status == "duplicate" {
				skipped++
				continue
			}
			if existing[rosterEntryKey(r.UID, r.Tier)] {
				r.Status = "existing"
				skipped++
			} else {
				added++
			}
		}
		if !commit {
			return nil
		}
		for _, r := range parsed {
			if r.Status != "new" {
				continue
			}
			if e := tx.Create(&database.VoyageRosterMember{RosterID: roster.ID, UID: r.UID, Tier: r.Tier, Name: r.Name, Version: 1}).Error; e != nil {
				return e
			}
		}
		if e := tx.Model(&current).Update("version", current.Version+1).Error; e != nil {
			return e
		}
		var e error
		syncResult, e = syncVoyageRoster(tx, current)
		return e
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, gin.H{"rows": parsed, "added": added, "skipped": skipped, "conflicts": 0, "sync": syncResult})
}
func (*VoyageRostersController) Update(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	roster, ok := rosterRequest(c)
	if !ok {
		return
	}
	id, e := strconv.ParseUint(c.Param("member"), 10, 32)
	if e != nil || id == 0 {
		Fail(c, 400, "用户记录无效")
		return
	}
	var req struct {
		Version  int    `json:"version"`
		Tier     string `json:"tier"`
		Name     string `json:"name"`
		Disabled bool   `json:"disabled"`
	}
	if c.ShouldBindJSON(&req) != nil || utf8.RuneCountInString(req.Name) > 80 || (req.Tier != "" && voyageTier(req.Tier) == "") {
		Fail(c, 400, "请检查昵称和档位")
		return
	}
	var row database.VoyageRosterMember
	var syncResult rosterSyncResult
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if e := tx.First(&roster, roster.ID).Error; e != nil {
			return e
		}
		if e := tx.Where("id = ? AND roster_id = ?", id, roster.ID).First(&row).Error; e != nil {
			return e
		}
		if row.Version != req.Version {
			return errDeliveryConflict
		}
		row.Name = strings.TrimSpace(req.Name)
		row.Tier = voyageTier(req.Tier)
		if roster.Category == "event" {
			row.Tier = ""
		}
		var duplicate int64
		if e := tx.Model(&database.VoyageRosterMember{}).Where("roster_id = ? AND uid = ? AND tier = ? AND id <> ?", roster.ID, row.UID, row.Tier, row.ID).Count(&duplicate).Error; e != nil {
			return e
		}
		if duplicate > 0 {
			return errRosterDuplicate
		}
		row.Disabled = req.Disabled
		row.Version++
		if e := tx.Save(&row).Error; e != nil {
			return e
		}
		if e := tx.Model(&roster).Update("version", gorm.Expr("version + 1")).Error; e != nil {
			return e
		}
		var e error
		syncResult, e = syncVoyageRoster(tx, roster)
		return e
	})
	if err != nil {
		if errors.Is(err, errRosterDuplicate) {
			Fail(c, 409, err.Error())
		} else {
			Fail(c, 409, "名单已更新，请刷新后重试")
		}
		return
	}
	Success(c, struct {
		database.VoyageRosterMember
		Sync rosterSyncResult `json:"sync"`
	}{row, syncResult})
}

// Deletion is explicit and record-scoped: selecting one UID/tier never deletes
// the same UID's other tiers, months, addresses or shipped parcel snapshots.
func (*VoyageRostersController) DeleteMembers(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	roster, ok := rosterRequest(c)
	if !ok {
		return
	}
	var req struct {
		Version int `json:"version"`
		Items   []struct {
			ID      uint `json:"id"`
			Version int  `json:"version"`
		} `json:"items"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Items) == 0 || len(req.Items) > 10000 {
		Fail(c, 400, "请选择 1 至 10000 条名单记录")
		return
	}
	var syncResult rosterSyncResult
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		if err := tx.First(&roster, roster.ID).Error; err != nil {
			return err
		}
		if roster.Version != req.Version {
			return errDeliveryConflict
		}
		ids := make([]uint, 0, len(req.Items))
		versions := map[uint]int{}
		for _, item := range req.Items {
			if item.ID == 0 || item.Version < 1 {
				return errors.New("名单记录无效")
			}
			if _, exists := versions[item.ID]; exists {
				return errors.New("请勿重复选择同一条记录")
			}
			versions[item.ID] = item.Version
			ids = append(ids, item.ID)
		}
		var rows []database.VoyageRosterMember
		if err := tx.Where("roster_id = ? AND id IN ?", roster.ID, ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errors.New("部分记录已删除或不属于这张名单，请刷新后重试")
		}
		for _, row := range rows {
			if row.Version != versions[row.ID] {
				return errDeliveryConflict
			}
		}
		if err := tx.Where("roster_id = ? AND id IN ?", roster.ID, ids).Delete(&database.VoyageRosterMember{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&roster).Update("version", gorm.Expr("version + 1")).Error; err != nil {
			return err
		}
		var err error
		syncResult, err = syncVoyageRoster(tx, roster)
		return err
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, gin.H{"deleted": len(req.Items), "sync": syncResult})
}
