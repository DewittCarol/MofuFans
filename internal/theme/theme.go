// Package theme defines the portable, versioned theme contract shared by both clients.
package theme

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

//go:embed default.json
var defaultJSON []byte

type Brand struct {
	SiteName      string `json:"site_name"`
	StreamerName  string `json:"streamer_name"`
	TabTitle      string `json:"tab_title"`
	AdminTitle    string `json:"admin_title"`
	Description   string `json:"description"`
	Logo          string `json:"logo"`
	Avatar        string `json:"avatar"`
	Favicon       string `json:"favicon"`
	AdminFavicon  string `json:"admin_favicon"`
	CursorMessage string `json:"cursor_message"`
	BilibiliUID   string `json:"bilibili_uid"`
}
type Module struct {
	Label string `json:"label"`
	Icon  string `json:"icon"`
}
type Appearance struct {
	Texture      bool `json:"texture"`
	PixelLogo    bool `json:"pixel_logo"`
	Cursor       bool `json:"cursor"`
	Pet          bool `json:"pet"`
	ModuleColors bool `json:"module_colors"`
}
type Config struct {
	SchemaVersion int               `json:"schema_version"`
	Brand         Brand             `json:"brand"`
	Colors        map[string]string `json:"colors"`
	AdminColors   map[string]string `json:"admin_colors"`
	Modules       map[string]Module `json:"modules"`
	Appearance    Appearance        `json:"appearance"`
	Pet           Pet               `json:"pet"`
}

type Pet struct {
	ModelURL string `json:"model_url"`
	Label    string `json:"label"`
}

func Default() Config {
	var c Config
	if err := json.Unmarshal(defaultJSON, &c); err != nil {
		panic(err)
	}
	return c
}
func Decode(raw []byte) (Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, errors.New("主题文件格式不正确")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return c, errors.New("主题文件包含多余内容")
	}
	// Version-one exports from older installations have four module entries.
	// Fill only the newly introduced optional fields, never overwrite saved colors.
	if c.Modules != nil {
		if _, ok := c.Modules["gifts"]; !ok {
			c.Modules["gifts"] = Default().Modules["gifts"]
		}
	}
	if c.Colors != nil && c.Colors["gifts"] == "" {
		c.Colors["gifts"] = c.Colors["primary"]
	}
	return c, Validate(c)
}

var hex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func AssetURL(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 2048 || strings.ContainsAny(s, "\\\r\n\t\"'<>") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return u.Host == "" && u.Scheme == ""
	}
	return u.Scheme == "https" && u.Hostname() != ""
}
func Validate(c Config) error {
	if c.SchemaVersion != 1 {
		return errors.New("不支持此主题版本")
	}
	defaults := Default()
	if c.Brand.BilibiliUID != "" && !regexp.MustCompile(`^[1-9][0-9]{0,18}$`).MatchString(c.Brand.BilibiliUID) {
		return errors.New("B 站 UID 格式不正确")
	}
	if !AssetURL(c.Pet.ModelURL) || utf8.RuneCountInString(c.Pet.Label) > 80 {
		return errors.New("桌宠地址或名称不正确")
	}
	for key, value := range map[string]string{"网站名称": c.Brand.SiteName, "主播名称": c.Brand.StreamerName, "前台页签标题": c.Brand.TabTitle, "后台标题": c.Brand.AdminTitle} {
		if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 80 {
			return fmt.Errorf("%s请填写 1–80 个字", key)
		}
	}
	if utf8.RuneCountInString(c.Brand.Description) > 300 || utf8.RuneCountInString(c.Brand.CursorMessage) > 300 {
		return errors.New("说明文字不能超过 300 字")
	}
	for _, v := range []string{c.Brand.Logo, c.Brand.Avatar, c.Brand.Favicon, c.Brand.AdminFavicon} {
		if !AssetURL(v) {
			return errors.New("图片地址需要是站内路径或 HTTPS 链接")
		}
	}
	if len(c.Colors) != len(defaults.Colors) {
		return errors.New("主题颜色项不完整")
	}
	for k := range defaults.Colors {
		if !hex.MatchString(c.Colors[k]) {
			return fmt.Errorf("颜色 %s 需要是 #RRGGBB 格式", k)
		}
	}
	if c.AdminColors == nil {
		return errors.New("后台颜色项不正确")
	}
	for k, v := range c.AdminColors {
		if _, ok := defaults.AdminColors[k]; !ok || !hex.MatchString(v) {
			return errors.New("后台颜色项不正确")
		}
	}
	if len(c.Modules) != len(defaults.Modules) {
		return errors.New("模块信息不完整")
	}
	for k := range defaults.Modules {
		v := c.Modules[k]
		if strings.TrimSpace(v.Label) == "" || utf8.RuneCountInString(v.Label) > 20 || !AssetURL(v.Icon) {
			return errors.New("模块名称或图标不正确")
		}
	}
	return nil
}
