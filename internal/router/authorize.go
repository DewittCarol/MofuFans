package router

import (
	"mofufans/internal/adminauth"
	"mofufans/internal/database"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// voyageAccessMiddleware exposes only visitor routes; management keeps authMiddleware.
func voyageAccessMiddleware(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if admin, ok := adminauth.Current(c); ok {
		c.Set("user", admin)
		c.Set("authed", true)
		c.Next()
		return
	}
	var config database.Config
	if database.DB.Select("voyage_public_enabled").First(&config).Error != nil {
		c.AbortWithStatusJSON(200, gin.H{"code": 503, "message": "舰礼暂时不可用"})
		return
	}
	if !config.VoyagePublicEnabled {
		c.AbortWithStatusJSON(200, gin.H{"code": 403, "message": "舰礼暂未开放"})
		return
	}
	c.Next()
}

func authMiddleware(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	user, ok := adminauth.Current(c)
	if !ok {
		session := sessions.Default(c)
		session.Delete("user")
		session.Delete("authed")
		session.Delete("auth_stamp")
		_ = session.Save()
		c.AbortWithStatusJSON(200, gin.H{"code": 408, "message": "请先登录"})
		return
	}
	c.Set("user", user)
	c.Set("authed", true)
	c.Next()
}
