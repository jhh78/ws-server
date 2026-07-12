package server

import "github.com/jhh78/ws-server/logging"

// extension.go — 비즈니스·부가 처리 확장 지점.
//
// 메인 루프: 수신 → JSON 파싱 → 훅 → 라우팅 → 응답.
// 액세스/시스템 로그의 기본 연동은 여기와 server 기동 경로에 있습니다.
// 추가 비즈니스 로직은 본문을 채우거나 package server 의 다른 파일로 분리하세요.

// extOnConnect 는 연결·허브 등록 직후, welcome 전송 전에 한 번 호출됩니다.
//
// Parameters:
//   - c: 방금 등록된 클라이언트
//
// 기본 구현: 시스템 INFO + 액세스 connect 로그.
func extOnConnect(c *Client) {
	c.log.Info("client", "connected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "connect",
	})
}

// extOnDisconnect 는 연결 종료 직전 (leaveAll / unregister 전) 호출됩니다.
//
// Parameters:
//   - c: 종료 중인 클라이언트
//
// 기본 구현: 시스템 INFO + 액세스 disconnect 로그.
func extOnDisconnect(c *Client) {
	c.log.Info("client", "disconnected", "id="+c.id, "remote="+c.remote)
	c.log.Access(logging.AccessEntry{
		ClientID:   c.id,
		RemoteAddr: c.remote,
		Action:     "disconnect",
	})
}

// extProcessInbound 는 JSON 파싱 직후, 라우팅 직전에 호출됩니다.
//
// Parameters:
//   - c: 발신 클라이언트
//   - msg: 파싱된 Envelope
//
// Returns:
//   - out: 라우팅에 넘길 메시지 (수정 가능)
//   - drop: true 이면 이후 단계 생략 (무응답 드롭)
//
// 기본 구현: 통과 (msg, false). TODO: 권한, payload 변환, 커스텀 type.
func extProcessInbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 권한 검사, payload 변환, 커스텀 type 처리 등
	return msg, false
}

// extProcessOutbound 는 클라이언트 송신 큐에 넣기 직전에 호출됩니다.
//
// Parameters:
//   - c: 수신 클라이언트
//   - msg: 아웃바운드 Envelope
//
// Returns:
//   - out: 실제 전송할 메시지
//   - drop: true 이면 전송 생략
//
// 기본 구현: 통과. TODO: 필터, 검열, 감사 로그.
func extProcessOutbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 필터, 검열, 감사 로그 등
	return msg, false
}

// extOnRouted 는 join/leave/send/whisper/ping 라우팅이 끝난 뒤 호출됩니다.
//
// Parameters:
//   - c: 요청 클라이언트
//   - action: "join" | "leave" | "send" | "whisper" | "ping"
//   - msg: 원본(또는 처리된) 인바운드 메시지
//
// 기본 구현: 액세스 로그 (Detail 에 whisper to 등).
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
