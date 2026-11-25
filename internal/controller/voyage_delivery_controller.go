package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"mofufans/internal/secret"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type VoyageAddress struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Note    string `json:"note"`
}

// Compare stored content exactly. A long-term preference excludes a batch's
// note; a parcel snapshot includes it because packing instructions can change.
func sameVoyageAddress(a, b *VoyageAddress, includeNote bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Name == b.Name && a.Phone == b.Phone && a.Address == b.Address && (!includeNote || a.Note == b.Note)
}

type addressHistory struct {
	IssueTitle string        `json:"issue_title"`
	Address    VoyageAddress `json:"address"`
}
type deliveryView struct {
	AddressSource   string                 `json:"address_source,omitempty"`
	LongTermSavedAt *time.Time             `json:"long_term_saved_at,omitempty"`
	LongTermChanged bool                   `json:"long_term_changed,omitempty"`
	LongTermOrigin  string                 `json:"long_term_origin,omitempty"`
	HasHistory      bool                   `json:"has_history,omitempty"`
	AddressBook     []addressBookEntryView `json:"address_book,omitempty"`
	LongTerm        *longTermAddressView   `json:"long_term,omitempty"`
	Items           []claimItemState       `json:"items"`
	Parcels         []claimParcel          `json:"parcels"`
	CanEditAddress  bool                   `json:"can_edit_address"`
	Labels          []string               `json:"labels"`
	Collecting      bool                   `json:"collecting"`
	History         []addressHistory       `json:"history,omitempty"`
	Address         *VoyageAddress         `json:"address"`
	Courier         string                 `json:"courier"`
	Tracking        string                 `json:"tracking"`
	ShippingNote    string                 `json:"shipping_note"`
	SubmittedAt     *time.Time             `json:"submitted_at"`
	ShippedAt       *time.Time             `json:"shipped_at"`
	Version         int                    `json:"version"`
}
type claimLimit struct {
	limiter *rate.Limiter
	seen    time.Time
}
type VoyageDeliveryController struct {
	mu     sync.Mutex
	limits map[string]*claimLimit
}

var claimPhone = regexp.MustCompile(`^\+?[0-9][0-9 -]{5,23}$`)
var trackingPattern = regexp.MustCompile(`^[A-Za-z0-9 -]{3,100}$`)
var claimUID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func deliveryRequest(c *gin.Context) bool {
	c.Header("Cache-Control", "private, no-store")
	limit := int64(16 << 10)
	if c.FullPath() == "/api/bilibili/messages" || strings.HasSuffix(c.FullPath(), "/parcels") || strings.HasSuffix(c.FullPath(), "/shipments/batch") || strings.HasSuffix(c.FullPath(), "/shipments/export") || strings.HasSuffix(c.FullPath(), "/members/import") || strings.HasSuffix(c.FullPath(), "/members/preview") || strings.HasSuffix(c.FullPath(), "/members/delete") {
		limit = 2 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
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
func (ctl *VoyageDeliveryController) allow(ip string) bool {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	now := time.Now()
	if ctl.limits == nil {
		ctl.limits = map[string]*claimLimit{}
	}
	entry := ctl.limits[ip]
	if entry == nil {
		// Bound both memory and rate without evicting active limits during an attack.
		if len(ctl.limits) >= 10000 {
			for k, v := range ctl.limits {
				if now.Sub(v.seen) > 10*time.Minute {
					delete(ctl.limits, k)
				}
			}
			if len(ctl.limits) >= 10000 {
				return false
			}
		}
		entry = &claimLimit{limiter: rate.NewLimiter(rate.Every(5*time.Second), 12)}
		ctl.limits[ip] = entry
	}
	entry.seen = now
	return entry.limiter.Allow()
}
func addressValid(a *VoyageAddress) bool {
	if a == nil {
		return false
	}
	a.Name = strings.TrimSpace(a.Name)
	a.Phone = strings.TrimSpace(a.Phone)
	a.Address = strings.TrimSpace(a.Address)
	a.Note = strings.TrimSpace(a.Note)
	return utf8.RuneCountInString(a.Name) >= 1 && utf8.RuneCountInString(a.Name) <= 40 && claimPhone.MatchString(a.Phone) && utf8.RuneCountInString(a.Address) >= 5 && utf8.RuneCountInString(a.Address) <= 300 && utf8.RuneCountInString(a.Note) <= 300
}
func readDelivery(db *gorm.DB, row database.VoyageRecipient) (deliveryView, error) {
	var record database.VoyageDelivery
	err := db.First(&record, "recipient_id = ?", row.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deliveryView{}, nil
	}
	if err != nil {
		return deliveryView{}, err
	}
	view := deliveryView{Courier: record.Courier, Tracking: record.Tracking, ShippingNote: record.ShippingNote, SubmittedAt: record.SubmittedAt, ShippedAt: record.ShippedAt, Version: record.Version}
	view.AddressSource, view.LongTermSavedAt = record.AddressSource, record.LongTermSavedAt
	view.LongTermOrigin = record.LongTermOrigin
	if record.AddressEncrypted != "" {
		raw, err := secret.OpenVoyageAddress(recipientScope(row.IssueID, row.UID), record.AddressEncrypted)
		if err != nil {
			return view, err
		}
		var address VoyageAddress
		if err = json.Unmarshal(raw, &address); err != nil {
			return view, err
		}
		view.Address = &address
	}
	if record.AddressSource == longTermAddressSource {
		p, err := readLongTermAddress(db, row.UID)
		if err != nil {
			return view, err
		}
		view.LongTermChanged = !p.Enabled
		if p.Enabled {
			address, err := openLongTermAddress(p)
			if err != nil {
				return view, err
			}
			view.LongTermChanged = !sameVoyageAddress(view.Address, &address, false)
		}
	}
	return view, nil
}

// Called only after successful current-issue verification; never accepts a separate UID.
func previousAddresses(tx *gorm.DB, current database.VoyageRecipient) ([]addressHistory, error) {
	rows := []database.VoyageRecipient{}
	err := tx.Joins("JOIN voyage_deliveries d ON d.recipient_id = voyage_recipients.id").Where("voyage_recipients.uid = ? AND voyage_recipients.id <> ? AND d.address_encrypted <> ''", current.UID, current.ID).Order("d.submitted_at desc").Limit(50).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := []addressHistory{}
	seen := map[string]bool{}
	for _, row := range rows {
		view, err := readDelivery(tx, row)
		if err != nil {
			return nil, err
		}
		if view.Address == nil {
			continue
		}
		a := *view.Address
		a.Note = ""
		key := a.Name + "\x00" + a.Phone + "\x00" + a.Address
		if seen[key] {
			continue
		}
		seen[key] = true
		var issue database.VoyageIssue
		if err := tx.Select("id", "title").First(&issue, row.IssueID).Error; err != nil {
			return nil, err
		}
		result = append(result, addressHistory{IssueTitle: issue.Title, Address: a})
		if len(result) == 8 {
			break
		}
	}
	return result, nil
}

var errClaim = errors.New("UID 或认证码不正确，或本期领取资格不可用")
var errDeliveryConflict = errors.New("记录已更新，请重新验证后再操作")
var errShipped = errors.New("已经发货，地址无法修改；如需调整请联系主播")
var errLongTermSavedAddress = errors.New("请先保存本次收货地址，再启用长期使用")

// Credentials travel in POST bodies, never URLs, storage, or public roster data.
// Both lookup and save re-check the current code, issue and disabled flag.
func (ctl *VoyageDeliveryController) Claim(c *gin.Context) {
	ctl.claim(c, false)
}

// MemberClaim never reads a recipient's private message code. It derives the
// subject from a current signature-verified account, then uses the same rules
// and write transaction as the original per-collection claim flow.
func (ctl *VoyageDeliveryController) MemberClaim(c *gin.Context) {
	ctl.claim(c, true)
}

func (ctl *VoyageDeliveryController) claim(c *gin.Context, memberAuth bool) {
	if !deliveryRequest(c) {
		return
	}
	if !ctl.allow(c.ClientIP()) {
		Fail(c, 429, "操作太频繁，请稍后再试")
		return
	}
	var req struct {
		UID             string         `json:"uid"`
		Code            string         `json:"code"`
		Action          string         `json:"action"`
		Address         *VoyageAddress `json:"address"`
		Version         int            `json:"version"`
		LongTermAction  string         `json:"long_term_action"`
		LongTermVersion int            `json:"long_term_version"`
		BookEntryID     uint           `json:"book_entry_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "请检查填写内容")
		return
	}
	issueID, e := strconv.ParseUint(c.Param("id"), 10, 32)
	req.UID = strings.TrimSpace(req.UID)
	if memberAuth {
		member, ok := MemberStrongUser(c)
		if !ok {
			Fail(c, 403, "请先登录并完成签名验证")
			return
		}
		uid := strconv.FormatInt(member.BilibiliUID, 10)
		if req.UID != "" && req.UID != uid {
			Fail(c, 403, "只能查看和修改本人的记录")
			return
		}
		req.UID = uid
	}
	if e != nil || issueID == 0 || !claimUID.MatchString(req.UID) || len(req.Code) > 80 {
		Fail(c, 401, errClaim.Error())
		return
	}
	if req.Action != "view" && req.Action != "save" && req.Action != "set_long_term" && req.Action != "cancel_long_term" && req.Action != "set_book_default" {
		Fail(c, 400, "操作无效")
		return
	}
	var attempt *guard.Attempt
	if !memberAuth {
		var allowed bool
		attempt, allowed = guard.BeginAuth(c, "claim", fmt.Sprintf("collection:%d:%s", issueID, req.UID))
		if !allowed {
			return
		}
		defer attempt.Cancel()
	}
	var result deliveryView
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if memberAuth {
			member, ok := MemberStrongUserInDB(c, tx)
			if !ok || strconv.FormatInt(member.BilibiliUID, 10) != req.UID {
				return errMemberIdentity
			}
		}
		var issue database.VoyageIssue
		var recipient database.VoyageRecipient
		if e := tx.First(&issue, issueID).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return errClaim
			}
			return e
		}
		batch, isBatch, e := batchFor(tx, issue.ID)
		if e != nil {
			return e
		}
		if (isBatch && !batch.Published) || (!isBatch && !issue.Published) {
			return errClaim
		}

		if e = tx.Where("issue_id = ? AND uid = ? AND disabled = ?", issue.ID, req.UID, false).First(&recipient).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return errClaim
			}
			return e
		}
		if !memberAuth && !secret.CheckVoyageCode(recipientScope(issue.ID, req.UID), req.Code, recipient.CodeHash) {
			attempt.Failure()
			return errClaim
		}
		if !memberAuth {
			attempt.Success()
		}
		if req.Action == "cancel_long_term" {
			if e := changeAddressPreference(tx, req.UID, "clear", req.LongTermVersion, nil, 0, 0, "user", 0, nil, ""); e != nil {
				return e
			}
		}
		// A view can materialize an already-consented address, only after auth.
		// Save compares the version the user actually saw before doing any fill.
		if req.Action == "view" {
			if e := inheritLongTermAddresses(tx, issue.ID, req.UID); e != nil {
				return e
			}
		}
		labels := batchLabels(recipient.BatchItems)
		current, e := readDelivery(tx, recipient)
		if e != nil {
			return e
		}
		fulfillment := tx.Migrator().HasTable(&database.VoyageParcel{})
		items, parcels, pending, e := claimFulfillment(tx, recipient)
		if e != nil {
			return e
		}
		canEdit := !isBatch || batchCollecting(batch) || (fulfillment && pending && current.Address != nil)
		if req.Action == "set_book_default" {
			saved, e := ownedBookAddress(tx, req.UID, req.BookEntryID)
			if e != nil {
				return e
			}
			if e = changeAddressPreference(tx, req.UID, "set", req.LongTermVersion, &saved, 0, 0, "user", 0, nil, ""); e != nil {
				return e
			}
			if e = inheritLongTermAddresses(tx, 0, req.UID); e != nil {
				return e
			}
		}
		if req.Action == "set_long_term" {
			if current.Version != req.Version {
				return errDeliveryConflict
			}
			if current.Address == nil {
				return errLongTermSavedAddress
			}
			// A preference toggle uses only the server's saved address. Request
			// drafts never update the collection or any existing parcel snapshot.
			saved := *current.Address
			if !addressValid(&saved) {
				return errLongTermSavedAddress
			}
			if e = changeAddressPreference(tx, req.UID, "set", req.LongTermVersion, &saved, issue.ID, recipient.ID, "user", 0, nil, ""); e != nil {
				return e
			}
			if e = inheritLongTermAddresses(tx, 0, req.UID); e != nil {
				return e
			}
		}
		if req.Action == "save" {
			if !canEdit {
				return errBatchClosed
			}
			if !fulfillment && current.ShippedAt != nil {
				return errShipped
			}
			if current.Version != req.Version {
				return errDeliveryConflict
			}
			if !addressValid(req.Address) {
				return errors.New("请填写收件人、有效手机号和完整收货地址")
			}
			if !sameVoyageAddress(current.Address, req.Address, true) {
				raw, _ := json.Marshal(req.Address)
				sealed, sealErr := secret.SealVoyageAddress(recipientScope(recipient.IssueID, req.UID), raw)
				if sealErr != nil {
					return sealErr
				}
				now := time.Now()
				if current.Version == 0 {
					e = tx.Create(&database.VoyageDelivery{RecipientID: recipient.ID, AddressEncrypted: sealed, SubmittedAt: &now, Version: 1}).Error
				} else {
					updated := tx.Model(&database.VoyageDelivery{}).Where("recipient_id = ? AND version = ?", recipient.ID, current.Version).Updates(map[string]any{"address_encrypted": sealed, "submitted_at": now, "version": current.Version + 1, "address_source": "", "long_term_version": 0, "long_term_saved_at": nil, "long_term_origin": ""})
					e = updated.Error
					if e == nil && updated.RowsAffected != 1 {
						return errDeliveryConflict
					}
				}
			} else if current.AddressSource != "" || current.LongTermSavedAt != nil {
				// Confirming an inherited address makes it a manual registration,
				// without pretending the address or packing instructions changed.
				updated := tx.Model(&database.VoyageDelivery{}).Where("recipient_id = ? AND version = ?", recipient.ID, current.Version).Updates(map[string]any{"address_source": "", "long_term_version": 0, "long_term_saved_at": nil, "long_term_origin": ""})
				e = updated.Error
				if e == nil && updated.RowsAffected != 1 {
					return errDeliveryConflict
				}
			}
			if e != nil {
				return e
			}
			if _, e = rememberVoyageAddress(tx, req.UID, issue.ID, recipient.ID, req.Address, "user"); e != nil {
				return e
			}
			if e = changeAddressPreference(tx, req.UID, req.LongTermAction, req.LongTermVersion, req.Address, issue.ID, recipient.ID, "user", 0, nil, ""); e != nil {
				return e
			}
			if e = inheritLongTermAddresses(tx, 0, req.UID); e != nil {
				return e
			}
		}
		result, e = readDelivery(tx, recipient)
		if e != nil {
			return e
		}
		result.Labels = labels
		result.Items = items
		result.Parcels = parcels
		result.CanEditAddress = canEdit
		if fulfillment {
			result.Courier = ""
			result.Tracking = ""
			result.ShippingNote = ""
			result.ShippedAt = nil
		}
		result.Collecting = !isBatch || batchCollecting(batch)
		result.History, e = previousAddresses(tx, recipient)
		if e != nil {
			return e
		}
		result.LongTerm, e = longTermState(tx, req.UID, result.Address)
		if e == nil {
			result.AddressBook, e = readAddressBook(tx, req.UID)
		}
		if memberAuth {
			// The global member identity permits own addresses, not disclosure of
			// private/draft collection titles and batch notes through old history.
			result.History = nil
			for i := range result.AddressBook {
				result.AddressBook[i].Uses = []addressBookUseView{}
			}
		}
		// Draft labels are private until the administrator confirms shipment.
		if result.ShippedAt == nil {
			result.Courier = ""
			result.Tracking = ""
			result.ShippingNote = ""
		}
		return e
	})
	if err != nil {
		switch {
		case errors.Is(err, errMemberIdentity):
			Fail(c, 403, errMemberIdentity.Error())
		case errors.Is(err, errClaim):
			Fail(c, 401, errClaim.Error())
		case errors.Is(err, errDeliveryConflict):
			Fail(c, 409, err.Error())
		case errors.Is(err, errBatchClosed):
			Fail(c, 409, err.Error())
		case errors.Is(err, errShipped):
			Fail(c, 409, err.Error())
		case errors.Is(err, errLongTermSavedAddress):
			Fail(c, 400, err.Error())
		case errors.Is(err, errBookEntry):
			Fail(c, 409, err.Error())
		case err.Error() == "请填写收件人、有效手机号和完整收货地址":
			Fail(c, 400, err.Error())
		default:
			Fail(c, 500, "记录暂时无法读取或保存，请稍后重试")
		}
		return
	}
	Success(c, result)
}

func (*VoyageDeliveryController) List(c *gin.Context) {
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	if err := syncLongTermAddresses(database.DB, issue.ID); err != nil {
		Fail(c, 500, "常用地址自动填写失败，请稍后重试")
		return
	}
	recipients, err := voyageRecipients(database.DB, issue.ID)
	if err != nil {
		Fail(c, 500, "名单读取失败")
		return
	}
	type item struct {
		ID       uint         `json:"id"`
		UID      string       `json:"uid"`
		Name     string       `json:"name"`
		Tier     string       `json:"tier"`
		Disabled bool         `json:"disabled"`
		Delivery deliveryView `json:"delivery"`
	}
	rows := []item{}
	for _, r := range recipients {
		view, err := readDelivery(database.DB, r)
		if err != nil {
			Fail(c, 500, "地址读取失败，请检查服务器加密配置")
			return
		}
		if view.Address == nil && addressBookAvailable(database.DB) {
			var count int64
			if err := database.DB.Model(&database.VoyageAddressBookEntry{}).Where("uid = ?", r.UID).Limit(1).Count(&count).Error; err != nil {
				Fail(c, 500, "地址簿暂时无法读取")
				return
			}
			view.HasHistory = count > 0
		}
		rows = append(rows, item{r.ID, r.UID, r.Name, r.Tier, r.Disabled, view})
	}
	Success(c, rows)
}

type shipmentRequest struct {
	ID               uint   `json:"id"`
	RecipientVersion int    `json:"recipient_version"`
	Version          int    `json:"version"`
	Courier          string `json:"courier"`
	Tracking         string `json:"tracking"`
	Note             string `json:"shipping_note"`
	Action           string `json:"action"`
}

type shipmentResult struct {
	ID       uint         `json:"id"`
	Delivery deliveryView `json:"delivery"`
}

func saveShipment(tx *gorm.DB, issueID uint, req shipmentRequest) (deliveryView, error) {
	if tx.Migrator().HasTable(&database.VoyageParcel{}) {
		return deliveryView{}, errors.New("发货已移到发货管理，请刷新页面后创建发货安排")
	}
	if e := lockVoyageBatchWrites(tx); e != nil {
		return deliveryView{}, e
	}
	recipient, err := voyageRecipient(tx, issueID, req.ID)
	if err != nil {
		return deliveryView{}, err
	}
	if recipient.Disabled {
		return deliveryView{}, errors.New("资格已停用")
	}
	if req.RecipientVersion != 0 && recipient.Version != req.RecipientVersion {
		return deliveryView{}, errDeliveryConflict
	}
	current, err := readDelivery(tx, recipient)
	if err != nil {
		return current, errors.New("地址读取失败，请检查加密配置")
	}
	if current.Version != req.Version {
		return current, errDeliveryConflict
	}
	if req.Action == "" {
		req.Action = "ship"
	}
	if req.Action != "draft" && req.Action != "ship" && req.Action != "unship" {
		return current, errors.New("操作无效")
	}
	courier, tracking, note := current.Courier, current.Tracking, current.ShippingNote
	shipped := current.ShippedAt
	if req.Action == "unship" {
		if shipped == nil {
			return current, errors.New("尚未发货，请刷新名单")
		}
		shipped = nil
	} else {
		if recipient.SentAt == nil {
			return current, errors.New("请先确认私信已发送")
		}
		courier, tracking, note = strings.TrimSpace(req.Courier), strings.TrimSpace(req.Tracking), strings.TrimSpace(req.Note)
		if courier == "" || utf8.RuneCountInString(courier) > 40 || !trackingPattern.MatchString(tracking) || utf8.RuneCountInString(note) > 300 {
			return current, errors.New("请填写快递公司和有效运单号，备注最多 300 字")
		}
		if req.Action == "ship" {
			if current.Address == nil {
				return current, errors.New("用户尚未填写地址，暂时不能确认发货")
			}
			if shipped == nil {
				now := time.Now()
				shipped = &now
			}
		}
	}
	if current.Version == 0 {
		err = tx.Create(&database.VoyageDelivery{RecipientID: recipient.ID, Courier: courier, Tracking: tracking, ShippingNote: note, ShippedAt: shipped, Version: 1}).Error
	} else {
		updated := tx.Model(&database.VoyageDelivery{}).Where("recipient_id = ? AND version = ?", recipient.ID, req.Version).Updates(map[string]any{"courier": courier, "tracking": tracking, "shipping_note": note, "shipped_at": shipped, "version": req.Version + 1})
		err = updated.Error
		if err == nil && updated.RowsAffected != 1 {
			return current, errDeliveryConflict
		}
	}
	if err != nil {
		return current, errors.New("运单保存失败，请刷新后重试")
	}
	return readDelivery(tx, recipient)
}

func (*VoyageDeliveryController) Ship(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	rowID, e := strconv.ParseUint(c.Param("recipient"), 10, 32)
	if e != nil || rowID == 0 {
		Fail(c, 400, "名单用户无效")
		return
	}
	var req shipmentRequest
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "物流信息格式无效")
		return
	}
	req.ID = uint(rowID)
	var result deliveryView
	err := database.DB.Transaction(func(tx *gorm.DB) error { var err error; result, err = saveShipment(tx, issue.ID, req); return err })
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, result)
}

// A batch is atomic: invalid or stale rows never leave a partially shipped batch.
func (*VoyageDeliveryController) Batch(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	var req struct {
		Action string            `json:"action"`
		Items  []shipmentRequest `json:"items"`
	}
	if c.ShouldBindJSON(&req) != nil {
		Fail(c, 400, "批量运单格式无效或文件过大")
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 500 {
		Fail(c, 400, "每批请选择 1–500 人")
		return
	}
	if req.Action != "draft" && req.Action != "ship" && req.Action != "unship" {
		Fail(c, 400, "批量操作无效")
		return
	}
	results := make([]shipmentResult, 0, len(req.Items))
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		seen := map[uint]bool{}
		for i, item := range req.Items {
			if item.ID == 0 || seen[item.ID] {
				return fmt.Errorf("第 %d 行用户无效或重复", i+1)
			}
			seen[item.ID] = true
			item.Action = req.Action
			view, err := saveShipment(tx, issue.ID, item)
			if err != nil {
				return fmt.Errorf("第 %d 行：%s；本批未保存，请修正后重试", i+1, err.Error())
			}
			results = append(results, shipmentResult{item.ID, view})
		}
		return nil
	})
	if err != nil {
		Fail(c, 409, err.Error())
		return
	}
	Success(c, results)
}
