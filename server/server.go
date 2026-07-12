package server

import (
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jhh78/ws-server/config"
	"github.com/jhh78/ws-server/logging"
)

// Server is a receive → JSON → process → deliver WebSocket server.
// No client application is shipped; external programs connect with JSON Envelopes.
type Server struct {
	name     string
	cfg      config.AppConfig
	hub      *Hub
	log      *logging.Logger
	upgrader websocket.Upgrader
}

// New creates a Server and opens log sinks from cfg.Log.
func New(cfg config.AppConfig) (*Server, error) {
	lg, err := logging.New(cfg.Log)
	if err != nil {
		return nil, err
	}
	s := &Server{
		name: cfg.ServerName,
		cfg:  cfg,
		log:  lg,
		hub:  NewHub(cfg.AreaConfig(), cfg.ChannelConfig()),
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
	return s, nil
}

// Logger exposes the logger for tests / extensions.
func (s *Server) Logger() *logging.Logger {
	return s.log
}

// Close closes log sinks.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.log.Close()
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
