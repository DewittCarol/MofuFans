package controller

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var voyageUIDPattern = regexp.MustCompile(`^[0-9]{1,19}$`)

type voyageImportRow struct {
	Line         int    `json:"line"`
	UID          string `json:"uid"`
	Tier         string `json:"tier"`
	Status       string `json:"status"`
	PreviousTier string `json:"previous_tier,omitempty"`
}

type voyageImportRecord struct {
	Line   int
	Values []string
}

func voyageTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "舰长", "captain", "3":
		return "captain"
	case "提督", "admiral", "2":
		return "admiral"
	case "总督", "governor", "1":
		return "governor"
	default:
		return ""
	}
}

func readVoyageImportRecords(text string) ([]voyageImportRecord, error) {
	delimiter := rune(0)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "\t") {
			delimiter = '\t'
			break
		}
		if strings.Contains(line, ",") {
			delimiter = ','
			break
		}
	}

	if delimiter == 0 {
		records := make([]voyageImportRecord, 0)
		for line, value := range strings.Split(text, "\n") {
			// Fields uses Unicode whitespace, so hand-typed spaces, full-width spaces,
			// non-breaking spaces, and pasted tabs all work in plain-text lists.
			fields := strings.Fields(strings.TrimSuffix(value, "\r"))
			if len(fields) > 0 {
				records = append(records, voyageImportRecord{Line: line + 1, Values: fields})
			}
		}
		return records, nil
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delimiter
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records := make([]voyageImportRecord, 0)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := r.FieldPos(0)
		records = append(records, voyageImportRecord{Line: line, Values: record})
	}
	return records, nil
}

func parseVoyageImport(text string) ([]voyageImportRow, error) {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		return nil, errors.New("请粘贴名单或选择 CSV 文件")
	}
	records, err := readVoyageImportRecords(text)
	if err != nil {
		return nil, errors.New("名单格式无法读取，请使用 CSV，或每行填写一个 UID；UID 和档位之间可用空格分隔")
	}
	uidCol, tierCol := 0, 1
	rows := []voyageImportRow{}
	seen := map[string]string{}
	first := true
	for _, parsed := range records {
		record, line := parsed.Values, parsed.Line
		empty := true
		for _, value := range record {
			if strings.TrimSpace(value) != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		if first {
			first = false
			for col, value := range record {
				key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
				if key == "uid" || key == "buid" || key == "用户id" || key == "用户uid" || key == "b站uid" {
					uidCol = col
					tierCol = -1
				}
			}
			if tierCol == -1 {
				for col, value := range record {
					switch strings.ToLower(strings.TrimSpace(value)) {
					case "档位", "等级", "大航海等级", "舰长类型", "tier", "guard_level":
						tierCol = col
					}
				}
				continue
			}
		}
		if uidCol >= len(record) {
			return nil, fmt.Errorf("第 %d 行缺少 UID", line)
		}
		uid := strings.TrimSpace(record[uidCol])
		if !voyageUIDPattern.MatchString(uid) {
			return nil, fmt.Errorf("第 %d 行 UID 无效，请使用完整数字，不能使用科学计数法", line)
		}
		_, uid, err = normalizeBilibiliUID(uid)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 UID 无效", line)
		}
		rawTier := ""
		if tierCol >= 0 && tierCol < len(record) {
			rawTier = strings.TrimSpace(record[tierCol])
		}
		tier := voyageTier(rawTier)
		if rawTier != "" && tier == "" {
			return nil, fmt.Errorf("第 %d 行档位无效，请使用舰长、提督、总督（或 3、2、1）", line)
		}
		status := "new"
		if old, ok := seen[uid]; ok {
			if old != "" && tier != "" && old != tier {
				return nil, fmt.Errorf("UID %s 在文件内有不同档位，请核对后再导入", uid)
			}
			status = "duplicate"
			if tier == "" {
				tier = old
			}
		}
		seen[uid] = tier
		rows = append(rows, voyageImportRow{Line: line, UID: uid, Tier: tier, Status: status})
		if len(rows) > 2000 {
			return nil, errors.New("每次最多导入 2000 行，请分批导入")
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("名单中没有用户；列头可以省略，请每行填写一个 UID，档位选填")
	}
	// A later duplicate may supply the optional tier for a new UID.
	for i := range rows {
		rows[i].Tier = seen[rows[i].UID]
	}
	return rows, nil
}
