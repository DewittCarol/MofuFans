package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestVoyageAssetLibraryPaginationAndRetainedUploads(t *testing.T) {
	directory := t.TempDir()
	controller := &VoyageController{Directory: directory}
	router := gin.New()
	router.GET("/assets", controller.Assets)
	// No issue references: unused uploads must still be reusable after reopening the editor.
	stamp := time.Unix(1700000000, 0)
	for i := 0; i < 27; i++ {
		name := fmt.Sprintf("%064x.png", i)
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("image fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "private.json"), []byte("not an asset"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, strings.Repeat("e", 64)+".png"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "private.json"), filepath.Join(directory, strings.Repeat("f", 64)+".png")); err != nil {
		t.Fatal(err)
	}
	call := func(query string) (int, []string, int, int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/assets"+query, nil))
		var body struct {
			Code int
			Data struct {
				Images   []string
				Total    int
				Page     int
				PageSize int `json:"page_size"`
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code == 200 && (w.Header().Get("Cache-Control") != "private, no-store" || body.Data.PageSize != 24) {
			t.Fatalf("bad pagination/cache: %s", w.Body.String())
		}
		return body.Code, body.Data.Images, body.Data.Total, body.Data.Page
	}
	code, first, total, page := call("")
	if code != 200 || total != 27 || page != 1 || len(first) != 24 || first[0] != "/api/voyage-assets/"+fmt.Sprintf("%064x.png", 0) {
		t.Fatalf("bad first page: %d %d %d %v", code, total, page, first)
	}
	code, second, total, page := call("?page=2")
	if code != 200 || total != 27 || page != 2 || len(second) != 3 || second[0] == first[0] {
		t.Fatal("second page overlap or missing retained files")
	}
	_, clamped, _, page := call("?page=999")
	if page != 2 || len(clamped) != 3 {
		t.Fatal("out of range page was not clamped")
	}
	for _, value := range []string{"0", "-1", "abc", "1000001"} {
		if code, _, _, _ := call("?page=" + value); code != 400 {
			t.Fatal("invalid page accepted")
		}
	}
	controller.Directory = filepath.Join(directory, "not-created-yet")
	code, empty, total, page := call("")
	if code != 200 || empty == nil || len(empty) != 0 || total != 0 || page != 1 {
		t.Fatal("new library should be an empty page")
	}
}
