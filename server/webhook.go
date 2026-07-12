package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jhh78/ws-server/logging"
)

// Webhook 은 env 에 설정된 URL 로 중계 이벤트를 HTTP POST 합니다.
//
// 이 서버는 비즈니스 로직 없이 중계만 하며, 외부 시스템은 웹훅으로 관찰·후처리합니다.
// URL 이 없으면 모든 Post 는 no-op 입니다. POST 는 고루틴에서 비동기로 수행되어
// WebSocket 중계 경로를 블로킹하지 않습니다.
type Webhook struct {
	// URLs 는 POST 대상 (하나 이상).
	URLs []string
	// ServerName 은 페이로드 server 필드.
	ServerName string
	// HTTP 는 타임아웃이 설정된 클라이언트.
	HTTP *http.Client
	// Log 는 POST 실패 시 경고 (nil 허용).
	Log *logging.Logger
}

// NewWebhook 은 URL 목록과 타임아웃으로 Webhook 을 만듭니다.
//
// Parameters:
//   - urls: WEBHOOK_URL 파싱 결과 (비어 있으면 nil 반환)
//   - timeoutMs: HTTP 클라이언트 타임아웃 (0 이하면 5000)
//   - serverName: 페이로드 식별명
//   - lg: 로거 (선택)
//
// Returns:
//   - *Webhook: 활성 디스패처, urls 없으면 nil
func NewWebhook(urls []string, timeoutMs int, serverName string, lg *logging.Logger) *Webhook {
	if len(urls) == 0 {
		return nil
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &Webhook{
		URLs:       append([]string(nil), urls...),
		ServerName: serverName,
		HTTP:       &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		Log:        lg,
	}
}

// WebhookPayload 는 외부 웹훅이 수신하는 JSON 본문입니다.
//
// 공통 필드:
//
//	event, ts, server, client_id, remote_addr
//	envelope — join/leave/send/whisper/ping 시 인바운드 스냅샷 (connect/disconnect 는 없음)
//
// 샘플 JSON 은 아래 상수(WebhookSample*) 및 README §7 참고.
type WebhookPayload struct {
	// Event 는 connect | disconnect | join | leave | send | whisper | ping.
	Event string `json:"event"`
	// Ts 는 Unix 밀리초.
	Ts int64 `json:"ts"`
	// Server 는 SERVER_NAME.
	Server string `json:"server"`
	// ClientID 는 서버 발급 연결 ID.
	ClientID string `json:"client_id,omitempty"`
	// RemoteAddr 는 피어 주소.
	RemoteAddr string `json:"remote_addr,omitempty"`
	// Envelope 는 라우팅 관련 이벤트일 때 인바운드 메시지 스냅샷 (connect/disconnect 는 생략 가능).
	Envelope *Envelope `json:"envelope,omitempty"`
}

// ---------------------------------------------------------------------------
// 웹훅 POST 본문 샘플 (문서·수신 측 구현 참고용. 런타임에 사용하지 않음)
// Content-Type: application/json; charset=utf-8
// ---------------------------------------------------------------------------

// WebhookSampleConnect 는 event=connect (envelope 없음).
const WebhookSampleConnect = `{
  "event": "connect",
  "ts": 1710000000000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321"
}`

// WebhookSampleDisconnect 는 event=disconnect (envelope 없음).
const WebhookSampleDisconnect = `{
  "event": "disconnect",
  "ts": 1710000005000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321"
}`

// WebhookSampleJoinArea 는 event=join, scope=area.
const WebhookSampleJoinArea = `{
  "event": "join",
  "ts": 1710000001000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "join",
    "scope": "area",
    "target": "lobby",
    "payload": { "name": "Alice" }
  }
}`

// WebhookSampleJoinChannel 는 event=join, scope=channel (party).
const WebhookSampleJoinChannel = `{
  "event": "join",
  "ts": 1710000001100,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "join",
    "scope": "channel",
    "channel_kind": "party",
    "target": "party-42",
    "payload": { "name": "Alice" }
  }
}`

// WebhookSampleLeave 는 event=leave.
const WebhookSampleLeave = `{
  "event": "leave",
  "ts": 1710000002000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "leave",
    "scope": "area",
    "target": "lobby"
  }
}`

// WebhookSampleSend 는 event=send (에리어 메시지).
const WebhookSampleSend = `{
  "event": "send",
  "ts": 1710000003000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "send",
    "scope": "area",
    "target": "lobby",
    "payload": { "text": "hello", "x": 1, "y": 2 }
  }
}`

// WebhookSampleWhisper 는 event=whisper (to = peer client_id).
const WebhookSampleWhisper = `{
  "event": "whisper",
  "ts": 1710000004000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "whisper",
    "to": "c2",
    "payload": { "text": "secret" }
  }
}`

// WebhookSamplePing 는 event=ping.
const WebhookSamplePing = `{
  "event": "ping",
  "ts": 1710000004500,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "ping"
  }
}`

// Post 는 이벤트를 모든 URL 에 비동기 JSON POST 합니다.
//
// Parameters:
//   - event: 이벤트 이름
//   - c: 관련 클라이언트 (nil 이면 id/addr 생략)
//   - msg: 관련 Envelope (nil 이면 페이로드에서 생략)
func (w *Webhook) Post(event string, c *Client, msg *Envelope) {
	if w == nil || len(w.URLs) == 0 {
		return
	}
	p := WebhookPayload{
		Event:  event,
		Ts:     time.Now().UnixMilli(),
		Server: w.ServerName,
	}
	if c != nil {
		p.ClientID = c.id
		p.RemoteAddr = c.remote
	}
	if msg != nil {
		// copy so async goroutine is not affected by later mutations
		cp := *msg
		p.Envelope = &cp
	}
	body, err := json.Marshal(p)
	if err != nil {
		return
	}
	// 중계 경로 비차단
	go w.deliver(body)
}

// deliver 는 모든 URL 에 동일 본문을 POST 합니다.
//
// Parameters:
//   - body: JSON 바이트
func (w *Webhook) deliver(body []byte) {
	for _, u := range w.URLs {
		req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			if w.Log != nil {
				w.Log.Warn("webhook", "new request failed", err.Error(), "url="+u)
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "ws-server-webhook/1")
		resp, err := w.HTTP.Do(req)
		if err != nil {
			if w.Log != nil {
				w.Log.Warn("webhook", "post failed", err.Error(), "url="+u)
			}
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 && w.Log != nil {
			w.Log.Warn("webhook", "non-2xx response",
				"status="+resp.Status, "url="+u)
		}
	}
}
