package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jhh78/ws-server/server"
)

// TestHealthHandler 는 GET /health 가 200 OK 를 반환하는지 확인합니다.
func TestHealthHandler(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(s.Close)

	resp, err := http.Get(s.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// TestWelcomeJSON 은 접속 직후 welcome 과 protocol 페이로드를 검증합니다.
func TestWelcomeJSON(t *testing.T) {
	t.Parallel()

	conn := dialWS(t)
	defer conn.Close()

	msg := expectType(t, conn, "welcome")
	if msg.ClientID == "" {
		t.Fatal("expected client_id")
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["protocol"] == nil {
		t.Fatalf("payload = %s", msg.Payload)
	}
}

// TestAreaBroadcastToAllMembers 는 에리어 멤버 전원 수신·외부자 미수신을 검증합니다.
func TestAreaBroadcastToAllMembers(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)

	a := dialWSURL(t, ts.URL, "/ws")
	defer a.Close()
	b := dialWSURL(t, ts.URL, "/ws")
	defer b.Close()
	outsider := dialWSURL(t, ts.URL, "/ws")
	defer outsider.Close()

	expectType(t, a, "welcome")
	expectType(t, b, "welcome")
	expectType(t, outsider, "welcome")

	mustWriteEnv(t, a, server.Envelope{
		Type: "join", Scope: "area", Target: "map-1",
		Payload: payloadJSON(map[string]string{"name": "Alice"}),
	})
	expectType(t, a, "joined")

	mustWriteEnv(t, b, server.Envelope{
		Type: "join", Scope: "area", Target: "map-1",
		Payload: payloadJSON(map[string]string{"name": "Bob"}),
	})
	expectType(t, b, "joined")
	expectType(t, a, "system")

	body := map[string]any{"text": "hello map", "x": 1, "y": 2}
	mustWriteEnv(t, a, server.Envelope{
		Type: "send", Scope: "area", Target: "map-1",
		Payload: payloadJSON(body),
	})

	gotA := expectType(t, a, "message")
	gotB := expectType(t, b, "message")
	for _, got := range []server.Envelope{gotA, gotB} {
		if got.Scope != "area" || got.Target != "map-1" {
			t.Fatalf("got scope/target = %s/%s", got.Scope, got.Target)
		}
		var p map[string]any
		if err := json.Unmarshal(got.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p["text"] != "hello map" {
			t.Fatalf("payload = %s", got.Payload)
		}
	}

	_ = outsider.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := outsider.ReadMessage(); err == nil {
		t.Fatal("outsider must not receive area message")
	}
}

// TestChannelBroadcastParty 는 파티 채널 격리 브로드캐스트를 검증합니다.
func TestChannelBroadcastParty(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)

	a := dialWSURL(t, ts.URL, "/ws")
	defer a.Close()
	b := dialWSURL(t, ts.URL, "/ws")
	defer b.Close()
	c := dialWSURL(t, ts.URL, "/ws")
	defer c.Close()

	expectType(t, a, "welcome")
	expectType(t, b, "welcome")
	expectType(t, c, "welcome")

	mustWriteEnv(t, a, server.Envelope{
		Type: "join", Scope: "channel", ChannelKind: "party", Target: "party-1",
		Payload: payloadJSON("Alice"),
	})
	expectType(t, a, "joined")

	mustWriteEnv(t, b, server.Envelope{
		Type: "join", Scope: "channel", ChannelKind: "party", Target: "party-1",
		Payload: payloadJSON("Bob"),
	})
	expectType(t, b, "joined")
	expectType(t, a, "system")

	mustWriteEnv(t, c, server.Envelope{
		Type: "join", Scope: "channel", ChannelKind: "party", Target: "party-2",
		Payload: payloadJSON("Carol"),
	})
	expectType(t, c, "joined")

	mustWriteEnv(t, a, server.Envelope{
		Type: "send", Scope: "channel", ChannelKind: "party", Target: "party-1",
		Payload: payloadJSON(map[string]string{"text": "party only"}),
	})

	gotA := expectType(t, a, "message")
	gotB := expectType(t, b, "message")
	if gotA.ChannelKind != "party" || gotB.Target != "party-1" {
		t.Fatalf("a=%+v b=%+v", gotA, gotB)
	}

	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("other party must not receive message")
	}
}

// TestChannelGuild 는 길드 채널 send/message 를 검증합니다.
func TestChannelGuild(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)

	a := dialWSURL(t, ts.URL, "/ws")
	defer a.Close()
	b := dialWSURL(t, ts.URL, "/ws")
	defer b.Close()
	expectType(t, a, "welcome")
	expectType(t, b, "welcome")

	mustWriteEnv(t, a, server.Envelope{Type: "join", Scope: "channel", ChannelKind: "guild", Target: "g1"})
	expectType(t, a, "joined")
	mustWriteEnv(t, b, server.Envelope{Type: "join", Scope: "channel", ChannelKind: "guild", Target: "g1"})
	expectType(t, b, "joined")
	expectType(t, a, "system")

	mustWriteEnv(t, a, server.Envelope{
		Type: "send", Scope: "channel", ChannelKind: "guild", Target: "g1",
		Payload: payloadJSON(map[string]string{"text": "guild hi"}),
	})
	expectType(t, a, "message")
	got := expectType(t, b, "message")
	if got.ChannelKind != "guild" {
		t.Fatalf("kind = %q", got.ChannelKind)
	}
}

// TestWhisperToClient 는 1:1 whisper 와 제3자 미수신을 검증합니다.
func TestWhisperToClient(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)

	a := dialWSURL(t, ts.URL, "/ws")
	defer a.Close()
	b := dialWSURL(t, ts.URL, "/ws")
	defer b.Close()
	c := dialWSURL(t, ts.URL, "/ws")
	defer c.Close()

	wa := expectType(t, a, "welcome")
	wb := expectType(t, b, "welcome")
	expectType(t, c, "welcome")

	mustWriteEnv(t, a, server.Envelope{
		Type: "whisper", To: wb.ClientID,
		Payload: payloadJSON(map[string]string{"text": "secret"}),
	})

	gotA := expectType(t, a, "whisper")
	gotB := expectType(t, b, "whisper")
	_ = wa
	_ = gotA
	var p map[string]string
	_ = json.Unmarshal(gotB.Payload, &p)
	if p["text"] != "secret" {
		t.Fatalf("payload = %s", gotB.Payload)
	}

	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("third party must not get whisper")
	}
}

// TestSendRequiresMembership 는 미가입 send 가 error 를 반환하는지 확인합니다.
func TestSendRequiresMembership(t *testing.T) {
	t.Parallel()

	conn := dialWS(t)
	defer conn.Close()
	expectType(t, conn, "welcome")

	mustWriteEnv(t, conn, server.Envelope{
		Type: "send", Scope: "area", Target: "nope",
		Payload: payloadJSON("x"),
	})
	errMsg := expectType(t, conn, "error")
	if errMsg.Error == "" {
		t.Fatal("expected error field")
	}
}

// TestLeaveArea 는 join → leave → left 시퀀스를 검증합니다.
func TestLeaveArea(t *testing.T) {
	t.Parallel()

	conn := dialWS(t)
	defer conn.Close()
	expectType(t, conn, "welcome")

	mustWriteEnv(t, conn, server.Envelope{Type: "join", Scope: "area", Target: "r1"})
	expectType(t, conn, "joined")
	mustWriteEnv(t, conn, server.Envelope{Type: "leave", Scope: "area", Target: "r1"})
	expectType(t, conn, "left")
}

// TestPingPong 은 type=ping → pong 을 검증합니다.
func TestPingPong(t *testing.T) {
	t.Parallel()

	conn := dialWS(t)
	defer conn.Close()
	expectType(t, conn, "welcome")
	mustWriteEnv(t, conn, server.Envelope{Type: "ping"})
	expectType(t, conn, "pong")
}

// TestWebSocketRejectsNonUpgrade 는 일반 HTTP GET /ws 가 업그레이드 실패(비 200)인지 확인합니다.
func TestWebSocketRejectsNonUpgrade(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(s.Close)

	resp, err := http.Get(s.URL + "/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// TestMaxClientsPerArea 는 에리어 인원 한도 초과 시 error 를 검증합니다.
func TestMaxClientsPerArea(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.MaxClientsPerArea = 1
	ts := httptest.NewServer(newTestServer(t, cfg).NewMux())
	t.Cleanup(ts.Close)

	a := dialWSURL(t, ts.URL, "/ws")
	defer a.Close()
	b := dialWSURL(t, ts.URL, "/ws")
	defer b.Close()
	expectType(t, a, "welcome")
	expectType(t, b, "welcome")

	mustWriteEnv(t, a, server.Envelope{Type: "join", Scope: "area", Target: "solo"})
	expectType(t, a, "joined")
	mustWriteEnv(t, b, server.Envelope{Type: "join", Scope: "area", Target: "solo"})
	expectType(t, b, "error")
}
