package controller

import (
	"fmt"
	"mofufans/internal/adminauth"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type UserController struct{}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserModifyRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

type UserAddRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

func (*UserController) Info(c *gin.Context) {
	Success(c, c.MustGet("user"))
}

func (*UserController) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request LoginRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&request) != nil || len(request.Username) > 200 {
		Fail(c, 400, "请求无效")
		return
	}
	var user database.Admin
	found := database.DB.Where("username = ?", request.Username).First(&user).Error == nil
	identity := "name:" + strings.ToLower(strings.TrimSpace(request.Username))
	if found {
		identity = fmt.Sprintf("admin:%d", user.ID)
	}
	a, ok := guard.BeginAuth(c, "admin-login", identity)
	if !ok {
		return
	}
	defer a.Cancel()
	valid, legacy := adminauth.VerifyPassword(user.Password, request.Password)
	if !found || !valid {
		a.Failure()
		log.WithFields(log.Fields{"ip": c.ClientIP(), "result": "denied"}).Warn("admin login")
		Fail(c, 401, "用户名或密码错误")
		return
	}
	if legacy {
		hashed, err := adminauth.HashPassword(request.Password)
		if err != nil {
			Fail(c, 500, "内部错误")
			return
		}
		old := user.Password
		result := database.DB.Model(&user).Where("password = ?", old).Update("password", hashed)
		if result.Error != nil || result.RowsAffected != 1 {
			Fail(c, 401, "请重新登录")
			return
		}
		user.Password = hashed
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("user", user.ID)
	session.Set("authed", true)
	session.Set("auth_stamp", adminauth.Stamp(user))
	session.Set("reauth_stamp", adminauth.Stamp(user))
	session.Set("reauth_until", time.Now().Add(guard.ReauthLifetime).Unix())
	if session.Save() != nil {
		Fail(c, 500, "内部错误")
		return
	}
	a.Success()
	log.WithFields(log.Fields{"admin_id": user.ID, "ip": c.ClientIP(), "result": "accepted"}).Info("admin login")
	Success(c, nil)
}

func (*UserController) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Delete("user")
	session.Delete("authed")
	session.Delete("auth_stamp")
	session.Delete("reauth_stamp")
	session.Delete("reauth_until")
	err := session.Save()
	if err != nil {
		Fail(c, 500, "内部错误")
		return
	}
	Success(c, nil)
}

func (*UserController) Get(c *gin.Context) {
	var users []database.Admin
	database.DB.Find(&users)
	Success(c, users)
}

func (*UserController) Put(c *gin.Context) {
	userModel := c.MustGet("user").(database.Admin)
	if !userModel.IsSuperAdmin {
		Fail(c, 403, "没有权限")
		return
	}
	var user database.Admin
	database.DB.First(&user, "id = ?", c.Param("id"))
	if user.ID == 0 {
		Fail(c, 404, "用户不存在")
		return
	}
	var request UserModifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求无效")
		return
	}
	if request.Username != "" {
		if len(strings.TrimSpace(request.Username)) > 200 {
			Fail(c, 400, "用户名过长")
			return
		}
		user.Username = strings.TrimSpace(request.Username)
	}
	if request.Password != "" {
		Fail(c, 400, "请使用新版后台设置新密码")
		return
	}
	if request.NewPassword != "" {
		hashed, err := adminauth.HashNewPassword(request.NewPassword)
		if err != nil {
			Fail(c, 400, err.Error())
			return
		}
		user.Password = hashed
	}
	if err := database.DB.Save(&user).Error; err != nil {
		log.Errorf("failed to save user: %v", err)
		Fail(c, 500, "内部错误")
		return
	}
	Success(c, user)
}

func (*UserController) Post(c *gin.Context) {
	userModel := c.MustGet("user").(database.Admin)
	if !userModel.IsSuperAdmin {
		Fail(c, 403, "权限不足")
		return
	}
	var request UserAddRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求无效")
		return
	}
	var user database.Admin
	user.Username = strings.TrimSpace(request.Username)
	if user.Username == "" || len(user.Username) > 200 || request.Password != "" {
		Fail(c, 400, "请使用新版后台填写有效的用户名与密码")
		return
	}
	hashed, err := adminauth.HashNewPassword(request.NewPassword)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	user.Password = hashed
	if err := database.DB.Create(&user).Error; err != nil {
		log.Errorf("failed to create user: %v", err)
		Fail(c, 501, "创建用户失败")
		return
	}
	Success(c, user)
}

func (*UserController) Delete(c *gin.Context) {
	userModel := c.MustGet("user").(database.Admin)
	if !userModel.IsSuperAdmin {
		Fail(c, 403, "没有权限")
		return
	}
	var user database.Admin
	database.DB.First(&user, "id = ?", c.Param("id"))
	if user.ID == 0 {
		Fail(c, 404, "用户不存在")
		return
	}
	if user.IsSuperAdmin {
		Fail(c, 403, "不能删除默认管理员")
		return
	}
	if err := database.DB.Delete(&user).Error; err != nil {
		log.Errorf("failed to delete user: %v", err)
		Fail(c, 502, "删除用户失败")
		return
	}
	Success(c, nil)
}
