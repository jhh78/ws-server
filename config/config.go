// Package config loads the active .env into AppConfig.
//
// Workflow: cp sample.env .env  then start the server.
// One config serves both game and chat (shared area + channel protocol).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// KnownKeys are interpreted by Load. Other keys go into Extra.
var KnownKeys = map[string]struct{}{
	"SERVER_NAME":             {},
	"NETWORK":                 {},
	"LISTEN_ADDR":             {},
	"WS_PATH":                 {},
	"HEALTH_PATH":             {},
	"READ_BUFFER_SIZE":        {},
	"WRITE_BUFFER_SIZE":       {},
	"ALLOW_ORIGINS":           {},
	"MAX_CLIENTS_PER_AREA":    {},
	"MAX_AREAS":               {},
	"MAX_CHANNELS":            {},
	"MAX_CLIENTS_PER_CHANNEL": {},
}

// AppConfig is the full runtime skeleton for the realtime server.
type AppConfig struct {
	// ServerName is a free-form label in logs / welcome (e.g. game-chat).
	ServerName string

	Network    string
	ListenAddr string

	WSPath          string
	HealthPath      string
	ReadBufferSize  int
	WriteBufferSize int
	AllowOrigins    string

	// Area limits (map / zone / lobby). 0 = unlimited.
	MaxClientsPerArea int
	MaxAreas          int

	// Channel limits (party / guild / whisper room / custom). 0 = unlimited.
	MaxChannels          int
	MaxClientsPerChannel int

	// Extra holds unknown keys from the env file (forward-compatible).
	Extra map[string]string
}

// Default returns safe defaults when keys are omitted.
func Default() AppConfig {
	return AppConfig{
		ServerName:           "ws-server",
		Network:              "tcp",
		ListenAddr:           ":8080",
		WSPath:               "/ws",
		HealthPath:           "/health",
		ReadBufferSize:       4096,
		WriteBufferSize:      4096,
		AllowOrigins:         "*",
		MaxClientsPerArea:    500,
		MaxAreas:             10000,
		MaxChannels:          20000,
		MaxClientsPerChannel: 200,
		Extra:                map[string]string{},
	}
}

// Load reads path into process env (if non-empty), then builds AppConfig.
func Load(path string) (AppConfig, error) {
	var st loadState
	var err error
	if path != "" {
		st, err = LoadFile(path)
		if err != nil {
			return AppConfig{}, err
		}
	}

	c := Default()
	c.Extra = map[string]string{}

	c.ServerName = envString("SERVER_NAME", c.ServerName)
	c.Network = strings.ToLower(envString("NETWORK", c.Network))
	c.ListenAddr = envString("LISTEN_ADDR", c.ListenAddr)
	c.WSPath = envString("WS_PATH", c.WSPath)
	c.HealthPath = envString("HEALTH_PATH", c.HealthPath)
	c.AllowOrigins = envString("ALLOW_ORIGINS", c.AllowOrigins)

	if c.ReadBufferSize, err = envIntStrict("READ_BUFFER_SIZE", c.ReadBufferSize); err != nil {
		return AppConfig{}, err
	}
	if c.WriteBufferSize, err = envIntStrict("WRITE_BUFFER_SIZE", c.WriteBufferSize); err != nil {
		return AppConfig{}, err
	}
	if c.MaxClientsPerArea, err = envIntStrict("MAX_CLIENTS_PER_AREA", c.MaxClientsPerArea); err != nil {
		return AppConfig{}, err
	}
	if c.MaxAreas, err = envIntStrict("MAX_AREAS", c.MaxAreas); err != nil {
		return AppConfig{}, err
	}
	if c.MaxChannels, err = envIntStrict("MAX_CHANNELS", c.MaxChannels); err != nil {
		return AppConfig{}, err
	}
	if c.MaxClientsPerChannel, err = envIntStrict("MAX_CLIENTS_PER_CHANNEL", c.MaxClientsPerChannel); err != nil {
		return AppConfig{}, err
	}

	for k, v := range st.fromFile {
		if _, known := KnownKeys[k]; !known {
			c.Extra[k] = v
		}
	}
	for k := range st.skippedOS {
		if _, known := KnownKeys[k]; known {
			continue
		}
		if v, ok := os.LookupEnv(k); ok {
			c.Extra[k] = v
		}
	}

	if err := c.Validate(); err != nil {
		return AppConfig{}, err
	}
	return c, nil
}

// Validate checks TCP/HTTP/limit invariants for a runnable server.
func (c AppConfig) Validate() error {
	switch c.Network {
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("NETWORK must be tcp/tcp4/tcp6 (got %q)", c.Network)
	}
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("LISTEN_ADDR is required")
	}
	if c.WSPath == "" || c.WSPath[0] != '/' {
		return fmt.Errorf("WS_PATH must start with / (got %q)", c.WSPath)
	}
	if c.HealthPath == "" || c.HealthPath[0] != '/' {
		return fmt.Errorf("HEALTH_PATH must start with / (got %q)", c.HealthPath)
	}
	if c.WSPath == c.HealthPath {
		return fmt.Errorf("WS_PATH and HEALTH_PATH must differ")
	}
	if c.ReadBufferSize < 0 || c.WriteBufferSize < 0 {
		return fmt.Errorf("buffer sizes must be >= 0")
	}
	if c.MaxClientsPerArea < 0 || c.MaxAreas < 0 || c.MaxChannels < 0 || c.MaxClientsPerChannel < 0 {
		return fmt.Errorf("limit values must be >= 0")
	}
	return nil
}

// SetListenAddr overrides LISTEN_ADDR (CLI -addr).
func (c *AppConfig) SetListenAddr(addr string) {
	if strings.TrimSpace(addr) != "" {
		c.ListenAddr = addr
	}
}

// ExtraString returns Extra or live env, else def.
func (c AppConfig) ExtraString(key, def string) string {
	if c.Extra != nil {
		if v, ok := c.Extra[key]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	if v, ok := lookupNonEmpty(key); ok {
		return v
	}
	return def
}

// ExtraInt parses ExtraString as int.
func (c AppConfig) ExtraInt(key string, def int) (int, bool) {
	raw := c.ExtraString(key, "")
	if raw == "" {
		return def, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def, false
	}
	return n, true
}

// AreaConfig projects area limits for the hub.
func (c AppConfig) AreaConfig() AreaLimits {
	return AreaLimits{MaxClientsPerArea: c.MaxClientsPerArea, MaxAreas: c.MaxAreas}
}

// ChannelConfig projects channel limits for the hub.
func (c AppConfig) ChannelConfig() ChannelLimits {
	return ChannelLimits{MaxChannels: c.MaxChannels, MaxClientsPerChannel: c.MaxClientsPerChannel}
}

// AreaLimits is the hub-facing area slice of config (avoids full AppConfig coupling).
type AreaLimits struct {
	MaxClientsPerArea int
	MaxAreas          int
}

// ChannelLimits is the hub-facing channel slice of config.
type ChannelLimits struct {
	MaxChannels          int
	MaxClientsPerChannel int
}
