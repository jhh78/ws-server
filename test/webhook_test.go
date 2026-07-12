package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jhh78/ws-server/server"
)

// TestWebhookReceivesConnectAndJoin 는 WEBHOOK_URL 로 connect·join 이 POST 되는지 검증합니다.
func TestWebhookReceivesConnectAndJoin(t *testing.T) {
	var mu sync.Mutex
	var events []string

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var p server.WebhookPayload
		if err := json.Unmarshal(body, &p); err != nil {
			t.Errorf("json: %v", err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		events = append(events, p.Event)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hook.Close)

	cfg := testConfig(t)
	cfg.WebhookURL = `["` + hook.URL + `"]`
	cfg.WebhookTimeoutMs = 2000

	ts := httptest.NewServer(newTestServer(t, cfg).NewMux())
	t.Cleanup(ts.Close)

	conn := dialWSURL(t, ts.URL, "/ws")
	defer conn.Close()
	expectType(t, conn, "welcome")
	mustWriteEnv(t, conn, server.Envelope{
		Type: "join", Scope: "area", Target: "lobby",
		Payload: payloadJSON(map[string]string{"name": "w"}),
	})
	expectType(t, conn, "joined")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(events)
		hasConnect, hasJoin := false, false
		for _, e := range events {
			if e == "connect" {
				hasConnect = true
			}
			if e == "join" {
				hasJoin = true
			}
		}
		mu.Unlock()
		if n >= 2 && hasConnect && hasJoin {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("events = %v (want connect + join)", events)
}
