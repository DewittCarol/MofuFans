package database

import (
	"gorm.io/gorm/clause"
	"strings"
)

func InitializeSongTags() error {
	defaults := []struct {
		kind  string
		names []string
	}{
		{"language", []string{"中文", "日文", "英文", "韩文", "粤语", "法文", "西班牙文", "其他语言"}},
		{"genre", []string{"流行", "KPOP", "JPOP", "说唱", "RnB", "摇滚", "民谣", "电子", "爵士", "古风", "ACG", "音乐剧"}},
	}
	for _, group := range defaults {
		for _, name := range group.names {
			row := SongTag{Kind: group.kind, Name: name, NameKey: strings.ToLower(name)}
			if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
