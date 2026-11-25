package bilibili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicCardNoCookieAndUIDValidation(t *testing.T) {
	mid := "1000001"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("profile lookup sent a cookie")
		}
		if r.URL.Path != "/x/web-interface/card" || r.URL.Query().Get("mid") != "1000001" {
			t.Error("wrong profile request")
		}
		fmt.Fprintf(w, `{"code":0,"data":{"card":{"mid":"%s","name":"公开昵称","face":"https://i1.hdslb.com/a.png"}}}`, mid)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if p, err := client.PublicCard(context.Background(), 1000001); err != nil || p.Name != "公开昵称" {
		t.Fatal(p, err)
	}
	mid = "1000002"
	if _, err := client.PublicCard(context.Background(), 1000001); err == nil {
		t.Fatal("mismatched user accepted")
	}
}
