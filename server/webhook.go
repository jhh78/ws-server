package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jhh78/ws-server/logging"
)

// Webhook 은 env 에 설정된 URL 로 중계 이벤트를 HTTP POST 합니다.
//
// 이 서버는 비즈니스 로직 없이 중계만 하며, 외부 시스템은 웹훅으로 관찰·후처리합니다.
// URL 이 없으면 모든 Post 는 no-op 입니다. POST 는 고루틴에서 비동기로 수행되어
// WebSocket 중계 경로를 블로킹하지 않습니다.
// 타임아웃 설정은 두지 않으며, 전송 성공·실패 결과를 로그에 기록합니다.
type Webhook struct {
	// URLs 는 POST 대상 (하나 이상).
	URLs []string
	// ServerName 은 페이로드 server 필드.
	ServerName string
	// HTTP 는 요청에 쓰는 클라이언트 (타임아웃 미설정).
	HTTP *http.Client
	// Log 는 POST 결과 기록 (nil 허용).
	Log *logging.Logger
}

// NewWebhook 은 URL 목록으로 Webhook 을 만듭니다.
//
// Parameters:
//   - urls: WEBHOOK_URL 파싱 결과 (비어 있으면 nil 반환)
//   - serverName: 페이로드 식별명
//   - lg: 로거 (선택)
//
// Returns:
//   - *Webhook: 활성 디스패처, urls 없으면 nil
func NewWebhook(urls []string, serverName string, lg *logging.Logger) *Webhook {
	if len(urls) == 0 {
		return nil
	}
	return &Webhook{
		URLs:       append([]string(nil), urls...),
		ServerName: serverName,
		HTTP:       &http.Client{}, // 타임아웃 없음 — 결과는 로그로 남김
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
		w.logResult("error", "marshal failed", err.Error(), "", event, 0)
		return
	}
	// 중계 경로 비차단
	go w.deliver(event, body)
}

// deliver 는 모든 URL 에 동일 본문을 POST 하고 결과를 로그에 남깁니다.
//
// Parameters:
//   - event: 이벤트 이름 (로그용)
//   - body: JSON 바이트
func (w *Webhook) deliver(event string, body []byte) {
	for _, u := range w.URLs {
		req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			w.logResult("error", "new request failed", err.Error(), u, event, 0)
			continue
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "ws-server-webhook/1")

		start := time.Now()
		resp, err := w.HTTP.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			w.logResult("error", "post failed", err.Error(), u, event, elapsed)
			continue
		}
		// drain body so connection can be reused
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			w.logResult("ok", "post ok",
				fmt.Sprintf("status=%d", resp.StatusCode), u, event, elapsed)
		} else {
			w.logResult("warn", "non-2xx response",
				fmt.Sprintf("status=%d", resp.StatusCode), u, event, elapsed)
		}
	}
}

// logResult 는 웹훅 전송 결과를 시스템 로그에 기록합니다.
//
// Parameters:
//   - level: ok | warn | error
//   - message: 요약
//   - detail: 부가 정보
//   - url: 대상 URL
//   - event: 이벤트 이름
//   - elapsed: 소요 시간 (0 이면 생략)
func (w *Webhook) logResult(level, message, detail, url, event string, elapsed time.Duration) {
	if w == nil || w.Log == nil {
		return
	}
	parts := []string{}
	if event != "" {
		parts = append(parts, "event="+event)
	}
	if url != "" {
		parts = append(parts, "url="+url)
	}
	if detail != "" {
		parts = append(parts, detail)
	}
	if elapsed > 0 {
		parts = append(parts, fmt.Sprintf("elapsed=%s", elapsed.Round(time.Millisecond)))
	}
	switch level {
	case "ok":
		w.Log.Info("webhook", message, parts...)
	case "warn":
		w.Log.Warn("webhook", message, parts...)
	default:
		w.Log.Error("webhook", message, parts...)
	}
}
