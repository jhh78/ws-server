package server

// extension.go — 비즈니스/부가 처리 확장 지점.
//
// 이 프로젝트의 메인 루프는:
//
//	수신(WebSocket) → JSON 파싱 → 여기 훅 호출 → 라우팅(전송) → 응답
//
// 클라이언트 프로그램은 포함하지 않는다. 외부 클라이언트가 JSON Envelope 로 붙는다.
// 실제 게임/채팅/검증/로그 등은 아래 빈 함수 본문을 채우거나,
// 같은 package server 의 다른 파일로 로직을 분리해 연결한다.

// extOnConnect 는 연결·등록 직후 한 번 호출된다 (welcome 전송 전).
func extOnConnect(c *Client) {
	// TODO: 인증 세션 연결, 메트릭, 접속 로그 등
}

// extOnDisconnect 는 연결 종료 직전 호출된다 (leaveAll / unregister 전).
func extOnDisconnect(c *Client) {
	// TODO: 세션 정리, 오프라인 처리 등
}

// extProcessInbound 는 JSON 파싱 직후, 라우팅(join/send/…) 직전에 호출된다.
// drop=true 이면 서버는 해당 메시지를 처리하지 않는다 (에러 응답은 훅에서 보낼 수 있음).
// 반환 out 이 nil 이 아니면 그 Envelope 로 라우팅한다.
func extProcessInbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 권한 검사, payload 변환, 커스텀 type 처리 등
	return msg, false
}

// extProcessOutbound 는 클라이언트에게 나가기 직전(큐에 넣기 전) 호출된다.
// drop=true 이면 전송하지 않는다. out 으로 페이로드를 수정할 수 있다.
func extProcessOutbound(c *Client, msg *Envelope) (out *Envelope, drop bool) {
	// TODO: 필터, 검열, 감사 로그, 암호화 래핑 등
	return msg, false
}

// extOnRouted 는 join/leave/send/whisper 등 라우팅이 끝난 뒤 호출된다.
// action 예: "join","leave","send","whisper","ping"
func extOnRouted(c *Client, action string, msg *Envelope) {
	// TODO: 통계, 비동기 후처리 등
	_ = action
	_ = msg
}
