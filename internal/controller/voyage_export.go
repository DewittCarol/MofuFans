package controller

import (
	"errors"
	"fmt"
	"mime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"mofufans/internal/database"
)

func (*VoyageDeliveryController) Export(c *gin.Context) {
	if !deliveryRequest(c) {
		return
	}
	issue, ok := voyageIssueForRequest(c)
	if !ok {
		return
	}
	var req struct {
		IDs []uint `json:"ids"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.IDs) == 0 || len(req.IDs) > 10000 {
		Fail(c, 400, "请选择需要导出的地址，最多 10000 人")
		return
	}
	requested := make(map[uint]bool, len(req.IDs))
	for _, id := range req.IDs {
		requested[id] = true
	}
	var data [][]string
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := lockVoyageBatchWrites(tx); e != nil {
			return e
		}
		if e := inheritLongTermAddresses(tx, issue.ID, ""); e != nil {
			return e
		}
		recipients, err := voyageRecipients(tx, issue.ID)
		if err != nil {
			return err
		}
		for _, row := range recipients {
			if !requested[row.ID] {
				continue
			}
			delete(requested, row.ID)
			d, err := readDelivery(tx, row)
			if err != nil {
				return err
			}
			if d.Address == nil {
				return errors.New("所选用户的地址已变更，请刷新名单后重新导出")
			}
			tier := map[string]string{"captain": "舰长", "admiral": "提督", "governor": "总督"}[row.Tier]
			status := "未发私信"
			if row.SentAt != nil {
				status = "已发私信"
			}
			if d.ShippedAt != nil {
				status = "已发货"
			}
			if row.Disabled {
				status = "已停用"
			}
			formatted := func(t *time.Time) string {
				if t == nil {
					return ""
				}
				return t.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02 15:04:05")
			}
			a := d.Address
			data = append(data, []string{row.UID, row.Name, tier, a.Name, a.Phone, a.Address, a.Note, status, d.Courier, d.Tracking, d.ShippingNote, formatted(d.SubmittedAt), formatted(d.ShippedAt), batchDescription(row)})
		}
		if len(requested) > 0 {
			return errors.New("部分用户不在本期名单中，请刷新后重新导出")
		}
		return nil
	})
	if err != nil {
		Fail(c, 409, "地址读取失败，请刷新名单后重试")
		return
	}
	if len(data) == 0 {
		Fail(c, 400, "没有可导出的收货地址")
		return
	}
	content, err := addressWorkbook(data)
	if err != nil {
		Fail(c, 500, "Excel 生成失败，请重试")
		return
	}
	filename := fmt.Sprintf("%s-收货地址-%s.xlsx", issue.Title, time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("20060102-1504"))
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Mofu-Export-Count", fmt.Sprint(len(data)))
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

// Explicit string cells keep UIDs, phone numbers and tracking IDs intact and never execute user text as formulas.
func addressWorkbook(rows [][]string) ([]byte, error) { return addressWorkbookColumns(rows, nil) }
func addressWorkbookColumns(rows [][]string, customHeaders []string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "收货地址"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	headers := []string{"UID", "昵称", "档位", "姓名", "手机号码", "地址", "收件备注", "状态", "快递公司", "运单号", "物流备注", "地址提交时间", "发货时间", "本次领取范围"}
	if len(customHeaders) == len(headers) {
		headers = customHeaders
	}
	all := append([][]string{headers}, rows...)
	for r, values := range all {
		for col, value := range values {
			cell, err := excelize.CoordinatesToCellName(col+1, r+1)
			if err != nil {
				return nil, err
			}
			if err = f.SetCellStr(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}
	body, err := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Family: "等线", Size: 11, Color: "49343D"}, Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "等线", Size: 11, Bold: true, Color: "49343D"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"FEBCC6"}, Pattern: 1}, Alignment: &excelize.Alignment{Vertical: "center"}})
	if err != nil {
		return nil, err
	}
	end := fmt.Sprintf("N%d", len(all))
	if err = f.SetCellStyle(sheet, "A2", end, body); err != nil {
		return nil, err
	}
	if err = f.SetCellStyle(sheet, "A1", "N1", head); err != nil {
		return nil, err
	}
	widths := []float64{22, 20, 10, 16, 22, 58, 35, 18, 18, 26, 35, 23, 23, 65}
	for i, width := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err = f.SetColWidth(sheet, col, col, width); err != nil {
			return nil, err
		}
	}
	if err = f.SetRowHeight(sheet, 1, 26); err != nil {
		return nil, err
	}
	for r := 2; r <= len(all); r++ {
		if err = f.SetRowHeight(sheet, r, 48); err != nil {
			return nil, err
		}
	}
	if err = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if err = f.AutoFilter(sheet, "A1:"+end, nil); err != nil {
		return nil, err
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
