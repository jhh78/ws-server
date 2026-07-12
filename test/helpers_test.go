// Package test 는 ws-server 프로토콜·설정·로깅 통합/단위 테스트입니다.
//
// 제품 클라이언트 앱이 아니며, 서버 규격·중계 동작 검증용입니다.
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

// testConfig 는 테스트용 AppConfig 를 반환합니다.
//
// 로그는 t.TempDir() 아래로 격리하여 저장소 logs/ 를 오염시키지 않습니다.
//
// Parameters:
//   - t: testing.T
//
// Returns:
//   - config.AppConfig: Default 기반 + file 로그 경로
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

// newTestServer 는 cfg 로 Server 를 만들고 t.Cleanup 에 Close 를 등록합니다.
//
// Parameters:
//   - t: testing.T
//   - cfg: 서버 설정
//
// Returns:
//   - *server.Server: 테스트 서버
func newTestServer(t *testing.T, cfg config.AppConfig) *server.Server {
	t.Helper()
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// dialWS 는 임시 httptest 서버에 WebSocket 으로 연결합니다.
//
// Parameters:
//   - t: testing.T
//
// Returns:
//   - *websocket.Conn: 열린 연결 (호출자가 Close)
func dialWS(t *testing.T) *websocket.Conn {
	t.Helper()
	ts := httptest.NewServer(newTestServer(t, testConfig(t)).NewMux())
	t.Cleanup(ts.Close)
	return dialWSURL(t, ts.URL, "/ws")
}

// dialWSURL 은 http(s) 베이스 URL 을 ws 로 바꿔 지정 경로에 Dial 합니다.
//
// Parameters:
//   - t: testing.T
//   - httpURL: httptest.URL
//   - wsPath: 예 "/ws"
//
// Returns:
//   - *websocket.Conn: 연결
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

// mustWriteEnv 는 Envelope 를 Text 프레임으로 전송합니다. 실패 시 t.Fatal.
//
// Parameters:
//   - t: testing.T
//   - conn: WebSocket
//   - m: 송신 Envelope
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

// readEnv 는 한 프레임을 읽어 Envelope 로 파싱합니다 (읽기 데드라인 3s).
//
// Parameters:
//   - t: testing.T
//   - conn: WebSocket
//
// Returns:
//   - server.Envelope: 파싱 결과
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

// expectType 은 typ 과 일치하는 Envelope 를 최대 10 프레임 내에서 기다립니다.
//
// Parameters:
//   - t: testing.T
//   - conn: WebSocket
//   - typ: 기대 type 문자열
//
// Returns:
//   - server.Envelope: 일치한 메시지
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

// payloadJSON 은 값을 Envelope.Payload 용 RawMessage 로 마샬합니다.
//
// Parameters:
//   - v: 직렬화할 값
//
// Returns:
//   - json.RawMessage: JSON 바이트
func payloadJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
