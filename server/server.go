package server

import (
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/config"
)

// Server is a receive → JSON → process → deliver WebSocket server.
// No client application is shipped; external programs connect with JSON Envelopes.
type Server struct {
	name     string
	cfg      config.AppConfig
	hub      *Hub
	upgrader websocket.Upgrader
}

// New creates a Server from AppConfig.
func New(cfg config.AppConfig) *Server {
	s := &Server{
		name: cfg.ServerName,
		cfg:  cfg,
		hub:  NewHub(cfg.AreaConfig(), cfg.ChannelConfig()),
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     s.checkOrigin,
	}
	return s
}

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

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	// RFC 6455: Upgrade only succeeds with proper Connection/Upgrade headers
	// (enforced by gorilla/websocket).
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := newClient(s.hub, conn, r.RemoteAddr)
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

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.WSPath, s.wsHandler)
	mux.HandleFunc(s.cfg.HealthPath, s.healthHandler)
	return mux
}

func (s *Server) Hub() *Hub {
	return s.hub
}

// ListenAndServe binds TCP and serves HTTP/WebSocket (RFC 6455 upgrade path).
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen(s.cfg.Network, s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	log.Printf("server=%s network=%s listen=%s ws=%s health=%s",
		s.name, s.cfg.Network, s.cfg.ListenAddr, s.cfg.WSPath, s.cfg.HealthPath)
	return http.Serve(ln, s.NewMux())
}
