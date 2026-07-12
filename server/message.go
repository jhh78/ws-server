package server

import (
	"encoding/json"
	"time"
)

// Scope for membership / delivery.
const (
	ScopeArea    = "area"    // map, zone, lobby room
	ScopeChannel = "channel" // party, guild, whisper room, custom
)

// Channel kinds (channel scope).
const (
	ChannelParty   = "party"
	ChannelGuild   = "guild"
	ChannelWhisper = "whisper"
	ChannelCustom  = "custom"
)

// Envelope is the JSON wire format used by external clients and this server.
//
// Main pipeline: receive frame → decode Envelope → extension hooks → route → encode → send.
//
// Inbound types: join, leave, send, whisper, ping
// Outbound types: welcome, joined, left, message, whisper, system, error, pong
type Envelope struct {
	Type        string          `json:"type"`
	Scope       string          `json:"scope,omitempty"`        // area | channel
	Target      string          `json:"target,omitempty"`       // area id or channel id
	ChannelKind string          `json:"channel_kind,omitempty"` // party|guild|whisper|custom
	To          string          `json:"to,omitempty"`           // whisper peer client_id
	From        string          `json:"from,omitempty"`
	ClientID    string          `json:"client_id,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Error       string          `json:"error,omitempty"`
	Ts          int64           `json:"ts,omitempty"`
}

func encodeEnvelope(m Envelope) ([]byte, error) {
	if m.Ts == 0 {
		m.Ts = time.Now().UnixMilli()
	}
	return json.Marshal(m)
}

func decodeEnvelope(data []byte) (Envelope, error) {
	var m Envelope
	err := json.Unmarshal(data, &m)
	return m, err
}

func rawJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}

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
