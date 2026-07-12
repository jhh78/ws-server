package server

import (
	"encoding/json"
)

// routeInbound dispatches a parsed Envelope to membership / delivery handlers.
// Core server responsibility: receive JSON → (extension) → route → respond.
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

// reply sends an Envelope to this client (through outbound extension).
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
