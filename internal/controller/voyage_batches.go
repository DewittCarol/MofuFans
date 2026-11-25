package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type VoyageBatchesController struct{}

var errBatchClosed = errors.New("本批次暂未开放或已结束地址收集，仍可查询已有地址与物流")

func batchFor(tx *gorm.DB, id uint) (database.VoyageBatch, bool, error) {
	var b database.VoyageBatch
	if !tx.Migrator().HasTable(&database.VoyageBatch{}) {
		return b, false, nil
	}
	e := tx.First(&b, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return b, false, nil
	}
	return b, e == nil, e
}

// UPDATE takes a transaction-scoped row lock on MySQL and a writer lock on
// SQLite. Call before business SELECTs so a waiter reads the preceding commit.
func lockVoyageBatchWrites(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&database.VoyageBatchSyncLock{}) {
		return nil
	}
	result := tx.Model(&database.VoyageBatchSyncLock{}).Where("id = ?", 1).Update("version", gorm.Expr("version + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("批次数据尚未初始化，请联系维护人员")
	}
	return nil
}
func batchCollecting(b database.VoyageBatch) bool {
	today := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	return b.Published && !b.Closed && (b.Opens == "" || today >= b.Opens) && (b.Closes == "" || today <= b.Closes)
}
func batchID(c *gin.Context) (uint, bool) {
	id, e := strconv.ParseUint(c.Param("id"), 10, 32)
	if e != nil || id == 0 {
		Fail(c, 400, "批次无效")
		return 0, false
	}
	return uint(id), true
}
func (*VoyageBatchesController) List(c *gin.Context) {
	rows := []database.VoyageBatch{}
	if e := database.DB.Order("id desc").Find(&rows).Error; e != nil {
		Fail(c, 500, "批次读取失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	Success(c, rows)
}
func (*VoyageBatchesController) Public(c *gin.Context) {
	var rows []database.VoyageBatch
	if e := database.DB.Where("published = ?", true).Order("id desc").Find(&rows).Error; e != nil {
		Fail(c, 500, "批次读取失败")
		return
	}
	result := []gin.H{}
	for _, b := range rows {
		result = append(result, gin.H{"id": b.ID, "title": b.Title, "note": b.Note, "opens": b.Opens, "closes": b.Closes, "closed": b.Closed, "collecting": batchCollecting(b)})
	}
	c.Header("Cache-Control", "no-store")
	Success(c, result)
}
func (*VoyageBatchesController) Save(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	var req struct {
		Title     string `json:"title"`
		Note      string `json:"note"`
		Published bool   `json:"published"`
		Closed    bool   `json:"closed"`
		Opens     string `json:"opens"`
		Closes    string `json:"closes"`
		Version   int    `json:"version"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "批次内容无效")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || utf8.RuneCountInString(req.Title) > 80 || utf8.RuneCountInString(req.Note) > 3000 {
		Fail(c, 400, "请填写批次名称（最多80字），说明最多3000字")
		return
	}
	for _, date := range []string{req.Opens, req.Closes} {
		if date != "" {
			if _, e := time.Parse("2006-01-02", date); e != nil {
				Fail(c, 400, "请选择有效日期")
				return
			}
		}
	}
	if req.Opens != "" && req.Closes != "" && req.Opens > req.Closes {
		Fail(c, 400, "截止日期不能早于开始日期")
		return
	}
	var b database.VoyageBatch
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if c.Param("id") == "" {
			if req.Published {
				return errors.New("请先建立批次并绑定名单，再开放给用户")
			}
			scope := database.VoyageIssue{Kind: "batch", Title: req.Title, Version: 1, Gifts: []database.VoyageGift{}, Images: []string{}}
			if e := tx.Create(&scope).Error; e != nil {
				return e
			}
			b = database.VoyageBatch{ID: scope.ID, Version: 1, Sources: []database.VoyageBatchSource{}}
		} else {
			id, e := strconv.ParseUint(c.Param("id"), 10, 32)
			if e != nil || id == 0 {
				return errors.New("批次无效")
			}
			if e = tx.First(&b, id).Error; e != nil {
				return e
			}
			if b.Version != req.Version {
				return errDeliveryConflict
			}
			b.Version++
		}
		if req.Published && !b.Published {
			rows, e := voyageRecipients(tx, b.ID)
			if e != nil {
				return e
			}
			n := 0
			for _, r := range rows {
				if !r.Disabled {
					n++
				}
			}
			if n == 0 {
				return errors.New("请先绑定名单并生成至少一位收件人")
			}
		}
		b.Title = req.Title
		b.Note = req.Note
		b.Published = req.Published
		b.Closed = req.Closed
		b.Opens = req.Opens
		b.Closes = req.Closes
		if e := tx.Save(&b).Error; e != nil {
			return e
		}
		if e := tx.Model(&database.VoyageIssue{}).Where("id = ?", b.ID).Updates(map[string]any{"title": b.Title, "published": b.Published, "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
		return inheritLongTermAddresses(tx, b.ID, "")
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, b)
}

type batchPreviewRow struct {
	UID    string                     `json:"uid"`
	Name   string                     `json:"name"`
	Items  []database.VoyageBatchItem `json:"items"`
	Change string                     `json:"change"`
}
type batchPreview struct {
	Token    string            `json:"token"`
	Rows     []batchPreviewRow `json:"rows"`
	Warnings []string          `json:"warnings"`
	Added    int               `json:"added"`
	Changed  int               `json:"changed"`
	Removed  int               `json:"removed"`
	Total    int               `json:"total"`
}

func batchPlan(tx *gorm.DB, b database.VoyageBatch, sources []database.VoyageBatchSource) (batchPreview, error) {
	p := batchPreview{Rows: []batchPreviewRow{}, Warnings: []string{}}
	if len(sources) > 60 {
		return p, errors.New("最多选择60张名单")
	}
	var shipped int64
	if e := tx.Model(&database.VoyageDelivery{}).Joins("JOIN voyage_recipients r ON r.id=voyage_deliveries.recipient_id").Where("r.issue_id = ? AND shipped_at IS NOT NULL", b.ID).Count(&shipped).Error; e != nil {
		return p, e
	}
	if shipped > 0 && !tx.Migrator().HasTable(&database.VoyageParcel{}) {
		return p, errors.New("本批次已有发货记录，名单范围已锁定；补发请新建批次")
	}
	var others []database.VoyageRecipient
	if e := tx.Where("issue_id <> ? AND disabled = ?", b.ID, false).Find(&others).Error; e != nil {
		return p, e
	}
	var batchRows []database.VoyageBatch
	if e := tx.Find(&batchRows).Error; e != nil {
		return p, e
	}
	batchMap := map[uint]database.VoyageBatch{}
	for _, v := range batchRows {
		batchMap[v.ID] = v
	}
	reserved := map[string]string{}
	reservedIdentity := map[string]string{}
	for _, other := range others {
		otherBatch, ok := batchMap[other.IssueID]
		if !ok {
			continue
		}
		for _, item := range other.BatchItems {
			reserved[other.UID+"/"+item.Key] = otherBatch.Title
			reservedIdentity[rosterItemIdentity(other.UID, item)] = otherBatch.Title
		}
	}
	parcels, e := activeParcels(tx)
	if e != nil {
		return p, e
	}
	for _, parcel := range parcels {
		for _, item := range parcel.Items {
			if item.CollectionID == b.ID {
				continue
			}
			title := batchMap[item.CollectionID].Title
			if title == "" {
				title = "已有发货安排"
			}
			reserved[entitlementKey(parcel.UID, item.Key)] = title
			reservedIdentity[rosterItemIdentity(parcel.UID, item.VoyageBatchItem)] = title
		}
	}
	candidate := map[string]*batchPreviewRow{}
	rosterVersions := map[uint]int{}
	seen := map[uint]bool{}
	for _, source := range sources {
		if seen[source.RosterID] {
			return p, errors.New("同一张名单只能绑定一次")
		}
		seen[source.RosterID] = true
		var roster database.VoyageRoster
		if e := tx.First(&roster, source.RosterID).Error; e != nil {
			return p, errors.New("所选名单已不存在")
		}
		rosterVersions[roster.ID] = roster.Version
		allowed := map[string]bool{}
		if roster.Category == "monthly" {
			if len(source.Tiers) == 0 {
				return p, errors.New("请为每张月度名单至少选择一个档位")
			}
			for _, tier := range source.Tiers {
				if voyageTier(tier) != tier || tier == "" || allowed[tier] {
					return p, errors.New("档位选择无效")
				}
				allowed[tier] = true
			}
		}
		var members []database.VoyageRosterMember
		if e := tx.Where("roster_id = ? AND disabled = ?", roster.ID, false).Order("id").Find(&members).Error; e != nil {
			return p, e
		}
		for _, m := range members {
			if roster.Category == "monthly" && m.Tier == "" {
				p.Warnings = append(p.Warnings, roster.Title+"：UID "+m.UID+" 未设置档位，暂不加入")
				continue
			}
			if roster.Category == "monthly" && !allowed[m.Tier] {
				continue
			}
			item := database.VoyageItem(roster, m)
			title, reservedKey := reserved[m.UID+"/"+item.Key]
			if logicalTitle, ok := reservedIdentity[rosterItemIdentity(m.UID, item)]; ok {
				title = logicalTitle
				reservedKey = true
			}
			if reservedKey {
				p.Warnings = append(p.Warnings, "UID "+m.UID+" 的「"+item.Label+"」已在「"+title+"」安排，未重复加入")
				continue
			}
			row := candidate[m.UID]
			if row == nil {
				row = &batchPreviewRow{UID: m.UID, Name: m.Name, Items: []database.VoyageBatchItem{}, Change: "new"}
				candidate[m.UID] = row
			}
			row.Items = append(row.Items, item)
			if m.Name != "" {
				row.Name = m.Name
			}
		}
	}
	var current []database.VoyageRecipient
	if e := tx.Where("issue_id = ?", b.ID).Order("id").Find(&current).Error; e != nil {
		return p, e
	}
	for _, parcel := range parcels {
		for _, item := range parcel.Items {
			if item.CollectionID != b.ID {
				continue
			}
			row := candidate[parcel.UID]
			if row == nil {
				row = &batchPreviewRow{UID: parcel.UID, Name: parcel.Name, Items: []database.VoyageBatchItem{}, Change: "new"}
				candidate[parcel.UID] = row
			}
			found := false
			for n, i := range row.Items {
				if i.Key == item.Key || rosterItemIdentity(parcel.UID, i) == rosterItemIdentity(parcel.UID, item.VoyageBatchItem) {
					row.Items[n] = item.VoyageBatchItem
					found = true
				}
			}
			if !found {
				row.Items = append(row.Items, item.VoyageBatchItem)
				p.Warnings = append(p.Warnings, "UID "+parcel.UID+" 的「"+item.Label+"」已有发货安排，本次保留；未寄出的包裹可先在发货管理取消")
			}
		}
	}
	for _, r := range current {
		row := candidate[r.UID]
		if row == nil {
			if len(r.BatchItems) > 0 {
				p.Rows = append(p.Rows, batchPreviewRow{UID: r.UID, Name: r.Name, Items: []database.VoyageBatchItem{}, Change: "removed"})
				p.Removed++
			}
			continue
		}
		left, _ := json.Marshal(r.BatchItems)
		right, _ := json.Marshal(row.Items)
		row.Change = "unchanged"
		if string(left) != string(right) || r.Name != row.Name {
			row.Change = "changed"
			p.Changed++
		}
	}
	for _, row := range candidate {
		if row.Change == "new" {
			p.Added++
		}
		p.Rows = append(p.Rows, *row)
		p.Total++
	}
	if p.Total > 10000 {
		return p, errors.New("单个批次最多10000位收件人")
	}
	sort.Slice(p.Rows, func(i, j int) bool { return p.Rows[i].UID < p.Rows[j].UID })
	raw, _ := json.Marshal(struct {
		Version int
		Sources []database.VoyageBatchSource
		Rosters map[uint]int
		Current []database.VoyageRecipient
		Preview batchPreview
	}{b.Version, sources, rosterVersions, current, p})
	p.Token = fmt.Sprintf("%x", sha256.Sum256(raw))
	return p, nil
}
func (*VoyageBatchesController) Sync(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	id, ok := batchID(c)
	if !ok {
		return
	}
	var req struct {
		Sources []database.VoyageBatchSource `json:"sources"`
		Version int                          `json:"version"`
		Token   string                       `json:"token"`
		Confirm bool                         `json:"confirm"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "绑定内容无效")
		return
	}
	var p batchPreview
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		var b database.VoyageBatch
		if e := tx.First(&b, id).Error; e != nil {
			return e
		}
		if b.Version != req.Version {
			return errDeliveryConflict
		}
		var e error
		p, e = batchPlan(tx, b, req.Sources)
		if e != nil {
			return e
		}
		if !req.Confirm {
			return nil
		}
		if req.Token == "" || req.Token != p.Token {
			return errors.New("名单或批次已变化，请重新预览后确认")
		}
		if b.Published && p.Total == 0 {
			return errors.New("开放中的批次不能清空收件人，请先关闭前台展示")
		}
		if e = applyBatchPreview(tx, b.ID, p); e != nil {
			return e
		}
		b.Sources = req.Sources
		b.Version++
		return tx.Save(&b).Error
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, p)
}

func applyBatchPreview(tx *gorm.DB, batchID uint, p batchPreview) error {
	for _, item := range p.Rows {
		var row database.VoyageRecipient
		err := tx.Where("issue_id = ? AND uid = ?", batchID, item.UID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sealed, hash, e := secret.NewVoyageCode(recipientScope(batchID, item.UID))
			if e != nil {
				return e
			}
			row = database.VoyageRecipient{IssueID: batchID, UID: item.UID, Name: item.Name, CodeEncrypted: sealed, CodeHash: hash, Version: 1, BatchItems: item.Items}
			if e = tx.Create(&row).Error; e != nil {
				return e
			}
			continue
		} else if err != nil {
			return err
		}
		if item.Change == "unchanged" {
			continue
		}
		wasRemoved := len(row.BatchItems) == 0
		row.BatchItems = item.Items
		row.Name = item.Name
		row.Version++
		row.SentAt = nil
		row.PreparedAt = nil
		if item.Change == "removed" {
			row.Disabled = true
		} else if wasRemoved {
			row.Disabled = false
		}
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
	}
	return inheritLongTermAddresses(tx, batchID, "")
}

func batchLabels(items []database.VoyageBatchItem) []string {
	labels := []string{}
	for _, i := range items {
		labels = append(labels, i.Label)
	}
	return labels
}
func batchDescription(row database.VoyageRecipient) string {
	return strings.Join(batchLabels(row.BatchItems), "、")
}
