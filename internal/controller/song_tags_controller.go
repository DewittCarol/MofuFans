package controller

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
	"mofufans/internal/database"
	"strings"
	"unicode/utf8"
)

func (*SongController) Tags(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rows := []database.SongTag{}
	if database.DB.Order("id asc").Find(&rows).Error != nil {
		Fail(c, 500, "标签加载失败，请重试")
		return
	}
	Success(c, gin.H{"items": rows, "total": len(rows), "page": 1, "page_size": len(rows)})
}

func (*SongController) CreateTag(c *gin.Context) {
	if !libraryRequest(c, 4096) {
		return
	}
	var input struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&input) != nil {
		Fail(c, 400, "标签格式无效")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if (input.Kind != "language" && input.Kind != "genre") || input.Name == "" || utf8.RuneCountInString(input.Name) > 24 {
		Fail(c, 400, "请选择语言或流派，标签名为 1–24 字")
		return
	}
	row := database.SongTag{Kind: input.Kind, Name: input.Name, NameKey: strings.ToLower(input.Name)}
	if database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error != nil {
		Fail(c, 500, "标签创建失败")
		return
	}
	if database.DB.Where("kind = ? AND name_key = ?", input.Kind, strings.ToLower(input.Name)).First(&row).Error != nil {
		Fail(c, 500, "标签读取失败")
		return
	}
	Success(c, row)
}
