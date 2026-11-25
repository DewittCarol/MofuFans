package controller

import (
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"testing"
)

func TestMemberAccessSwitchAndPostingPolicy(t *testing.T) {
	appearanceDB(t)
	database.DB.Create(&database.Config{SpamPrompt: "keep"})
	r := gin.New()
	ctl := &ConfigController{}
	r.PUT("/settings", ctl.PutSettings)
	r.PUT("/config", ctl.Put)
	r.GET("/account", MemberAccountsEnabled, func(c *gin.Context) { Success(c, nil) })
	_, enabled := performJSONRequest(r, "GET", "/account", "", nil)
	if enabled.Code != 200 {
		t.Fatal("accounts should initially be enabled")
	}
	_, saved := performJSONRequest(r, "PUT", "/settings", `{"spam_prompt":"keep","disable_member_accounts":true,"require_verified_user_to_post":true}`, nil)
	var cfg database.Config
	database.DB.First(&cfg)
	if saved.Code != 200 || !cfg.DisableMemberAccounts || cfg.RequireVerifiedUserToPost {
		t.Fatal("disabled accounts must allow anonymous posting")
	}
	_, denied := performJSONRequest(r, "GET", "/account", "", nil)
	if denied.Code != 403 {
		t.Fatal("member endpoint remained accessible")
	}
	performJSONRequest(r, "PUT", "/config", `{"announcement":"keep","require_verified_user_to_post":true}`, nil)
	database.DB.First(&cfg)
	if cfg.RequireVerifiedUserToPost {
		t.Fatal("legacy config route bypassed anonymous-only policy")
	}
	performJSONRequest(r, "PUT", "/settings", `{"spam_prompt":"keep","disable_member_accounts":false,"require_verified_user_to_post":true}`, nil)
	_, restored := performJSONRequest(r, "GET", "/account", "", nil)
	database.DB.First(&cfg)
	if restored.Code != 200 || !cfg.RequireVerifiedUserToPost {
		t.Fatal("re-enabling accounts failed")
	}
}
