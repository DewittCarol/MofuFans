package controller

import (
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"strconv"
	"time"
)

type messageQueueSummary struct {
	ID        uint             `json:"id"`
	AdminID   uint             `json:"admin_id"`
	Owner     string           `json:"owner"`
	Scope     string           `json:"scope"`
	ScopeID   uint             `json:"scope_id"`
	Title     string           `json:"title"`
	SenderUID string           `json:"sender_uid"`
	Status    string           `json:"status"`
	Note      string           `json:"note"`
	Interval  int              `json:"interval"`
	UpdatedAt time.Time        `json:"updated_at"`
	Counts    map[string]int64 `json:"counts" gorm:"-"`
	Total     int64            `json:"total" gorm:"-"`
	CanManage bool             `json:"can_manage" gorm:"-"`
}

// All authenticated site administrators can monitor delivery. Only the existing
// owner-scoped action endpoints can change a queue or access message contents.
func (ctl *BilibiliMessagesController) Monitor(c *gin.Context) {
	admin := queueOwner(c)
	if admin == 0 {
		return
	}
	scope := c.Query("scope")
	scopeID, e := strconv.ParseUint(c.Query("scope_id"), 10, 32)
	if e != nil || scopeID == 0 || (scope != "voyage" && scope != "gift") {
		Fail(c, 400, "请选择地址收集批次或电子礼物")
		return
	}
	var queues []messageQueueSummary
	query := database.DB.Table("bilibili_message_queues AS q").Select("q.id,q.admin_id,COALESCE(a.username,'') AS owner,q.scope,q.scope_id,CASE WHEN q.scope='voyage' THEN COALESCE(v.title,'') ELSE COALESCE(g.title,'') END AS title,q.sender_uid,q.status,q.note,q.interval,q.updated_at").Joins("LEFT JOIN admins AS a ON a.id=q.admin_id").Joins("LEFT JOIN voyage_issues AS v ON q.scope='voyage' AND v.id=q.scope_id").Joins("LEFT JOIN digital_gifts AS g ON q.scope='gift' AND g.id=q.scope_id").Where("q.scope=? AND q.scope_id=?", scope, scopeID).Order("CASE WHEN q.status IN ('running','paused','testing','prepared','awaiting_confirmation') THEN 0 ELSE 1 END,q.id DESC").Limit(30)
	if query.Scan(&queues).Error != nil {
		Fail(c, 500, "发送队列读取失败，请刷新重试")
		return
	}
	if queues == nil {
		queues = []messageQueueSummary{}
	}
	ids := []uint{}
	byID := map[uint]int{}
	for i := range queues {
		ids = append(ids, queues[i].ID)
		byID[queues[i].ID] = i
		queues[i].Counts = map[string]int64{}
		queues[i].CanManage = queues[i].AdminID == admin
	}
	if len(ids) > 0 {
		var counts []struct {
			QueueID uint
			Status  string
			Count   int64
		}
		if database.DB.Model(&database.BilibiliMessageItem{}).Select("queue_id,status,COUNT(*) AS count").Where("queue_id IN ?", ids).Group("queue_id,status").Scan(&counts).Error != nil {
			Fail(c, 500, "发送进度读取失败")
			return
		}
		for _, count := range counts {
			i := byID[count.QueueID]
			queues[i].Counts[count.Status] = count.Count
			queues[i].Total += count.Count
		}
		for i := range queues {
			if queues[i].Status == "completed" {
				review := queues[i].Counts["failed"] + queues[i].Counts["unknown"]
				queues[i].Note = "本轮待发私信已处理完"
				if review > 0 {
					queues[i].Note = "本轮待发私信已处理完，仍有待复核记录，请查看发送失败和结果待确认列表"
				}
			}
		}
	}
	selected := 0
	if value := c.Query("queue_id"); value != "" {
		id, err := strconv.ParseUint(value, 10, 32)
		i, ok := byID[uint(id)]
		if err != nil || !ok {
			Fail(c, 404, "该批次没有这份发送队列，请刷新后重选")
			return
		}
		selected = i
	}
	type item struct {
		ID        uint       `json:"id"`
		UID       string     `json:"uid"`
		Name      string     `json:"name"`
		Status    string     `json:"status"`
		Note      string     `json:"note"`
		SentAt    *time.Time `json:"sent_at"`
		UpdatedAt time.Time  `json:"updated_at"`
	}
	rows := []item{}
	current := []item{}
	page := 1
	var total int64
	status := c.DefaultQuery("status", "all")
	allowed := map[string]bool{"all": true, "review": true, "pending": true, "sending": true, "sent": true, "failed": true, "unknown": true}
	if !allowed[status] {
		Fail(c, 400, "发送状态筛选无效")
		return
	}
	if value := c.Query("page"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100000 {
			Fail(c, 400, "页码无效")
			return
		}
		page = n
	}
	if len(queues) > 0 {
		q := queues[selected]
		items := database.DB.Table("bilibili_message_items AS i").Where("i.queue_id=?", q.ID)
		if status == "review" {
			items = items.Where("i.status IN ?", []string{"failed", "unknown"})
		} else if status != "all" {
			items = items.Where("i.status=?", status)
		}
		if e := items.Count(&total).Error; e != nil {
			Fail(c, 500, "发送明细读取失败")
			return
		}
		pages := int((total + 19) / 20)
		if pages < 1 {
			pages = 1
		}
		if page > pages {
			page = pages
		}
		selectRows := func() {
			name := database.DB.Table("bilibili_message_items AS i").Select("i.id,i.uid,CASE WHEN ?='voyage' THEN COALESCE(v.name,'') ELSE COALESCE(g.name,'') END AS name,i.status,i.note,i.sent_at,i.updated_at", q.Scope).Joins("LEFT JOIN voyage_recipients AS v ON ?='voyage' AND v.issue_id=? AND v.uid=i.uid", q.Scope, q.ScopeID).Joins("LEFT JOIN digital_gift_recipients AS g ON ?='gift' AND g.gift_id=? AND g.uid=i.uid", q.Scope, q.ScopeID).Where("i.queue_id=?", q.ID)
			if status == "review" {
				name = name.Where("i.status IN ?", []string{"failed", "unknown"})
			} else if status != "all" {
				name = name.Where("i.status=?", status)
			}
			e = name.Order("i.id").Limit(20).Offset((page - 1) * 20).Scan(&rows).Error
		}
		selectRows()
		if e != nil {
			Fail(c, 500, "发送明细读取失败")
			return
		}
		if database.DB.Table("bilibili_message_items AS i").Select("i.id,i.uid,i.status,i.note,i.sent_at,i.updated_at").Where("i.queue_id=? AND i.status='sending'", q.ID).Order("i.id").Limit(1).Scan(&current).Error != nil {
			Fail(c, 500, "当前发送状态读取失败")
			return
		}
	}
	var chosen *messageQueueSummary
	if len(queues) > 0 {
		chosen = &queues[selected]
	}
	var clock database.BilibiliMessageClock
	database.DB.Where("id=1").Limit(1).Find(&clock)
	Success(c, gin.H{"queues": queues, "selected": chosen, "items": rows, "current": current, "page": page, "page_size": 20, "total": total, "status": status, "next_at": clock.NextAt})
}
