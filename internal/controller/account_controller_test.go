package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeAvatarStorage struct{}

func (fakeAvatarStorage) Save(context.Context, int64, string) (string, error) {
	return "/upload-img/test-avatar.jpg", nil
}

func (fakeAvatarStorage) Delete(string) error { return nil }

// Existing authentication fixtures must stay independent of live Bilibili.
type unavailableLoginProfileReader struct{}

func (unavailableLoginProfileReader) PublicSignature(context.Context, int64) (bilibili.Profile, error) {
	return bilibili.Profile{}, errors.New("profile lookup unavailable in this fixture")
}

type testAPIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func performJSONRequest(router http.Handler, method, path, body string, cookies []*http.Cookie) (*httptest.ResponseRecorder, testAPIResponse) {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	for _, item := range cookies {
		request.AddCookie(item)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var response testAPIResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder, response
}

func TestAccountVerificationRegistrationAndDisabledLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var err error
	database.DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "account.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := database.DB.AutoMigrate(&database.User{}, &database.BilibiliVerificationAccount{}, &database.BilibiliVerificationRequest{}); err != nil {
		t.Fatal(err)
	}
	controller := &AccountController{AvatarStorage: fakeAvatarStorage{}}
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-session-secret-that-is-long-enough"))))
	router.POST("/verification", controller.StartVerification)
	router.POST("/register", controller.Register)
	router.POST("/login", controller.Login)

	_, startResponse := performJSONRequest(router, http.MethodPost, "/verification", `{"bilibili_uid":"123456"}`, nil)
	if startResponse.Code != 410 {
		t.Fatalf("retired verification must fail: %+v", startResponse)
	}
	now := time.Now().UTC()
	confirmationUntil := now.Add(10 * time.Minute)
	database.DB.Create(&database.BilibiliVerificationRequest{BilibiliUID: 123456, CredentialHash: "legacy", Status: database.VerificationVerified, RequestedAt: now, ExpiresAt: confirmationUntil, VerifiedAt: &now, ConfirmationUntil: &confirmationUntil})

	_, registerResponse := performJSONRequest(router, http.MethodPost, "/register", `{"username":"test-user","password":"strong-password"}`, nil)
	if registerResponse.Code != 410 {
		t.Fatalf("old verified follow requests cannot register: %+v", registerResponse)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	database.DB.Create(&database.User{BilibiliUID: 123456, Username: "test-user", PasswordHash: string(hash), IsDisabled: true, VerifiedAt: now})
	_, loginResponse := performJSONRequest(router, http.MethodPost, "/login", `{"username":"test-user","password":"strong-password"}`, nil)
	if loginResponse.Code != 403 {
		t.Fatalf("disabled login should fail, got %+v", loginResponse)
	}
}
