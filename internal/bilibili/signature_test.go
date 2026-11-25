package bilibili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPublicSignatureFreshNoCookiesAndExactUID(t *testing.T) {
	value := "Bird or Cage"
	mid := "10001"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Cookie") != "" {
			t.Error("signature request leaked a login cookie")
		}
		if r.Header.Get("Cache-Control") != "no-cache" || r.URL.Path != "/x/web-interface/card" || r.URL.Query().Get("mid") != "10001" {
			t.Error("unexpected signature request")
		}
		fmt.Fprintf(w, `{"code":0,"data":{"card":{"mid":"%s","name":"测试资料","face":"","sign":"%s"}}}`, mid, value)
	}))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(server.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "SESSDATA", Value: "must-not-leak"}})
	httpClient := server.Client()
	httpClient.Jar = jar
	client := &Client{BaseURL: server.URL, HTTPClient: httpClient}
	p, err := client.PublicSignature(context.Background(), 10001)
	if err != nil || p.Signature != value {
		t.Fatal(p, err)
	}
	value = "Bird or Cage123"
	p, err = client.PublicSignature(context.Background(), 10001)
	if err != nil || p.Signature != value || calls != 2 {
		t.Fatal("lookup used stale data", p, err, calls)
	}
	mid = "610010"
	if _, err = client.PublicSignature(context.Background(), 10001); err == nil {
		t.Fatal("wrong UID accepted")
	}
}

func TestPublicSignatureFailsClosed(t *testing.T) {
	for _, body := range []string{`{"code":-412,"data":{"card":{"mid":"10001","name":"a","sign":"x"}}}`, `{"data":{"card":{"mid":"10001","name":"a","sign":"x"}}}`, `{"code":0,"data":{"card":{"mid":"10001","name":"a"}}}`, `<html>verification</html>`, `{"code":0,"data":{"card":{"mid":"10001","name":"a","sign":"` + strings.Repeat("a", 4097) + `"}}}`} {
		t.Run(fmt.Sprintf("case-%d", len(body)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
			if _, err := client.PublicSignature(context.Background(), 10001); err == nil {
				t.Fatal("invalid payload accepted")
			}
		})
	}
}
