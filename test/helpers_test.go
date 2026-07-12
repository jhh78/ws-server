package test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/config"
	"github.com/jhh78/ws-server/server"
)

func testConfig() config.AppConfig {
	return config.Default()
}

func dialWS(t *testing.T) *websocket.Conn {
	t.Helper()
	s := httptest.NewServer(server.New(testConfig()).NewMux())
	t.Cleanup(s.Close)
	return dialWSURL(t, s.URL, "/ws")
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
