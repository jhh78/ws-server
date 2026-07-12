package server

import (
	"encoding/json"
	"time"
)

// 멤버십·전달 범위 (Envelope.scope).
const (
	// ScopeArea 는 맵·존·로비 등 에리어.
	ScopeArea = "area"
	// ScopeChannel 은 파티·길드·귓속말 룸·커스텀 채널.
	ScopeChannel = "channel"
)

// 채널 종류 (Envelope.channel_kind, scope=channel).
const (
	// ChannelParty 는 파티 채널.
	ChannelParty = "party"
	// ChannelGuild 는 길드 채널.
	ChannelGuild = "guild"
	// ChannelWhisper 는 귓속말용 룸 (1:1 은 보통 type=whisper).
	ChannelWhisper = "whisper"
	// ChannelCustom 은 기타 (channel_kind 생략 시 기본).
	ChannelCustom = "custom"
)

// Envelope 는 외부 클라이언트와 서버가 주고받는 JSON 와이어 포맷입니다.
//
// 메인 파이프라인: 프레임 수신 → decode → 확장 훅 → route → encode → 송신.
//
// 인바운드 type: join, leave, send, whisper, ping
// 아웃바운드 type: welcome, joined, left, message, whisper, system, error, pong
type Envelope struct {
	// Type 은 메시지 종류 (필수).
	Type string `json:"type"`
	// Scope 는 area | channel (join/leave/send).
	Scope string `json:"scope,omitempty"`
	// Target 은 에리어 ID 또는 채널 ID.
	Target string `json:"target,omitempty"`
	// ChannelKind 는 party|guild|whisper|custom.
	ChannelKind string `json:"channel_kind,omitempty"`
	// To 는 whisper 대상 client_id.
	To string `json:"to,omitempty"`
	// From 은 표시 이름 (서버가 채우는 경우 많음).
	From string `json:"from,omitempty"`
	// ClientID 는 서버 발급 연결 ID.
	ClientID string `json:"client_id,omitempty"`
	// Payload 는 앱 데이터 (임의 JSON).
	Payload json.RawMessage `json:"payload,omitempty"`
	// Error 는 type=error 시 요약 메시지.
	Error string `json:"error,omitempty"`
	// Ts 는 서버 시각 Unix 밀리초.
	Ts int64 `json:"ts,omitempty"`
}

// encodeEnvelope 는 Envelope 를 JSON 바이트로 직렬화합니다. Ts 가 0 이면 현재 시각을 채웁니다.
//
// Parameters:
//   - m: 아웃바운드 메시지
//
// Returns:
//   - []byte: JSON
//   - error: Marshal 오류
func encodeEnvelope(m Envelope) ([]byte, error) {
	if m.Ts == 0 {
		m.Ts = time.Now().UnixMilli()
	}
	return json.Marshal(m)
}

// decodeEnvelope 는 Text 프레임 바이트를 Envelope 로 파싱합니다.
//
// Parameters:
//   - data: UTF-8 JSON
//
// Returns:
//   - Envelope: 파싱 결과
//   - error: Unmarshal 오류
func decodeEnvelope(data []byte) (Envelope, error) {
	var m Envelope
	err := json.Unmarshal(data, &m)
	return m, err
}

// rawJSON 은 임의 값을 json.RawMessage 로 마샬합니다. 실패 시 null.
//
// Parameters:
//   - v: 직렬화할 값
//
// Returns:
//   - json.RawMessage: 페이로드 조각
func rawJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}

// errorEnvelope 는 type=error 프레임 바이트를 만듭니다.
//
// Parameters:
//   - errMsg: 오류 요약
//   - scope: 관련 scope (선택)
//   - target: 관련 target (선택)
//
// Returns:
//   - []byte: 인코딩된 오류 Envelope
func errorEnvelope(errMsg string, scope, target string) []byte {
	b, _ := encodeEnvelope(Envelope{
		Type:   "error",
		Scope:  scope,
		Target: target,
		Error:  errMsg,
		Payload: rawJSON(map[string]string{
			"message": errMsg,
		}),
	})
	return b
}

// systemEnvelope 는 type=system (join/leave 알림 등) 프레임 바이트를 만듭니다.
//
// Parameters:
//   - scope: area | channel
//   - target: 대상 ID
//   - from: 표시 이름
//   - clientID: 발신 연결 ID
//   - text: payload.event 문자열
//
// Returns:
//   - []byte: 인코딩된 system Envelope
func systemEnvelope(scope, target, from, clientID, text string) []byte {
	b, _ := encodeEnvelope(Envelope{
		Type:     "system",
		Scope:    scope,
		Target:   target,
		From:     from,
		ClientID: clientID,
		Payload:  rawJSON(map[string]string{"event": text}),
	})
	return b
}
