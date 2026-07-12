package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jhh78/ws-server/server"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(server.New(testConfig()).NewMux())
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

// 1) 특정 에리어에 들어가 있는 유저 전체에게 전송
func TestAreaBroadcastToAllMembers(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(server.New(testConfig()).NewMux())
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
	expectType(t, a, "system") // bob joined

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

// 2) 특정 채널(파티/길드 등) 멤버에게만 전송
func TestChannelBroadcastParty(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(server.New(testConfig()).NewMux())
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

	// a,b in same party; c in different party
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

func TestChannelGuild(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(server.New(testConfig()).NewMux())
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

// 귓속말: 특정 client_id 1:1
func TestWhisperToClient(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(server.New(testConfig()).NewMux())
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
	if gotB.To != wb.ClientID || gotA.ClientID != wa.ClientID {
		// from client_id should be a's
	}
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

func TestPingPong(t *testing.T) {
	t.Parallel()

	conn := dialWS(t)
	defer conn.Close()
	expectType(t, conn, "welcome")
	mustWriteEnv(t, conn, server.Envelope{Type: "ping"})
	expectType(t, conn, "pong")
}

func TestWebSocketRejectsNonUpgrade(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(server.New(testConfig()).NewMux())
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

func TestMaxClientsPerArea(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.MaxClientsPerArea = 1
	ts := httptest.NewServer(server.New(cfg).NewMux())
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
