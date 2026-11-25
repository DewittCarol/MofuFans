package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mofufans/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func websiteAssetTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	var err error
	database.DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "website-assets.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB.AutoMigrate(&database.Admin{}, &database.WebsiteAssetRequest{}, &database.WebsiteAssetFile{}, &database.WebsiteAssetSettings{}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ctl := &WebsiteAssetController{Directory: directory}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Actor") == "owner" {
			c.Set("user", database.Admin{BaseModel: database.BaseModel{ID: 1}, Username: "owner", IsSuperAdmin: true})
		} else {
			c.Set("user", database.Admin{BaseModel: database.BaseModel{ID: 2}, Username: "streamer"})
		}
		c.Next()
	})
	router.GET("/website-assets", ctl.List)
	router.POST("/website-assets", ctl.Create)
	router.PUT("/website-assets/requests/:id", ctl.Update)
	router.DELETE("/website-assets/requests/:id", ctl.Delete)
	router.GET("/website-assets/files/:id", ctl.Download)
	router.GET("/website-assets/settings", ctl.GetSettings)
	router.PUT("/website-assets/settings", ctl.PutSettings)
	return router, directory
}

func uploadWebsiteAsset(t *testing.T, router http.Handler, filename string, content []byte) testAPIResponse {
	t.Helper()
	return postWebsiteRequest(t, router, "用于网站桌宠，保留透明背景", map[string][]byte{filename: content})
}

func postWebsiteRequest(t *testing.T, router http.Handler, description string, files map[string][]byte) testAPIResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("description", description)
	for filename, content := range files {
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(content)
	}
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/website-assets", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var response testAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func performWebsiteAssetJSON(router http.Handler, method, target, body, actor string) testAPIResponse {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if actor != "" {
		request.Header.Set("X-Test-Actor", actor)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var response testAPIResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return response
}

func TestWebsiteAssetHandoffPermissionsAndPrivateFiles(t *testing.T) {
	router, directory := websiteAssetTestRouter(t)
	content := []byte("arbitrary-vrm-payload")
	response := uploadWebsiteAsset(t, router, `..\models\桌宠.vrm`, content)
	if response.Code != 200 {
		t.Fatalf("upload failed: %+v", response)
	}
	var created database.WebsiteAssetRequest
	if err := json.Unmarshal(response.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.SubmittedByName != "streamer" || created.Status != database.WebsiteAssetPending || len(created.Files) != 1 || created.Files[0].OriginalName != "桌宠.vrm" {
		t.Fatalf("unexpected upload metadata: %+v", created)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || len(entries[0].Name()) != 40 {
		t.Fatalf("private storage did not use an opaque name: %v %+v", err, entries)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/website-assets/files/%d", created.Files[0].ID), nil))
	if download.Code != 200 || !bytes.Equal(download.Body.Bytes(), content) || download.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("private download failed: %d %v %s", download.Code, download.Header(), download.Body.String())
	}

	forbidden := performWebsiteAssetJSON(router, http.MethodPut, fmt.Sprintf("/website-assets/requests/%d", created.ID), `{"status":"completed"}`, "")
	if forbidden.Code != 403 {
		t.Fatalf("normal administrator changed completion status: %+v", forbidden)
	}
	completed := performWebsiteAssetJSON(router, http.MethodPut, fmt.Sprintf("/website-assets/requests/%d", created.ID), `{"status":"completed"}`, "owner")
	if completed.Code != 200 {
		t.Fatalf("owner could not complete request: %+v", completed)
	}
	var saved database.WebsiteAssetRequest
	if err := database.DB.First(&saved, created.ID).Error; err != nil || saved.Status != database.WebsiteAssetCompleted || saved.CompletedAt == nil || saved.CompletedByName != "owner" {
		t.Fatalf("completion was not recorded: %+v %v", saved, err)
	}

	settingsDenied := performWebsiteAssetJSON(router, http.MethodPut, "/website-assets/settings", `{"instructions":"new instructions"}`, "")
	if settingsDenied.Code != 403 {
		t.Fatalf("normal administrator edited settings: %+v", settingsDenied)
	}
	settingsSaved := performWebsiteAssetJSON(router, http.MethodPut, "/website-assets/settings", `{"instructions":"先上传原文件，再说明放置位置。"}`, "owner")
	if settingsSaved.Code != 200 {
		t.Fatalf("owner could not edit instructions: %+v", settingsSaved)
	}

	deleted := performWebsiteAssetJSON(router, http.MethodDelete, fmt.Sprintf("/website-assets/requests/%d", created.ID), "", "owner")
	if deleted.Code != 200 {
		t.Fatalf("owner could not delete request: %+v", deleted)
	}
	entries, _ = os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("deleting a request left private files behind")
	}
}

func TestWebsiteAssetUploadValidationAndOwnership(t *testing.T) {
	router, _ := websiteAssetTestRouter(t)
	if response := uploadWebsiteAsset(t, router, "empty.zip", nil); response.Code != 400 {
		t.Fatalf("empty file accepted: %+v", response)
	}
	response := uploadWebsiteAsset(t, router, "cursor.zip", []byte("zip payload"))
	var created database.WebsiteAssetRequest
	_ = json.Unmarshal(response.Data, &created)
	updated := performWebsiteAssetJSON(router, http.MethodPut, fmt.Sprintf("/website-assets/requests/%d", created.ID), `{"description":"管理员补充后的用途"}`, "owner")
	if updated.Code != 200 {
		t.Fatalf("owner could not edit another request: %+v", updated)
	}
	invalid := performWebsiteAssetJSON(router, http.MethodPut, fmt.Sprintf("/website-assets/requests/%d", created.ID), `{"description":"   "}`, "")
	if invalid.Code != 400 {
		t.Fatalf("empty description accepted: %+v", invalid)
	}
}

func TestWebsiteRequestWithoutAttachments(t *testing.T) {
	router, directory := websiteAssetTestRouter(t)
	description := "手机上点不开歌单菜单，希望修复后能正常筛选。"
	response := postWebsiteRequest(t, router, "  "+description+"\n", nil)
	if response.Code != 200 {
		t.Fatalf("text-only request failed: %+v", response)
	}
	var created database.WebsiteAssetRequest
	if err := json.Unmarshal(response.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Description != description || created.Status != database.WebsiteAssetPending || created.SubmittedByName != "streamer" || created.Files == nil || len(created.Files) != 0 {
		t.Fatalf("unexpected text-only request: %+v", created)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("text-only request wrote files: %v %v", entries, err)
	}
	listed := performWebsiteAssetJSON(router, http.MethodGet, "/website-assets?status=pending", "", "")
	var list websiteAssetList
	if err := json.Unmarshal(listed.Data, &list); err != nil || listed.Code != 200 || list.PendingCount != 1 || len(list.Items) != 1 || list.Items[0].ID != created.ID || list.Items[0].Files == nil {
		t.Fatalf("text-only request missing from queue: %+v %v", list, err)
	}
	endpoint := fmt.Sprintf("/website-assets/requests/%d", created.ID)
	updated := performWebsiteAssetJSON(router, http.MethodPut, endpoint, `{"description":"补充：手机横屏时也会出现这个问题。"}`, "")
	if updated.Code != 200 {
		t.Fatalf("author could not update text-only request: %+v", updated)
	}
	denied := performWebsiteAssetJSON(router, http.MethodPut, endpoint, `{"status":"completed"}`, "")
	if denied.Code != 403 {
		t.Fatalf("text-only request bypassed status permission: %+v", denied)
	}
	for _, status := range []string{"completed", "pending"} {
		response := performWebsiteAssetJSON(router, http.MethodPut, endpoint, fmt.Sprintf(`{"status":%q}`, status), "owner")
		var saved database.WebsiteAssetRequest
		if err := json.Unmarshal(response.Data, &saved); err != nil || response.Code != 200 || saved.Status != status || (saved.CompletedAt != nil) != (status == "completed") || len(saved.Files) != 0 {
			t.Fatalf("could not change text-only status to %s: %+v %v", status, saved, err)
		}
	}
	deleted := performWebsiteAssetJSON(router, http.MethodDelete, endpoint, "", "owner")
	if deleted.Code != 200 {
		t.Fatalf("could not delete text-only request: %+v", deleted)
	}
	var remaining int64
	database.DB.Model(&database.WebsiteAssetRequest{}).Where("id = ?", created.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatal("deleted text-only request still exists")
	}
}

func TestWebsiteRequestDescriptionAndAttachmentLimits(t *testing.T) {
	router, _ := websiteAssetTestRouter(t)
	for _, description := range []string{"", " \t\n", strings.Repeat("字", 4001)} {
		for _, files := range []map[string][]byte{nil, {"screen.png": []byte("screenshot")}} {
			if response := postWebsiteRequest(t, router, description, files); response.Code != 400 {
				t.Fatalf("invalid description accepted: %+v", response)
			}
		}
	}
	if response := postWebsiteRequest(t, router, strings.Repeat("字", 4000), nil); response.Code != 200 {
		t.Fatalf("maximum-length text-only request rejected: %+v", response)
	}
	files := map[string][]byte{}
	for i := 0; i < websiteAssetMaxFiles; i++ {
		files[fmt.Sprintf("file-%d.txt", i)] = []byte("attachment")
	}
	if response := postWebsiteRequest(t, router, "新增内容", files); response.Code != 200 {
		t.Fatalf("eight attachments rejected: %+v", response)
	}
	files["extra.txt"] = []byte("extra")
	if response := postWebsiteRequest(t, router, "新增内容", files); response.Code != 400 {
		t.Fatalf("more than eight attachments accepted: %+v", response)
	}
}
