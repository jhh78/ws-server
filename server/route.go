package server

import (
	"encoding/json"
)

// routeInbound 은 파싱된 Envelope 를 멤버십·중계 핸들러로 디스패치합니다.
//
// 서버 핵심: 수신 JSON → route/중계 → 응답 (+ 웹훅 알림).
//
// Parameters:
//   - msg: 인바운드 Envelope (nil 아님 가정)
func (c *Client) routeInbound(msg *Envelope) {
	switch msg.Type {
	case "join":
		c.routeJoin(msg)
		extOnRouted(c, "join", msg)
	case "leave":
		c.routeLeave(msg)
		extOnRouted(c, "leave", msg)
	case "send":
		c.routeSend(msg)
		extOnRouted(c, "send", msg)
	case "whisper":
		c.routeWhisper(msg)
		extOnRouted(c, "whisper", msg)
	case "ping":
		c.reply(Envelope{Type: "pong", ClientID: c.id})
		extOnRouted(c, "ping", msg)
	default:
		c.replyError("unknown type: "+msg.Type, msg.Scope, msg.Target)
	}
}

// applyNameFromPayload 는 join payload 에서 표시 이름을 설정합니다.
//
// 지원 형식:
//   - JSON 문자열: "Alice"
//   - 객체: { "name": "Alice" }
//
// Parameters:
//   - payload: Envelope.Payload
func (c *Client) applyNameFromPayload(payload json.RawMessage) {
	if len(payload) == 0 {
		return
	}
	var asString string
	if err := json.Unmarshal(payload, &asString); err == nil && asString != "" {
		c.setName(asString)
		return
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &obj); err == nil && obj.Name != "" {
		c.setName(obj.Name)
	}
}

// routeJoin 은 scope=area|channel 가입을 처리하고 joined + system 알림을 보냅니다.
//
// Parameters:
//   - msg: type=join Envelope
func (c *Client) routeJoin(msg *Envelope) {
	c.applyNameFromPayload(msg.Payload)

	switch msg.Scope {
	case ScopeArea, "":
		scope := ScopeArea
		if errMsg := c.hub.joinArea(c, msg.Target); errMsg != "" {
			c.replyError(errMsg, scope, msg.Target)
			return
		}
		c.reply(Envelope{
			Type:     "joined",
			Scope:    scope,
			Target:   msg.Target,
			ClientID: c.id,
			From:     c.getName(),
			Payload: rawJSON(map[string]any{
				"scope":  scope,
				"target": msg.Target,
			}),
		})
		notice := systemEnvelope(scope, msg.Target, c.getName(), c.id, "joined")
		c.hub.broadcastArea(msg.Target, notice, c)

	case ScopeChannel:
		kind := msg.ChannelKind
		if kind == "" {
			kind = ChannelCustom
		}
		if errMsg := c.hub.joinChannel(c, kind, msg.Target); errMsg != "" {
			c.replyError(errMsg, ScopeChannel, msg.Target)
			return
		}
		c.reply(Envelope{
			Type:        "joined",
			Scope:       ScopeChannel,
			Target:      msg.Target,
			ChannelKind: kind,
			ClientID:    c.id,
			From:        c.getName(),
			Payload: rawJSON(map[string]any{
				"scope":        ScopeChannel,
				"channel_kind": kind,
				"target":       msg.Target,
			}),
		})
		notice := systemEnvelope(ScopeChannel, msg.Target, c.getName(), c.id, "joined")
		var n Envelope
		_ = json.Unmarshal(notice, &n)
		n.ChannelKind = kind
		nb, _ := encodeEnvelope(n)
		c.hub.broadcastChannel(kind, msg.Target, nb, c)

	default:
		c.replyError("scope must be area or channel", msg.Scope, msg.Target)
	}
}

// routeLeave 는 에리어/채널 탈퇴를 처리하고 left + system 을 보냅니다.
//
// Parameters:
//   - msg: type=leave Envelope
func (c *Client) routeLeave(msg *Envelope) {
	switch msg.Scope {
	case ScopeArea, "":
		c.hub.leaveArea(c, msg.Target)
		c.reply(Envelope{
			Type:     "left",
			Scope:    ScopeArea,
			Target:   msg.Target,
			ClientID: c.id,
		})
		notice := systemEnvelope(ScopeArea, msg.Target, c.getName(), c.id, "left")
		c.hub.broadcastArea(msg.Target, notice, c)

	case ScopeChannel:
		kind := msg.ChannelKind
		if kind == "" {
			kind = ChannelCustom
		}
		c.hub.leaveChannel(c, kind, msg.Target)
		c.reply(Envelope{
			Type:        "left",
			Scope:       ScopeChannel,
			Target:      msg.Target,
			ChannelKind: kind,
			ClientID:    c.id,
		})
		notice := systemEnvelope(ScopeChannel, msg.Target, c.getName(), c.id, "left")
		var n Envelope
		_ = json.Unmarshal(notice, &n)
		n.ChannelKind = kind
		nb, _ := encodeEnvelope(n)
		c.hub.broadcastChannel(kind, msg.Target, nb, c)

	default:
		c.replyError("scope must be area or channel", msg.Scope, msg.Target)
	}
}

// routeSend 는 멤버십을 검사한 뒤 message 를 브로드캐스트합니다 (본인 에코 포함).
//
// Parameters:
//   - msg: type=send Envelope
func (c *Client) routeSend(msg *Envelope) {
	if msg.Target == "" {
		c.replyError("target is required", msg.Scope, "")
		return
	}

	out := &Envelope{
		Type:        "message",
		Scope:       msg.Scope,
		Target:      msg.Target,
		ChannelKind: msg.ChannelKind,
		From:        c.getName(),
		ClientID:    c.id,
		Payload:     msg.Payload,
	}

	switch msg.Scope {
	case ScopeArea, "":
		out.Scope = ScopeArea
		if !c.hub.inArea(c, msg.Target) {
			c.replyError("not a member of area; join first", ScopeArea, msg.Target)
			return
		}
		out, drop := extProcessOutbound(c, out)
		if drop || out == nil {
			return
		}
		raw, _ := encodeEnvelope(*out)
		c.hub.broadcastArea(msg.Target, raw, c)
		c.trySend(raw)

	case ScopeChannel:
		kind := msg.ChannelKind
		if kind == "" {
			kind = ChannelCustom
		}
		out.ChannelKind = kind
		if !c.hub.inChannel(c, kind, msg.Target) {
			c.replyError("not a member of channel; join first", ScopeChannel, msg.Target)
			return
		}
		out, drop := extProcessOutbound(c, out)
		if drop || out == nil {
			return
		}
		raw, _ := encodeEnvelope(*out)
		c.hub.broadcastChannel(kind, msg.Target, raw, c)
		c.trySend(raw)

	default:
		c.replyError("scope must be area or channel", msg.Scope, msg.Target)
	}
}

// routeWhisper 는 to=client_id 로 1:1 메시지를 전달하고 발신자에게 에코합니다.
//
// Parameters:
//   - msg: type=whisper Envelope
func (c *Client) routeWhisper(msg *Envelope) {
	if msg.To == "" {
		c.replyError("to (client_id) is required for whisper", "", "")
		return
	}
	peer := c.hub.getByID(msg.To)
	if peer == nil {
		c.replyError("peer not found: "+msg.To, "", "")
		return
	}
	out := &Envelope{
		Type:     "whisper",
		From:     c.getName(),
		ClientID: c.id,
		To:       msg.To,
		Payload:  msg.Payload,
	}
	out, drop := extProcessOutbound(c, out)
	if drop || out == nil {
		return
	}
	raw, _ := encodeEnvelope(*out)
	peer.trySend(raw)
	c.trySend(raw)
}

// reply 는 Envelope 를 이 클라이언트에게 보냅니다 (아웃바운드 훅은 통과만).
//
// Parameters:
//   - msg: 송신할 Envelope
func (c *Client) reply(msg Envelope) {
	out, drop := extProcessOutbound(c, &msg)
	if drop || out == nil {
		return
	}
	raw, err := encodeEnvelope(*out)
	if err != nil {
		return
	}
	c.trySend(raw)
}

// replyError 는 type=error Envelope 를 보냅니다.
//
// Parameters:
//   - errMsg: 오류 요약
//   - scope: 관련 scope
//   - target: 관련 target
func (c *Client) replyError(errMsg, scope, target string) {
	c.reply(Envelope{
		Type:   "error",
		Scope:  scope,
		Target: target,
		Error:  errMsg,
		Payload: rawJSON(map[string]string{
			"message": errMsg,
		}),
	})
}
