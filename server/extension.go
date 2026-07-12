package server

import "github.com/jhh78/ws-server/logging"

// extension.go — 중계 서버 부가 처리 (로컬 로그 + 선택적 웹훅).
//
// 이 프로젝트는 **중계만** 수행합니다. 인증·필터·비즈니스 규칙은 서버 안에 두지 않고,
// WEBHOOK_URL 이 설정된 경우 이벤트를 외부 엔드포인트로 HTTP POST 합니다.
// 웹훅 실패는 중계를 막지 않습니다 (비동기, best-effort).
//
// # 웹훅 전송 데이터 샘플
//
// 실제 POST 본문은 WebhookPayload JSON 입니다. 이벤트별 샘플 상수는 webhook.go 의
// WebhookSample* 를 보세요. 요약:
//
//	connect / disconnect — envelope 없음
//	  { "event":"connect", "ts":..., "server":"ws-server", "client_id":"c1", "remote_addr":"..." }
//
//	join / leave / send / whisper / ping — envelope 에 인바운드 메시지 포함
//	  { "event":"join", "ts":..., "server":"...", "client_id":"c1", "remote_addr":"...",
//	    "envelope": { "type":"join", "scope":"area", "target":"lobby", "payload":{...} } }
//
// 전체 JSON 예: WebhookSampleConnect, WebhookSampleJoinArea, WebhookSampleSend, ...

// extOnConnect 는 연결·허브 등록 직후, welcome 전송 전에 한 번 호출됩니다.
//
// Parameters:
//   - c: 방금 등록된 클라이언트
//
// 동작: 시스템/액세스 로그 + 웹훅 event=connect.
// 전송 샘플: WebhookSampleConnect
func extOnConnect(c *Client) {
	c.log.Info("client", "connected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "connect",
	})
	if c.hub != nil {
		c.hub.webhook.Post("connect", c, nil)
	}
}

// extOnDisconnect 는 연결 종료 직전 (leaveAll / unregister 전) 호출됩니다.
//
// Parameters:
//   - c: 종료 중인 클라이언트
//
// 동작: 시스템/액세스 로그 + 웹훅 event=disconnect.
// 전송 샘플: WebhookSampleDisconnect
func extOnDisconnect(c *Client) {
	c.log.Info("client", "disconnected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "disconnect",
	})
	if c.hub != nil {
		c.hub.webhook.Post("disconnect", c, nil)
	}
}

// extProcessInbound 는 JSON 파싱 직후, 라우팅 직전에 호출됩니다.
//
// 중계 전용: 메시지를 수정·드롭하지 않고 그대로 통과시킵니다.
//
// Parameters:
//   - c: 발신 클라이언트
//   - msg: 파싱된 Envelope
//
// Returns:
//   - out: 동일 msg
//   - drop: 항상 false
func extProcessInbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	return msg, false
}

// extProcessOutbound 는 클라이언트 송신 큐에 넣기 직전에 호출됩니다.
//
// 중계 전용: 메시지를 수정·드롭하지 않고 그대로 통과시킵니다.
//
// Parameters:
//   - c: 수신 클라이언트
//   - msg: 아웃바운드 Envelope
//
// Returns:
//   - out: 동일 msg
//   - drop: 항상 false
func extProcessOutbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	return msg, false
}

// extOnRouted 는 join/leave/send/whisper/ping 라우팅이 끝난 뒤 호출됩니다.
//
// Parameters:
//   - c: 요청 클라이언트
//   - action: "join" | "leave" | "send" | "whisper" | "ping"
//   - msg: 인바운드 메시지
//
// 동작: 액세스 로그 + 웹훅 (action 이름 = event, envelope 포함).
// 전송 샘플:
//   join    → WebhookSampleJoinArea / WebhookSampleJoinChannel
//   leave   → WebhookSampleLeave
//   send    → WebhookSampleSend
//   whisper → WebhookSampleWhisper
//   ping    → WebhookSamplePing
func extOnRouted(c *Client, action string, msg *Envelope) {
	if msg == nil {
		return
	}
	c.log.Access(logging.AccessEntry{
		ClientID:    c.id,
		RemoteAddr:  c.remote,
		Action:      action,
		Type:        msg.Type,
		Scope:       msg.Scope,
		Target:      msg.Target,
		ChannelKind: msg.ChannelKind,
		Detail:      msg.To,
	})
	if c.hub != nil {
		c.hub.webhook.Post(action, c, msg)
	}
}
