// Package server 는 TCP 위 WebSocket 실시간 중계 허브입니다.
//
// 파이프라인: 수신 → JSON Envelope → (로그·선택 웹훅) → 라우팅/중계 → 송신.
// 비즈니스 로직은 두지 않으며, WEBHOOK_URL 이 있으면 이벤트를 외부로 POST 합니다.
package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/config"
	"github.com/jhh78/ws-server/logging"
)

// Server 는 수신 → JSON → 중계 → 전달 WebSocket 서버입니다.
//
// 클라이언트 애플리케이션은 포함되지 않으며, 외부 프로그램이 Envelope JSON 으로 연결합니다.
type Server struct {
	name     string
	cfg      config.AppConfig
	hub      *Hub
	log      *logging.Logger
	upgrader websocket.Upgrader
}

// New 는 서버를 생성하고 로그 싱크·선택적 웹훅을 연결합니다.
//
// Parameters:
//   - cfg: 검증된 AppConfig
//
// Returns:
//   - *Server: 기동 가능한 서버
//   - error: 로그 싱크 오픈 실패
func New(cfg config.AppConfig) (*Server, error) {
	lg, err := logging.New(cfg.Log)
	if err != nil {
		return nil, err
	}
	hub := NewHub(cfg.AreaConfig(), cfg.ChannelConfig())
	wh := NewWebhook(cfg.WebhookURLs(), cfg.ServerName, lg)
	hub.SetWebhook(wh)

	s := &Server{
		name: cfg.ServerName,
		cfg:  cfg,
		log:  lg,
		hub:  hub,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     s.checkOrigin,
	}
	lg.Info("server", "logger ready",
		"system_mode="+cfg.Log.SystemMode,
		"access_mode="+cfg.Log.AccessMode,
	)
	if wh != nil {
		lg.Info("server", "webhook enabled", fmt.Sprintf("urls=%d", len(wh.URLs)))
	} else {
		lg.Info("server", "webhook disabled", "WEBHOOK_URL empty")
	}
	return s, nil
}

// Logger 는 테스트·확장용 로거를 노출합니다.
//
// Returns:
//   - *logging.Logger: 서버 소유 로거
func (s *Server) Logger() *logging.Logger {
	return s.log
}

// Close 는 로그 싱크를 닫습니다.
//
// Returns:
//   - error: 로거 Close 오류
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.log.Close()
}

// checkOrigin 은 ALLOW_ORIGINS 에 따라 브라우저 Origin 을 허용합니다.
//
// Parameters:
//   - r: 업그레이드 HTTP 요청
//
// Returns:
//   - bool: 허용 여부 (* 또는 목록 일치)
func (s *Server) checkOrigin(r *http.Request) bool {
	allow := strings.TrimSpace(s.cfg.AllowOrigins)
	if allow == "" || allow == "*" {
		return true
	}
	origin := r.Header.Get("Origin")
	for _, o := range strings.Split(allow, ",") {
		if strings.TrimSpace(o) == origin {
			return true
		}
	}
	return false
}

// wsHandler 는 HTTP→WebSocket 업그레이드 후 클라이언트를 등록하고 펌프를 시작합니다.
//
// Parameters:
//   - w: 응답 작성기
//   - r: 업그레이드 요청
func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("ws", "upgrade failed", err.Error(), "remote="+r.RemoteAddr)
		s.log.Access(logging.AccessEntry{
			RemoteAddr: r.RemoteAddr,
			Action:     "upgrade_fail",
			Detail:     err.Error(),
		})
		return
	}

	client := newClient(s.hub, conn, r.RemoteAddr, s.log)
	s.hub.register(client)
	extOnConnect(client)

	client.reply(Envelope{
		Type:     "welcome",
		ClientID: client.id,
		Payload: rawJSON(map[string]any{
			"server":  s.name,
			"message": "connected",
			"protocol": map[string]any{
				"version":   1,
				"encoding":  "json",
				"transport": "websocket",
				"types":     []string{"join", "leave", "send", "whisper", "ping"},
				"scopes":    []string{ScopeArea, ScopeChannel},
				"channel_kinds": []string{
					ChannelParty, ChannelGuild, ChannelWhisper, ChannelCustom,
				},
			},
		}),
	})

	go client.writePump()
	client.readPump()
}

// healthHandler 는 plain "OK" 헬스 응답을 반환합니다.
//
// Parameters:
//   - w: 응답 작성기
//   - r: 요청 (미사용)
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// NewMux 는 WS_PATH / HEALTH_PATH 핸들러가 등록된 ServeMux 를 만듭니다.
//
// Returns:
//   - *http.ServeMux: 라우팅 테이블
func (s *Server) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.WSPath, s.wsHandler)
	mux.HandleFunc(s.cfg.HealthPath, s.healthHandler)
	return mux
}

// Hub 는 멤버십 허브 참조를 반환합니다 (테스트용).
//
// Returns:
//   - *Hub: 서버 소유 허브
func (s *Server) Hub() *Hub {
	return s.hub
}

// ListenAndServe 는 TCP 를 바인드하고 HTTP/WebSocket 을 서빙합니다 (RFC 6455 업그레이드).
//
// Returns:
//   - error: Listen 또는 Serve 실패 (정상 블로킹 시 반환 안 함)
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen(s.cfg.Network, s.cfg.ListenAddr)
	if err != nil {
		s.log.Error("server", "listen failed", err.Error())
		return err
	}
	s.log.Info("server", "listening",
		"network="+s.cfg.Network,
		"addr="+s.cfg.ListenAddr,
		"ws="+s.cfg.WSPath,
	)
	log.Printf("server=%s network=%s listen=%s ws=%s health=%s",
		s.name, s.cfg.Network, s.cfg.ListenAddr, s.cfg.WSPath, s.cfg.HealthPath)
	return http.Serve(ln, s.NewMux())
}
