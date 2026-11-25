package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mofufans/internal/database"
	"mofufans/internal/theme"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

type ThemeController struct{}
type themeRequest struct {
	Revision uint            `json:"revision"`
	Name     string          `json:"name"`
	Theme    json.RawMessage `json:"theme"`
}

func readThemeRequest(c *gin.Context) (themeRequest, theme.Config, error) {
	var r themeRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, theme.Config{}, errors.New("主题请求格式不正确或超过 64 KB")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return r, theme.Config{}, errors.New("主题请求包含多余内容")
	}
	t, err := theme.Decode(r.Theme)
	return r, t, err
}
func (t *ThemeController) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var row database.SiteTheme
	if err := database.DB.First(&row, 1).Error; err != nil {
		Fail(c, 503, "主题配置暂时不可用")
		return
	}
	cfg, err := theme.Decode([]byte(row.Config))
	if err != nil {
		Fail(c, 500, "主题配置读取失败")
		return
	}
	Success(c, gin.H{"revision": row.Revision, "theme": cfg})
}
func (t *ThemeController) Put(c *gin.Context) {
	r, cfg, err := readThemeRequest(c)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	raw, _ := json.Marshal(cfg)
	// Compare-and-swap prevents another open editor from silently losing changes.
	result := database.DB.Model(&database.SiteTheme{}).Where("id = ? AND revision = ?", 1, r.Revision).Updates(map[string]any{"config": string(raw), "revision": r.Revision + 1})
	if result.Error != nil {
		Fail(c, 500, "保存主题失败")
		return
	}
	if result.RowsAffected != 1 {
		Fail(c, 409, "主题已在其他页面修改，请重新载入后再保存")
		return
	}
	Success(c, gin.H{"revision": r.Revision + 1, "theme": cfg})
}
func (t *ThemeController) Presets(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var rows []database.ThemePreset
	if database.DB.Order("id asc").Find(&rows).Error != nil {
		Fail(c, 500, "读取预设失败")
		return
	}
	out := []gin.H{{"id": "default", "name": "默认主题", "builtin": true, "theme": theme.Default()}}
	for _, row := range rows {
		cfg, err := theme.Decode([]byte(row.Config))
		if err != nil {
			continue
		}
		out = append(out, gin.H{"id": strconv.FormatUint(uint64(row.ID), 10), "name": row.Name, "builtin": false, "theme": cfg})
	}
	Success(c, out)
}
func (t *ThemeController) SavePreset(c *gin.Context) {
	r, cfg, err := readThemeRequest(c)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	name := strings.TrimSpace(r.Name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		Fail(c, 400, "预设名称请填写 1–80 个字")
		return
	}
	raw, _ := json.Marshal(cfg)
	row := database.ThemePreset{Name: name, Config: string(raw)}
	if database.DB.Create(&row).Error != nil {
		Fail(c, 500, "保存预设失败")
		return
	}
	Success(c, gin.H{"id": strconv.FormatUint(uint64(row.ID), 10), "name": row.Name, "builtin": false, "theme": cfg})
}
func (t *ThemeController) DeletePreset(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Fail(c, 400, "内置预设不能删除")
		return
	}
	if database.DB.Delete(&database.ThemePreset{}, id).Error != nil {
		Fail(c, 500, "删除预设失败")
		return
	}
	Success(c, nil)
}
