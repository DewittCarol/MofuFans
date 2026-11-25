package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/spf13/viper"
)

func TestMultipartUploadCommitAndFailurePreserveStaging(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			payload := bytes.Repeat([]byte("large-gift"), 2<<20)
			source := filepath.Join(t.TempDir(), "staged.bin")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			parts := map[int][]byte{}
			var completed, aborted bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.Method == "POST" && r.URL.Query().Has("uploads"):
					if r.Header.Get("x-oss-object-acl") != "private" {
						t.Error("multipart upload must be private")
					}
					_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><Bucket>test-bucket</Bucket><Key>digital-gifts/large.bin</Key><UploadId>test-upload</UploadId></InitiateMultipartUploadResult>`)
				case r.Method == "PUT":
					number, _ := strconv.Atoi(r.URL.Query().Get("partNumber"))
					if fail && number == 2 {
						w.WriteHeader(403)
						_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>simulated part failure</Message></Error>`)
						return
					}
					parts[number], _ = io.ReadAll(r.Body)
					w.Header().Set("ETag", `"part-`+strconv.Itoa(number)+`"`)
				case r.Method == "POST" && r.URL.Query().Has("uploadId"):
					var body struct {
						Parts []struct{ PartNumber int } `xml:"Part"`
					}
					if err := xml.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Parts) != 2 {
						t.Errorf("bad completion manifest: %v", err)
					}
					if !bytes.Equal(append(parts[1], parts[2]...), payload) {
						t.Error("multipart contents differ from staged file")
					}
					completed = true
					_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><Bucket>test-bucket</Bucket><Key>digital-gifts/large.bin</Key><ETag>complete</ETag></CompleteMultipartUploadResult>`)
				case r.Method == "DELETE":
					aborted = true
					w.WriteHeader(204)
				default:
					w.WriteHeader(405)
				}
			}))
			defer server.Close()
			client, err := oss.New(server.URL, "test-id", "test-secret", oss.AuthVersion(oss.AuthV4), oss.Region("cn-hangzhou"), oss.ForcePathStyle(true))
			if err != nil {
				t.Fatal(err)
			}
			bucket, _ := client.Bucket("test-bucket")
			a := NewOSSAssets("digital-gifts", bucket)
			err = a.PutFile(context.Background(), "large.bin", source)
			if (err != nil) != fail {
				t.Fatalf("failure=%v, upload error=%v", fail, err)
			}
			mu.Lock()
			if completed == fail || aborted != fail {
				t.Errorf("completed=%v aborted=%v failure=%v", completed, aborted, fail)
			}
			mu.Unlock()
			if _, err := os.Stat(source); err != nil {
				t.Fatal("staging removed before database commit")
			}
			if !fail {
				a.ReleaseStaging(source)
				if _, err := os.Stat(source); !os.IsNotExist(err) {
					t.Fatal("committed staging was not removed")
				}
			}
		})
	}
}
