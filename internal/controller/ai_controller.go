package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"mofufans/internal/ai"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type AIController struct {
	Client  *ai.Client
	mu      sync.Mutex
	probeMu sync.Mutex
}
type AIRequest struct {
	ai.Options
	APIKey      string `json:"api_key"`
	ClearKey    bool   `json:"clear_key"`
	SpamPrompt  string `json:"spam_prompt"`
	TestContent string `json:"test_content"`
}

func aiOwner(c *gin.Context) bool {
	user, _ := c.Get("user")
	admin, ok := user.(database.Admin)
	if !ok || !admin.IsSuperAdmin {
		Fail(c, 403, "仅站点所有者可以配置 AI 服务")
		return false
	}
	c.Header("Cache-Control", "no-store")
	return true
}
func aiView(config database.Config) (any, error) {
	options, err := config.AIOptions()
	if err != nil {
		return nil, err
	}
	return struct {
		ai.Options
		APIKeyConfigured bool   `json:"api_key_configured"`
		SpamPrompt       string `json:"spam_prompt"`
	}{options, config.AIKeyCiphertext != "", config.SpamPrompt}, nil
}
func (ctl *AIController) Get(c *gin.Context) {
	if !aiOwner(c) {
		return
	}
	var config database.Config
	if database.DB.First(&config).Error != nil {
		Fail(c, 500, "AI 设置读取失败")
		return
	}
	view, err := aiView(config)
	if err != nil {
		Fail(c, 500, err.Error())
		return
	}
	Success(c, view)
}
func aiPropose(request AIRequest, config database.Config, requireModel bool) (ai.Options, string, error) {
	options := ai.Normalize(request.Options)
	if err := ai.Validate(options, requireModel); err != nil {
		return options, "", err
	}
	old, err := config.AIOptions()
	if err != nil {
		return options, "", err
	}
	key := strings.TrimSpace(request.APIKey)
	if len(key) > 8192 || strings.ContainsAny(key, "\r\n") {
		return options, "", errors.New("API Key 格式不正确")
	}
	if request.ClearKey {
		key = ""
	} else if key == "" && config.AIKeyCiphertext != "" {
		if options.BaseURL != ai.Normalize(old).BaseURL || options.Protocol != old.Protocol {
			return options, "", errors.New("更换服务地址或协议后，请重新填写对应的 API Key")
		}
		key, err = config.AIKey()
		if err != nil {
			return options, "", err
		}
	}
	return options, key, nil
}
func (ctl *AIController) Put(c *gin.Context) {
	if !aiOwner(c) {
		return
	}
	var request AIRequest
	if !aiBody(c, &request) {
		return
	}
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	var config database.Config
	if database.DB.First(&config).Error != nil {
		Fail(c, 500, "AI 设置读取失败")
		return
	}
	options, key, err := aiPropose(request, config, request.Enabled)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	if options.Enabled && key == "" {
		Fail(c, 400, "启用 AI 审核前请填写 API Key")
		return
	}
	rules := strings.TrimSpace(request.SpamPrompt)
	if rules == "" || len(rules) > 12000 {
		Fail(c, 400, "请填写判定标准，长度不超过 12000 字节")
		return
	}
	ciphertext := ""
	if key != "" {
		ciphertext, err = secret.EncryptCookie(key)
		if err != nil {
			Fail(c, 500, "AI 密钥保存失败，请检查实例加密配置")
			return
		}
	}
	raw, _ := json.Marshal(options)
	if database.DB.Model(&config).Updates(map[string]any{"ai_config": string(raw), "ai_key_ciphertext": ciphertext, "deep_seek_api_key": "", "spam_prompt": rules}).Error != nil {
		Fail(c, 500, "AI 设置保存失败")
		return
	}
	config.AIConfig = string(raw)
	config.AIKeyCiphertext = ciphertext
	config.SpamPrompt = rules
	view, _ := aiView(config)
	Success(c, view)
}
func (ctl *AIController) Models(c *gin.Context) { ctl.probe(c, false) }
func (ctl *AIController) Test(c *gin.Context)   { ctl.probe(c, true) }
func (ctl *AIController) probe(c *gin.Context, test bool) {
	if !aiOwner(c) {
		return
	}
	if !ctl.probeMu.TryLock() {
		Fail(c, 429, "已有一个模型测试或模型列表请求正在进行，请稍后重试")
		return
	}
	defer ctl.probeMu.Unlock()
	var request AIRequest
	if !aiBody(c, &request) {
		return
	}
	var config database.Config
	if database.DB.First(&config).Error != nil {
		Fail(c, 500, "AI 设置读取失败")
		return
	}
	options, key, err := aiPropose(request, config, test)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	client := ctl.Client
	if client == nil {
		client = ai.DefaultClient
	}
	timeout := 20 * time.Second
	if test {
		timeout = ai.RequestTimeout
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	if test {
		content := strings.TrimSpace(request.TestContent)
		if content == "" {
			content = "这是一条连接测试提问：下一次直播打算唱什么歌？"
		}
		if len([]rune(content)) > 4000 {
			Fail(c, 400, "测试文字最多 4000 字")
			return
		}
		var images []ai.Image
		if options.ReviewImages {
			im := image.NewRGBA(image.Rect(0, 0, 128, 128))
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					im.Set(x, y, color.RGBA{220, 60, 80, 255})
				}
			}
			var data bytes.Buffer
			_ = png.Encode(&data, im)
			images = []ai.Image{{MIME: "image/png", Data: data.Bytes()}}
		}
		verdict, err := client.Review(ctx, options, key, request.SpamPrompt, content, "连接测试", len(images), images)
		if err != nil {
			Fail(c, 400, err.Error())
			return
		}
		Success(c, gin.H{"is_spam": verdict.IsSpam, "reason": verdict.Reason, "images_tested": len(images), "message": "当前模型已返回有效审核结果"})
	} else {
		models, err := client.Models(ctx, options, key)
		if err != nil {
			Fail(c, 400, err.Error())
			return
		}
		Success(c, models)
	}
}

func aiBody(c *gin.Context, out any) bool {
	origin := c.GetHeader("Origin")
	if origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != c.Request.Host {
			Fail(c, 403, "请从当前网站后台操作")
			return false
		}
	}
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		Fail(c, 403, "请从当前网站后台操作")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if c.ShouldBindJSON(out) != nil {
		Fail(c, 400, "请求内容无效或过大")
		return false
	}
	return true
}
