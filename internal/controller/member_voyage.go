package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

var errMemberIdentity = errors.New("登录已失效或尚未完成签名验证，请重新登录后操作")
var errMemberBookFull = errors.New("地址簿最多保留 50 个不同地址，请联系主播协助处理")

type MemberVoyageController struct{}

type memberAddressBookView struct {
	UID      string                 `json:"uid"`
	Entries  []addressBookEntryView `json:"entries"`
	LongTerm *longTermAddressView   `json:"long_term"`
}

func memberAddressBook(tx *gorm.DB, uid string) (memberAddressBookView, error) {
	view := memberAddressBookView{UID: uid}
	var err error
	view.Entries, err = readAddressBook(tx, uid)
	if err != nil {
		return view, err
	}
	// Personal address management does not disclose draft collection titles or
	// administrator-only use history. Batch notes remain on their own claim page.
	for i := range view.Entries {
		view.Entries[i].Uses = []addressBookUseView{}
	}
	view.LongTerm, err = longTermState(tx, uid, nil)
	return view, err
}

func (*MemberVoyageController) AddressBook(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	member, ok := MemberStrongUser(c)
	if !ok {
		Fail(c, 403, errMemberIdentity.Error())
		return
	}
	uid := strconv.FormatInt(member.BilibiliUID, 10)
	if expected := c.Query("expected_uid"); expected != "" && expected != uid {
		Fail(c, 403, "登录账号已切换，请重新打开用户中心")
		return
	}
	view, err := memberAddressBook(database.DB, uid)
	if err != nil {
		Fail(c, 503, "地址簿暂时无法读取，请稍后重试")
		return
	}
	Success(c, view)
}

func (*MemberVoyageController) SaveAddressBook(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	member, ok := MemberStrongUser(c)
	if !ok {
		Fail(c, 403, errMemberIdentity.Error())
		return
	}
	uid := strconv.FormatInt(member.BilibiliUID, 10)
	var req struct {
		Action          string         `json:"action"`
		UID             string         `json:"uid"`
		Address         *VoyageAddress `json:"address"`
		EntryID         uint           `json:"entry_id"`
		LongTermVersion int            `json:"long_term_version"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Action != "save" && req.Action != "set_default" && req.Action != "clear_default") {
		Fail(c, 400, "请检查填写内容")
		return
	}
	if req.UID != "" && req.UID != uid {
		Fail(c, 403, "只能修改本人的地址簿")
		return
	}
	if req.Action == "save" {
		if !addressValid(req.Address) {
			Fail(c, 400, "请填写收件人、有效手机号和完整收货地址")
			return
		}
		if req.Address.Note != "" {
			Fail(c, 400, "备注请在对应收集批次中填写")
			return
		}
	}
	var view memberAddressBookView
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVoyageBatchWrites(tx); err != nil {
			return err
		}
		live, ok := MemberStrongUserInDB(c, tx)
		if !ok || live.BilibiliUID != member.BilibiliUID {
			return errMemberIdentity
		}
		switch req.Action {
		case "save":
			// Immutable versions preserve snapshots already used by collections.
			var entries []database.VoyageAddressBookEntry
			if err := tx.Where("uid = ?", uid).Find(&entries).Error; err != nil {
				return err
			}
			if len(entries) >= 50 {
				found := false
				for _, entry := range entries {
					address, err := openBookEntry(entry)
					if err != nil {
						return err
					}
					if sameVoyageAddress(&address, req.Address, false) {
						found = true
						break
					}
				}
				if !found {
					return errMemberBookFull
				}
			}
			if _, err := rememberVoyageAddress(tx, uid, 0, 0, req.Address, "user"); err != nil {
				return err
			}
		case "set_default":
			address, err := ownedBookAddress(tx, uid, req.EntryID)
			if err != nil {
				return err
			}
			if err = changeAddressPreference(tx, uid, "set", req.LongTermVersion, &address, 0, 0, "user", 0, nil, ""); err != nil {
				return err
			}
			if err = inheritLongTermAddresses(tx, 0, uid); err != nil {
				return err
			}
		case "clear_default":
			if err := changeAddressPreference(tx, uid, "clear", req.LongTermVersion, nil, 0, 0, "user", 0, nil, ""); err != nil {
				return err
			}
		}
		var err error
		view, err = memberAddressBook(tx, uid)
		return err
	})
	if err != nil {
		switch {
		case errors.Is(err, errMemberIdentity):
			Fail(c, 403, err.Error())
		case errors.Is(err, errBookEntry), errors.Is(err, errDeliveryConflict), errors.Is(err, errMemberBookFull):
			Fail(c, 409, err.Error())
		default:
			Fail(c, 503, "地址簿暂时无法保存，请稍后重试")
		}
		return
	}
	Success(c, view)
}

func (*MemberVoyageController) Collections(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	member, ok := MemberStrongUser(c)
	if !ok {
		Fail(c, 403, errMemberIdentity.Error())
		return
	}
	uid := strconv.FormatInt(member.BilibiliUID, 10)
	rows := []database.VoyageIssue{}
	if expected := c.Query("expected_uid"); expected != "" && expected != uid {
		Fail(c, 403, "登录账号已切换，请重新打开用户中心")
		return
	}
	// The public catalog switch controls visitors. Authenticated recipients may
	// still access their own published records; drafts and disabled recipients
	// remain private. No recipient or claim is created by visiting this endpoint.
	err := database.DB.Joins("JOIN voyage_recipients r ON r.issue_id = voyage_issues.id").Where("r.uid = ? AND r.disabled = ?", uid, false).Order("voyage_issues.id desc").Limit(500).Find(&rows).Error
	if err != nil {
		Fail(c, 503, "领取记录暂时无法读取，请稍后重试")
		return
	}
	result := []gin.H{}
	for _, issue := range rows {
		batch, isBatch, err := batchFor(database.DB, issue.ID)
		if err != nil {
			Fail(c, 503, "领取记录暂时无法读取，请稍后重试")
			return
		}
		if isBatch {
			if !batch.Published {
				continue
			}
			result = append(result, gin.H{"id": batch.ID, "title": batch.Title, "note": batch.Note, "opens": batch.Opens, "closes": batch.Closes, "closed": batch.Closed, "collecting": batchCollecting(batch)})
		} else if issue.Published {
			result = append(result, gin.H{"id": issue.ID, "title": issue.Title, "note": "", "opens": "", "closes": "", "closed": false, "collecting": true})
		}
	}
	Success(c, result)
}
