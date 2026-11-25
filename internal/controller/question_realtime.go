package controller

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"mofufans/internal/guard"
	"mofufans/internal/siteconfig"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"
)

type wsPeer struct {
	conn *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

func (p *wsPeer) close() { p.once.Do(func() { close(p.done); _ = p.conn.Close() }) }
func (p *wsPeer) offer(message []byte) {
	select {
	case <-p.done:
		return
	default:
	}
	select {
	case p.send <- message:
	default:
		p.close()
	}
}
func (p *wsPeer) write() {
	defer p.close()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.done:
			return
		case message := <-p.send:
			_ = p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if p.conn.WriteMessage(websocket.TextMessage, message) != nil {
				return
			}
		case <-tick.C:
			if p.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
				return
			}
		}
	}
}
func (ctl *QuestionController) SSE(c *gin.Context) {
	release, ok := guard.AcquireStream(c)
	if !ok {
		return
	}
	defer release()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Accel-Buffering", "no")
	client := make(chan SSEvent, 64)
	ctl.clientsMutex.Lock()
	ctl.clients[client] = true
	ctl.clientsMutex.Unlock()
	defer func() {
		ctl.clientsMutex.Lock()
		if ctl.clients[client] {
			delete(ctl.clients, client)
			close(client)
		}
		ctl.clientsMutex.Unlock()
	}()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	write := func(event sse.Event) bool {
		control := http.NewResponseController(c.Writer)
		if control.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		if sse.Encode(c.Writer, event) != nil {
			return false
		}
		return control.Flush() == nil
	}
	if !write(sse.Event{Event: "connected", Data: "connected"}) {
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-tick.C:
			if !write(sse.Event{Event: "heartbeat", Data: "heartbeat"}) {
				return
			}
		case event, ok := <-client:
			if !ok {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil || len(payload) > 64<<10 {
				continue
			}
			if !write(sse.Event{Event: "emoji", Data: string(payload)}) {
				return
			}
		}
	}
}
func (ctl *QuestionController) WebSocket(c *gin.Context) {
	if !guard.OriginAllowed(c.Request, viper.GetStringSlice("server.allowed_origins")) || (siteconfig.Active().SecureCookie && c.GetHeader("Origin") != "" && !strings.HasPrefix(c.GetHeader("Origin"), "https://")) {
		c.AbortWithStatus(403)
		return
	}
	release, ok := guard.AcquireStream(c)
	if !ok {
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 1024, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool {
		return guard.OriginAllowed(r, viper.GetStringSlice("server.allowed_origins"))
	}}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		release()
		return
	}
	peer := &wsPeer{conn: conn, send: make(chan []byte, 64), done: make(chan struct{})}
	clientID := rand.Text()
	protection, clientIP := guard.Current(c), c.ClientIP()
	initial, _ := json.Marshal(map[string]string{"type": "connected", "clientId": clientID})
	peer.offer(initial)
	ctl.wsClientsMutex.Lock()
	ctl.wsClients[conn] = peer
	ctl.wsClientsMutex.Unlock()
	go peer.write()
	go func() {
		defer release()
		defer peer.close()
		defer func() {
			ctl.wsClientsMutex.Lock()
			delete(ctl.wsClients, conn)
			ctl.wsClientsMutex.Unlock()
			ctl.broadcastCursorLeave(clientID, conn)
		}()
		conn.SetReadLimit(1024)
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
		limiter := rate.NewLimiter(30, 40)
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				if errors.Is(err, websocket.ErrReadLimit) && protection != nil {
					protection.Note(clientIP, "stream", "invalid_message")
				}
				return
			}
			if kind != websocket.TextMessage || !limiter.Allow() {
				if protection != nil {
					protection.Note(clientIP, "stream", "message_limit")
				}
				return
			}
			if !ctl.cursorLimit.Allow() {
				continue
			} // Bound room-wide fanout independently of the number of senders.
			var msg struct {
				Type   string  `json:"type"`
				CardID int64   `json:"cardId"`
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
			}
			if json.Unmarshal(data, &msg) != nil || msg.Type != "cursor" || msg.CardID < 1 || msg.CardID > 1<<53 || math.IsNaN(msg.X) || math.IsNaN(msg.Y) || msg.X < -1 || msg.X > 100 || msg.Y < -1 || msg.Y > 100 {
				if protection != nil {
					protection.Note(clientIP, "stream", "invalid_message")
				}
				return
			}
			ctl.broadcastCursor(clientID, map[string]interface{}{"type": "cursor", "cardId": msg.CardID, "x": msg.X, "y": msg.Y}, conn)
		}
	}()
}
func (ctl *QuestionController) broadcastCursor(clientID string, msg map[string]interface{}, sender *websocket.Conn) {
	msg["clientId"] = clientID
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ctl.wsClientsMutex.Lock()
	defer ctl.wsClientsMutex.Unlock()
	for conn, peer := range ctl.wsClients {
		if conn != sender {
			peer.offer(data)
		}
	}
}
func (ctl *QuestionController) broadcastCursorLeave(clientID string, sender *websocket.Conn) {
	data, _ := json.Marshal(map[string]string{"type": "cursor_leave", "clientId": clientID})
	ctl.wsClientsMutex.Lock()
	defer ctl.wsClientsMutex.Unlock()
	for conn, peer := range ctl.wsClients {
		if conn != sender {
			peer.offer(data)
		}
	}
}
