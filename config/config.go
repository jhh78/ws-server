// Package config 는 sample.env / .env 기반 런타임 설정을 로드·검증합니다.
//
// 워크플로:
//
//	cp sample.env .env  → 서버 기동
//
// 하나의 AppConfig 가 게임·채팅 공통(에리어 + 채널) 프로토콜을 담당합니다.
// 우선순위: CLI(-addr) > 이미 설정된 OS 환경변수 > -env 파일 > Default().
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// KnownKeys 는 Load 가 해석하는 환경 변수 이름 집합입니다.
//
// 여기에 없는 키는 AppConfig.Extra 에 보관되어 향후 확장에 쓰일 수 있습니다.
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
	// logging (시스템 / 액세스 각각 sample.env 에 등록)
	"SYSTEM_LOG_MODE": {},
	"ACCESS_LOG_MODE": {},
	"SYSTEM_LOG_FILE": {},
	"ACCESS_LOG_FILE": {},
	"LOG_DB_DRIVER":   {},
	"LOG_DB_DSN":      {},
}

// AppConfig 는 실시간 서버의 전체 런타임 설정입니다.
//
// 필드 그룹:
//   - 서버 식별·TCP/WebSocket
//   - 에리어·채널 한도
//   - 시스템/액세스 로그 (Log)
//   - Extra: 알 수 없는 키
type AppConfig struct {
	// ServerName 은 로그·welcome 에 표시되는 자유 형식 이름입니다.
	ServerName string

	// Network 은 net.Listen 네트워크 (tcp | tcp4 | tcp6).
	Network string
	// ListenAddr 은 바인드 주소 (예: :8080, 0.0.0.0:8080).
	ListenAddr string

	// WSPath 는 WebSocket 업그레이드 경로 (/ 로 시작).
	WSPath string
	// HealthPath 는 헬스 체크 경로 (WSPath 와 달라야 함).
	HealthPath string
	// ReadBufferSize 는 gorilla/websocket 읽기 버퍼(바이트).
	ReadBufferSize int
	// WriteBufferSize 는 gorilla/websocket 쓰기 버퍼(바이트).
	WriteBufferSize int
	// AllowOrigins 는 브라우저 Origin 허용 (* 또는 쉼표 구분 목록).
	AllowOrigins string

	// MaxClientsPerArea 는 에리어당 최대 인원 (0 = 무제한).
	MaxClientsPerArea int
	// MaxAreas 는 동시 에리어 수 (0 = 무제한).
	MaxAreas int

	// MaxChannels 는 동시 채널 수 (0 = 무제한).
	MaxChannels int
	// MaxClientsPerChannel 는 채널당 최대 인원 (0 = 무제한).
	MaxClientsPerChannel int

	// Log 는 시스템·액세스 로그 싱크 설정입니다.
	Log LogConfig

	// Extra 는 파일/OS 의 알 수 없는 키 값입니다 (호환 확장용).
	Extra map[string]string
}

// Default 는 키가 생략되었을 때 쓰는 안전한 기본값을 반환합니다.
//
// Returns:
//   - AppConfig: sample.env 기본과 동일한 계열의 기본값
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
		Log:                  DefaultLog(),
		Extra:                map[string]string{},
	}
}

// Load 는 path 의 env 파일을 프로세스 환경에 반영(비어 있지 않으면)한 뒤 AppConfig 를 구성합니다.
//
// Parameters:
//   - path: env 파일 경로. 빈 문자열이면 파일 로드 없이 OS env + Default 만 사용
//
// Returns:
//   - AppConfig: 검증 통과한 설정
//   - error: 파일 오류, 정수 파싱 실패, Validate 실패 등
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
	if c.Log, err = LoadLog(); err != nil {
		return AppConfig{}, fmt.Errorf("log: %w", err)
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

// Validate 는 TCP/HTTP/한도 불변식을 검사합니다.
//
// Returns:
//   - error: NETWORK, LISTEN_ADDR, 경로, 버퍼·한도 음수 등 위반 시
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

// SetListenAddr 은 CLI -addr 로 LISTEN_ADDR 을 덮어씁니다.
//
// Parameters:
//   - addr: 비어 있지 않을 때만 ListenAddr 에 적용
func (c *AppConfig) SetListenAddr(addr string) {
	if strings.TrimSpace(addr) != "" {
		c.ListenAddr = addr
	}
}

// ExtraString 은 Extra 또는 라이브 OS env 에서 문자열을 조회합니다.
//
// Parameters:
//   - key: 환경 변수 이름
//   - def: 없을 때 기본값
//
// Returns:
//   - string: 발견된 값 또는 def
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

// ExtraInt 는 ExtraString 결과를 정수로 파싱합니다.
//
// Parameters:
//   - key: 환경 변수 이름
//   - def: 파싱 실패·부재 시 반환값
//
// Returns:
//   - int: 파싱된 값 또는 def
//   - bool: 키에서 유효한 정수를 읽었으면 true
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

// AreaConfig 는 허브용 에리어 한도 슬라이스를 반환합니다.
//
// Returns:
//   - AreaLimits: MaxClientsPerArea, MaxAreas
func (c AppConfig) AreaConfig() AreaLimits {
	return AreaLimits{MaxClientsPerArea: c.MaxClientsPerArea, MaxAreas: c.MaxAreas}
}

// ChannelConfig 는 허브용 채널 한도 슬라이스를 반환합니다.
//
// Returns:
//   - ChannelLimits: MaxChannels, MaxClientsPerChannel
func (c AppConfig) ChannelConfig() ChannelLimits {
	return ChannelLimits{MaxChannels: c.MaxChannels, MaxClientsPerChannel: c.MaxClientsPerChannel}
}

// AreaLimits 는 허브에 넘기는 에리어 한도입니다 (전체 AppConfig 결합 회피).
type AreaLimits struct {
	// MaxClientsPerArea 는 에리어당 최대 클라이언트 수 (0 = 무제한).
	MaxClientsPerArea int
	// MaxAreas 는 동시 존재 가능한 에리어 수 (0 = 무제한).
	MaxAreas int
}

// ChannelLimits 는 허브에 넘기는 채널 한도입니다.
type ChannelLimits struct {
	// MaxChannels 는 동시 채널 수 (0 = 무제한).
	MaxChannels int
	// MaxClientsPerChannel 는 채널당 최대 인원 (0 = 무제한).
	MaxClientsPerChannel int
}
