package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/spf13/viper"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLocalAssetsPathsAtomicityAndRange(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	a := NewAssets("gallery", t.TempDir())
	ctx := context.Background()
	for _, name := range []string{"../secret", "/secret", "a/b", "a\\b", "..", ".hidden", ""} {
		if err := a.Put(ctx, name, strings.NewReader("x")); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := a.Put(ctx, "photo.png", strings.NewReader("0123456789")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/photo.png", nil)
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	a.Serve(w, req, "photo.png")
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("bad range response: %d %s", w.Code, w.Body)
	}
	if err := a.Put(ctx, "photo.png", brokenReader{}); err == nil {
		t.Fatal("failed write accepted")
	}
	data, _ := os.ReadFile(filepath.Join(a.Directory, "photo.png"))
	if string(data) != "0123456789" {
		t.Fatal("failed upload replaced existing data")
	}
	if err := a.RollbackFile(ctx, "photo.png", filepath.Join(a.Directory, "photo.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Directory, "photo.png")); err != nil {
		t.Fatal("rollback discarded local staging required for retry")
	}
	if err := a.Delete(ctx, "photo.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Stat(ctx, "photo.png"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestOSSPrivateObjectsNamespacesRangeAndDelete(t *testing.T) {
	viper.Reset()
	viper.Set("oss.prefix", "site-one")
	t.Cleanup(viper.Reset)
	objects := map[string][]byte{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "OSS4-HMAC-SHA256") {
			t.Error("missing V4 signature")
		}
		key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
		if r.Method == "GET" && r.URL.Query().Has("prefix") {
			type item struct {
				Key          string
				Size         int
				LastModified string
			}
			out := struct {
				XMLName     xml.Name `xml:"ListBucketResult"`
				IsTruncated bool
				Contents    []item
			}{}
			for name, data := range objects {
				if strings.HasPrefix(name, r.URL.Query().Get("prefix")) {
					out.Contents = append(out.Contents, item{name, len(data), "2026-01-01T00:00:00.000Z"})
				}
			}
			_ = xml.NewEncoder(w).Encode(out)
			return
		}
		switch r.Method {
		case "PUT":
			if r.Header.Get("x-oss-object-acl") != "private" {
				t.Error("upload did not force private ACL")
			}
			objects[key], _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"fake"`)
		case "DELETE":
			delete(objects, key)
			w.WriteHeader(204)
		case "HEAD", "GET":
			data, ok := objects[key]
			if !ok {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, key, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(data))
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	client, err := oss.New(server.URL, "test-id", "test-secret", oss.AuthVersion(oss.AuthV4), oss.Region("cn-hangzhou"), oss.ForcePathStyle(true))
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := client.Bucket("test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"uploads", "backgrounds", "branding", "emojis", "gallery", "voyage", "digital-gifts"} {
		t.Run(namespace, func(t *testing.T) {
			a := &Assets{Namespace: namespace, bucket: bucket}
			ctx := context.Background()
			payload := strings.Repeat(namespace, 1024)
			if err := a.Put(ctx, "sample.bin", strings.NewReader(payload)); err != nil {
				t.Fatal(err)
			}
			info, err := a.Stat(ctx, "sample.bin")
			if err != nil || info.Size() != int64(len(payload)) {
				t.Fatalf("stat: %v %v", info, err)
			}
			list, err := a.List(ctx)
			if err != nil || len(list) != 1 {
				t.Fatalf("list: %v %v", list, err)
			}
			for _, method := range []string{"GET", "HEAD"} {
				req := httptest.NewRequest(method, "/download", nil)
				req.Header.Set("Range", "bytes=7-30")
				w := httptest.NewRecorder()
				a.Serve(w, req, "sample.bin")
				if w.Code != 206 || w.Header().Get("Content-Length") != strconv.Itoa(24) {
					t.Fatalf("%s range: %d %s", method, w.Code, w.Body)
				}
				if method == "GET" && w.Body.String() != payload[7:31] {
					t.Fatal("incorrect range content")
				}
				if method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("HEAD returned bytes")
				}
			}
			req := httptest.NewRequest("GET", "/download", nil)
			req.Header.Set("Range", "bytes=999999-")
			w := httptest.NewRecorder()
			a.Serve(w, req, "sample.bin")
			if w.Code != 416 {
				t.Fatalf("range beyond file: %d", w.Code)
			}
			if err := a.Delete(ctx, "sample.bin"); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Stat(ctx, "sample.bin"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("deleted file remained: %v", err)
			}
		})
	}
	if len(objects) != 0 {
		t.Fatal("object deletion leaked across namespaces")
	}
}

func TestStorageConfigurationFailsClosed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, kind := range []string{"", "typo", "OSS"} {
		viper.Set("storage_type", kind)
		if Validate() == nil {
			t.Fatal("unknown storage accepted")
		}
	}
	viper.Set("storage_type", "oss")
	viper.Set("oss.endpoint", "http://example.com")
	if Validate() == nil {
		t.Fatal("insecure or incomplete OSS configuration accepted")
	}
	a := NewAssets("gallery", "")
	viper.Set("oss.prefix", "../other-site")
	if _, err := a.key("x.png"); err == nil {
		t.Fatal("cross-prefix write accepted")
	}
}
