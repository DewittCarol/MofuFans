package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"mofufans/internal/guard"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func realtimeRig(t *testing.T) (*httptest.Server, *guard.Manager) {
	t.Helper()
	g, err := guard.New(guard.Options{Directory: t.TempDir(), Secret: strings.Repeat("r", 32)})
	if err != nil {
		t.Fatal(err)
	}
	ctl := NewQuestionController()
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(g.Middleware(nil))
	r.GET("/api/ws", ctl.WebSocket)
	r.GET("/api/sse", ctl.SSE)
	server := httptest.NewServer(r)
	t.Cleanup(func() { server.Close(); g.Close(); close(ctl.eventChan) })
	return server, g
}
func waitStreams(t *testing.T, g *guard.Manager, want int) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if g.View().Connections == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("stream reservations %d, want %d", g.View().Connections, want)
}
func dialRealtime(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var hello map[string]string
	if err = conn.ReadJSON(&hello); err != nil || hello["type"] != "connected" {
		t.Fatal(hello, err)
	}
	return conn
}
func TestRealtimeOriginAndCursorPayloadIsolation(t *testing.T) {
	server, g := realtimeRig(t)
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/ws", http.Header{"Origin": []string{"https://other.example"}})
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatal("foreign browser origin admitted")
	}
	response.Body.Close()
	a, b := dialRealtime(t, server), dialRealtime(t, server)
	if err = a.WriteJSON(map[string]any{"type": "cursor", "cardId": 1, "x": 12.5, "y": 30, "secret": "must-not-broadcast"}); err != nil {
		t.Fatal(err)
	}
	_, payload, err := b.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "must-not-broadcast") {
		t.Fatal("arbitrary fields relayed")
	}
	var msg map[string]any
	if json.Unmarshal(payload, &msg) != nil || msg["type"] != "cursor" || msg["x"] != 12.5 {
		t.Fatal(string(payload))
	}
	a.Close()
	b.Close()
	waitStreams(t, g, 0)
}
func TestRealtimeOversizedAndInvalidCoordinatesCloseConnection(t *testing.T) {
	server, g := realtimeRig(t)
	for _, payload := range []string{strings.Repeat("x", 2048), `{"type":"cursor","cardId":1,"x":1000000000,"y":0}`} {
		conn := dialRealtime(t, server)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := conn.ReadMessage(); err == nil {
			t.Fatal("invalid message connection remained open")
		}
		conn.Close()
		waitStreams(t, g, 0)
	}
}
func TestRealtimeSSEFlushesAndReleasesCancelledConnection(t *testing.T) {
	server, g := realtimeRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/sse", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "connected") {
		t.Fatal(line, err)
	}
	if response.Header.Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("SSE wildcard origin remains")
	}
	cancel()
	response.Body.Close()
	waitStreams(t, g, 0)
}
