package controller

import (
	"bytes"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"io"
	"mofufans/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOSSStorageDoesNotBypassGiftAuthorization(t *testing.T) {
	router, ctl := giftTestRouter(t)
	objects := map[string][]byte{}
	downloads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			if r.Header.Get("x-oss-object-acl") != "private" {
				t.Error("gift object is not private")
			}
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case "HEAD", "GET":
			data, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			downloads++
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, "gift", time.Now(), bytes.NewReader(data))
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	client, err := oss.New(server.URL, "test-id", "test-secret", oss.ForcePathStyle(true), oss.AuthVersion(oss.AuthV4), oss.Region("cn-hangzhou"))
	if err != nil {
		t.Fatal(err)
	}
	bucket, _ := client.Bucket("test-bucket")
	ctl.Files = storage.NewOSSAssets("digital-gifts", bucket)
	gift := giftCreate(t, router, "shared", true)
	password := giftPassword(t, router, gift.ID, "")
	file := giftUpload(t, router, gift.ID, "gift.txt", "private gift content")
	if len(objects) != 1 {
		t.Fatal("upload did not reach OSS")
	}
	entries, _ := os.ReadDir(ctl.Directory)
	if len(entries) != 0 {
		t.Fatal("completed upload left a local copy")
	}
	base := fmt.Sprintf("/api/gifts/%d", gift.ID)
	download := fmt.Sprintf("%s/files/%d", base, file.ID)
	req := httptest.NewRequest("GET", download, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 401 || downloads != 0 {
		t.Fatal("unauthorized request touched OSS")
	}
	unlocked, _ := giftJSON(t, router, "POST", base+"/unlock", map[string]string{"password": password}, nil, 200)
	req = httptest.NewRequest("GET", download, nil)
	req.AddCookie(unlocked.Result().Cookies()[0])
	req.Header.Set("Range", "bytes=8-11")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "gift" || w.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("broken private range: %d %s", w.Code, w.Body)
	}
	giftJSON(t, router, "DELETE", fmt.Sprintf("/api/admin/gifts/%d/files/%d", gift.ID, file.ID), nil, nil, 200)
	if len(objects) != 0 {
		t.Fatal("deleted gift file remained in OSS")
	}
}
