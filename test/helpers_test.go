package test

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/config"
	"github.com/jhh78/ws-server/server"
)

func testConfig(t *testing.T) config.AppConfig {
	t.Helper()
	cfg := config.Default()
	// Isolate logs under temp dir so tests do not touch repo logs/
	dir := t.TempDir()
	cfg.Log.SystemMode = config.LogModeFile
	cfg.Log.AccessMode = config.LogModeFile
	cfg.Log.SystemFile = filepath.Join(dir, "system.log")
	cfg.Log.AccessFile = filepath.Join(dir, "access.log")
	return cfg
}

func newTestServer(t *testing.T, cfg config.AppConfig) *server.Server {
	t.Helper()
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func dialWS(t *testing.T) *websocket.Conn {
	t.Helper()
	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)
	return dialWSURL(t, ts.URL, "/ws")
}

func dialWSURL(t *testing.T, httpURL, wsPath string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http") + wsPath
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(%s): %v", wsURL, err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return conn
}

func mustWriteEnv(t *testing.T, conn *websocket.Conn, m server.Envelope) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

func readEnv(t *testing.T, conn *websocket.Conn) server.Envelope {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var m server.Envelope
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json: %v body=%s", err, data)
	}
	return m
}

func expectType(t *testing.T, conn *websocket.Conn, typ string) server.Envelope {
	t.Helper()
	for i := 0; i < 10; i++ {
		m := readEnv(t, conn)
		if m.Type == typ {
			return m
		}
	}
	t.Fatalf("did not receive type %q", typ)
	return server.Envelope{}
}

func payloadJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
