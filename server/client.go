package server

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/logging"
)

// WebSocket 읽기/쓰기·하트비트 관련 상수.
const (
	// writeWait 는 단일 쓰기 데드라인.
	writeWait = 10 * time.Second
	// pongWait 는 피어 Pong 대기 시간 (읽기 데드라인).
	pongWait = 60 * time.Second
	// pingPeriod 는 서버 Ping 주기 (pongWait 보다 짧게).
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize 는 연결당 최대 수신 페이로드(바이트).
	maxMessageSize = 64 * 1024
	// sendBufSize 는 송신 큐 길이 (포화 시 프레임 드롭).
	sendBufSize = 256
)

// clientSeq 는 서버 전역 클라이언트 ID 일련번호입니다.
var clientSeq atomic.Uint64

// Client 는 외부 WebSocket 연결 하나입니다 (저장소 내 클라이언트 앱 없음).
//
// 필드 요약:
//   - hub/conn/send: 허브·소켓·송신 큐
//   - areas/channels: 가입 멤버십
//   - name: 표시 이름 (join payload)
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	id     string
	remote string
	log    *logging.Logger

	areas    map[string]struct{}
	channels map[string]struct{}

	name string
	mu   sync.Mutex
}

// newClient 는 고유 id 를 발급하고 빈 멤버십 맵을 가진 Client 를 만듭니다.
//
// Parameters:
//   - hub: 소속 허브
//   - conn: 업그레이드된 WebSocket
//   - remote: 피어 주소 문자열
//   - lg: 로거 (nil 이면 Nop)
//
// Returns:
//   - *Client: 등록 전 클라이언트
func newClient(hub *Hub, conn *websocket.Conn, remote string, lg *logging.Logger) *Client {
	id := fmt.Sprintf("c%d", clientSeq.Add(1))
	if lg == nil {
		lg = logging.Nop()
	}
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, sendBufSize),
		id:       id,
		remote:   remote,
		log:      lg,
		areas:    make(map[string]struct{}),
		channels: make(map[string]struct{}),
	}
}

// ID 는 서버가 발급한 클라이언트 ID 를 반환합니다 (외부 도구·로그용).
//
// Returns:
//   - string: 예 "c1"
func (c *Client) ID() string { return c.id }

// RemoteAddr 는 피어 주소 문자열을 반환합니다.
//
// Returns:
//   - string: RemoteAddr
func (c *Client) RemoteAddr() string { return c.remote }

// trySend 는 송신 큐에 non-blocking 으로 넣습니다. 포화 시 드롭합니다.
//
// Parameters:
//   - data: 이미 인코딩된 JSON 바이트
//
// Returns:
//   - bool: 큐 적재 성공 여부
func (c *Client) trySend(data []byte) bool {
	select {
	case c.send <- data:
		return true
	default:
		log.Printf("drop message for slow client id=%s", c.id)
		return false
	}
}

// setName 은 표시 이름을 설정합니다 (스레드 세이프).
//
// Parameters:
//   - name: 표시 이름
func (c *Client) setName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = name
}

// getName 은 표시 이름을 반환합니다. 비어 있으면 client id.
//
// Returns:
//   - string: name 또는 id
func (c *Client) getName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name != "" {
		return c.name
	}
	return c.id
}

// readPump 는 수신 루프입니다: 프레임 수신 → onReceive → (종료 시) 훅·leaveAll·unregister.
//
// 호출 스레드에서 블로킹합니다. writePump 는 별 고루틴에서 실행해야 합니다.
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

// onReceive 는 인바운드 프레임 한 개의 메인 파이프라인입니다.
//
// 처리 단계:
//  1. JSON → Envelope
//  2. extProcessInbound
//  3. routeInbound
//
// Parameters:
//   - data: Text 프레임 페이로드
func (c *Client) onReceive(data []byte) {
	// 1) JSON parse
	msg, err := decodeEnvelope(data)
	if err != nil {
		c.log.Access(logging.AccessEntry{
			ClientID:   c.id,
			RemoteAddr: c.remote,
			Action:     "error",
			Detail:     "invalid json",
		})
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

// writePump 는 송신 큐·주기적 Ping 을 소켓에 씁니다.
//
// send 채널이 닫히면 Close 프레임을 보내고 종료합니다.
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
