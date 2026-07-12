package server

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 * 1024
	sendBufSize    = 256
)

var clientSeq atomic.Uint64

// Client is one external WebSocket connection (no in-repo client app).
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	id     string
	remote string

	areas    map[string]struct{}
	channels map[string]struct{}

	name string
	mu   sync.Mutex
}

func newClient(hub *Hub, conn *websocket.Conn, remote string) *Client {
	id := fmt.Sprintf("c%d", clientSeq.Add(1))
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, sendBufSize),
		id:       id,
		remote:   remote,
		areas:    make(map[string]struct{}),
		channels: make(map[string]struct{}),
	}
}

// ID returns the server-assigned client id (for external tools / logs).
func (c *Client) ID() string { return c.id }

// RemoteAddr returns the peer address string.
func (c *Client) RemoteAddr() string { return c.remote }

func (c *Client) trySend(data []byte) bool {
	select {
	case c.send <- data:
		return true
	default:
		log.Printf("drop message for slow client id=%s", c.id)
		return false
	}
}

func (c *Client) setName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = name
}

func (c *Client) getName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name != "" {
		return c.name
	}
	return c.id
}

// readPump: receive → JSON → extension → route.
func (c *Client) readPump() {
	defer func() {
		extOnDisconnect(c)
		c.hub.leaveAll(c)
		c.hub.unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error id=%s: %v", c.id, err)
			}
			break
		}
		c.onReceive(data)
	}
}

// onReceive is the main server pipeline for one inbound frame.
func (c *Client) onReceive(data []byte) {
	// 1) JSON parse
	msg, err := decodeEnvelope(data)
	if err != nil {
		c.replyError("invalid json message", "", "")
		return
	}

	// 2) extension: business / filter (stub by default)
	processed, drop := extProcessInbound(c, &msg)
	if drop {
		return
	}
	if processed == nil {
		return
	}

	// 3) route + respond (membership / broadcast / whisper)
	c.routeInbound(processed)
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
