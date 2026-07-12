package server

import "github.com/jhh78/ws-server/logging"

// extension.go — 비즈니스/부가 처리 확장 지점.
//
// 메인 루프: 수신 → JSON 파싱 → 훅 → 라우팅 → 응답
// 액세스/시스템 로그의 기본 연동은 여기와 server 기동 경로에 있다.
// 추가 비즈니스 로직은 본문을 채우거나 package server 의 다른 파일로 분리한다.

// extOnConnect 는 연결·등록 직후 한 번 호출된다 (welcome 전송 전).
func extOnConnect(c *Client) {
	c.log.Info("client", "connected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "connect",
	})
}

// extOnDisconnect 는 연결 종료 직전 호출된다 (leaveAll / unregister 전).
func extOnDisconnect(c *Client) {
	c.log.Info("client", "disconnected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "disconnect",
	})
}

// extProcessInbound 는 JSON 파싱 직후, 라우팅 직전에 호출된다.
func extProcessInbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 권한 검사, payload 변환, 커스텀 type 처리 등
	return msg, false
}

// extProcessOutbound 는 클라이언트에게 나가기 직전(큐에 넣기 전) 호출된다.
func extProcessOutbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 필터, 검열, 감사 로그 등
	return msg, false
}

// extOnRouted 는 join/leave/send/whisper 등 라우팅이 끝난 뒤 호출된다.
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
		Detail:      msg.To, // whisper peer when present
	})
}
