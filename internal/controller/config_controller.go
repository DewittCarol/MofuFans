package controller

import (
	"mofufans/internal/appearance"
	"mofufans/internal/database"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type ConfigController struct{}

type ConfigRequest struct {
	Announcement              string `json:"announcement"`
	RequireVerifiedUserToPost *bool  `json:"require_verified_user_to_post"`
}

type SettingsRequest struct {
	DisableMemberAccounts     *bool                    `json:"disable_member_accounts,omitempty"`
	CursorTheme               *string                  `json:"cursor_theme,omitempty"`
	CursorThemes              []appearance.CursorTheme `json:"cursor_themes,omitempty"`
	RequireVerifiedUserToPost *bool                    `json:"require_verified_user_to_post"`
	VoyagePublicEnabled       *bool                    `json:"voyage_public_enabled,omitempty"`
	PetDefaultExpanded        *bool                    `json:"pet_default_expanded,omitempty"`
	AllowRepeatedReactions    *bool                    `json:"allow_repeated_reactions,omitempty"`
}

func (*ConfigController) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var config database.Config
	database.DB.First(&config)
	Success(c, struct {
		database.Config
		CursorThemes []appearance.CursorTheme `json:"cursor_themes"`
	}{config, appearance.CursorThemes()})
}

func (*ConfigController) Put(c *gin.Context) {
	var request ConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求错误")
		return
	}
	var config database.Config
	database.DB.First(&config)
	config.Announcement = request.Announcement
	if request.RequireVerifiedUserToPost != nil {
		config.RequireVerifiedUserToPost = *request.RequireVerifiedUserToPost
	}
	if config.DisableMemberAccounts {
		config.RequireVerifiedUserToPost = false
	}
	if err := database.DB.Model(&config).Updates(map[string]any{"announcement": config.Announcement, "require_verified_user_to_post": config.RequireVerifiedUserToPost}).Error; err != nil {
		log.Errorf("failed to save config: %v", err)
		Fail(c, 500, "内部错误")
		return
	}
	Success(c, struct {
		database.Config
		CursorThemes []appearance.CursorTheme `json:"cursor_themes"`
	}{config, appearance.CursorThemes()})
}

func (*ConfigController) GetSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var config database.Config
	if err := database.DB.First(&config).Error; err != nil {
		log.Errorf("failed to get settings: %v", err)
		Fail(c, 500, "内部错误")
		return
	}
	Success(c, SettingsRequest{
		DisableMemberAccounts:     &config.DisableMemberAccounts,
		CursorTheme:               &config.CursorTheme,
		CursorThemes:              appearance.CursorThemes(),
		RequireVerifiedUserToPost: &config.RequireVerifiedUserToPost,
		VoyagePublicEnabled:       &config.VoyagePublicEnabled,
		PetDefaultExpanded:        &config.PetDefaultExpanded,
		AllowRepeatedReactions:    &config.AllowRepeatedReactions,
	})
}

func (*ConfigController) PutSettings(c *gin.Context) {
	var request SettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求错误")
		return
	}

	var config database.Config
	if err := database.DB.First(&config).Error; err != nil {
		log.Errorf("failed to get settings: %v", err)
		Fail(c, 500, "内部错误")
		return
	}
	if request.CursorTheme != nil {
		if !appearance.ValidCursorTheme(*request.CursorTheme) {
			Fail(c, 400, "请选择有效的鼠标指针套装")
			return
		}
		config.CursorTheme = *request.CursorTheme
	}
	if request.RequireVerifiedUserToPost != nil {
		config.RequireVerifiedUserToPost = *request.RequireVerifiedUserToPost
	}
	if request.DisableMemberAccounts != nil {
		config.DisableMemberAccounts = *request.DisableMemberAccounts
	}
	if config.DisableMemberAccounts {
		config.RequireVerifiedUserToPost = false
	}
	if request.VoyagePublicEnabled != nil {
		config.VoyagePublicEnabled = *request.VoyagePublicEnabled
	}
	if request.PetDefaultExpanded != nil {
		config.PetDefaultExpanded = *request.PetDefaultExpanded
	}
	if request.AllowRepeatedReactions != nil {
		config.AllowRepeatedReactions = *request.AllowRepeatedReactions
	}
	updates := map[string]any{}
	if request.CursorTheme != nil {
		updates["cursor_theme"] = config.CursorTheme
	}
	if request.DisableMemberAccounts != nil {
		updates["disable_member_accounts"] = config.DisableMemberAccounts
	}
	if request.RequireVerifiedUserToPost != nil || config.DisableMemberAccounts {
		updates["require_verified_user_to_post"] = config.RequireVerifiedUserToPost
	}
	if request.VoyagePublicEnabled != nil {
		updates["voyage_public_enabled"] = config.VoyagePublicEnabled
	}
	if request.PetDefaultExpanded != nil {
		updates["pet_default_expanded"] = config.PetDefaultExpanded
	}
	if request.AllowRepeatedReactions != nil {
		updates["allow_repeated_reactions"] = config.AllowRepeatedReactions
	}
	if err := database.DB.Model(&config).Updates(updates).Error; err != nil {
		log.Errorf("failed to save settings: %v", err)
		Fail(c, 500, "内部错误")
		return
	}
	Success(c, SettingsRequest{
		DisableMemberAccounts:     &config.DisableMemberAccounts,
		CursorTheme:               &config.CursorTheme,
		CursorThemes:              appearance.CursorThemes(),
		RequireVerifiedUserToPost: &config.RequireVerifiedUserToPost,
		VoyagePublicEnabled:       &config.VoyagePublicEnabled,
		PetDefaultExpanded:        &config.PetDefaultExpanded,
		AllowRepeatedReactions:    &config.AllowRepeatedReactions,
	})
}

func (*ConfigController) PutAppearance(c *gin.Context) {
	var request struct {
		CursorTheme string `json:"cursor_theme"`
	}
	if !installationBody(c, &request) {
		return
	}
	if !appearance.ValidCursorTheme(request.CursorTheme) {
		Fail(c, 400, "请选择有效的鼠标指针套装")
		return
	}
	var config database.Config
	if database.DB.First(&config).Error != nil || database.DB.Model(&config).Update("cursor_theme", request.CursorTheme).Error != nil {
		Fail(c, 500, "鼠标样式保存失败")
		return
	}
	Success(c, gin.H{"cursor_theme": request.CursorTheme})
}
