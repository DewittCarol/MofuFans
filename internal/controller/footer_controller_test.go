package controller

import (
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"testing"
)

func TestFooterPersistenceAndClearing(t *testing.T) {
	appearanceDB(t)
	database.DB.Create(&database.Config{AIConfig: "keep-ai", SpamPrompt: "keep-rules", BackgroundURL: "keep-background"})
	r := gin.New()
	ctl := &ConfigController{}
	r.PUT("/footer", ctl.PutFooter)
	footer := FooterSettings{ICPNumber: " 测试 ICP 备案号 ", PoliceNumber: "测试公网安备 12345678901234 号", PoliceIcon: "/api/theme-assets/test.png"}
	if code, body := settingsRequest(t, r, "PUT", "/footer", footer); code != 200 {
		t.Fatal(body)
	}
	var config database.Config
	database.DB.First(&config)
	if config.FooterICPNumber != "测试 ICP 备案号" || config.FooterPoliceURL != "https://beian.mps.gov.cn/#/query/webSearch?code=12345678901234" || config.AIConfig != "keep-ai" || config.BackgroundURL != "keep-background" {
		t.Fatal("footer not saved independently")
	}
	if code, body := settingsRequest(t, r, "PUT", "/footer", FooterSettings{}); code != 200 {
		t.Fatal(body)
	}
	database.DB.First(&config)
	if config.FooterICPNumber != "" || config.FooterPoliceNumber != "" || config.FooterPoliceURL != "" || config.FooterPoliceIcon != "" {
		t.Fatal("empty fields were not cleared")
	}
}
func TestFooterRejectsUnsafeLinks(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "https://beian.mps.gov.cn.evil.test/", "http://beian.mps.gov.cn/", "https://evil@beian.mps.gov.cn/", "https://beian.mps.gov.cn:1234/"} {
		f := FooterSettings{PoliceURL: value}
		if f.validate() == nil {
			t.Errorf("unsafe link accepted: %s", value)
		}
	}
	for _, value := range []string{"javascript:alert(1)", "//evil.test/image.png", "/bad\"onerror=alert(1)"} {
		f := FooterSettings{PoliceIcon: value}
		if f.validate() == nil {
			t.Error("unsafe icon accepted")
		}
	}
}
