package test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jhh78/ws-server/config"
)

// projectRoot 는 저장소 루트 경로를 반환합니다 (이 파일 기준 상위).
//
// Parameters:
//   - t: testing.T
//
// Returns:
//   - string: 절대 경로에 가까운 정리된 루트
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

// clearConfigEnv 는 테스트 간 오염을 막기 위해 주요 설정 OS env 를 Unset 합니다.
//
// Parameters:
//   - t: testing.T
func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"SERVER_NAME", "NETWORK", "LISTEN_ADDR", "WS_PATH", "HEALTH_PATH",
		"MAX_CLIENTS_PER_AREA", "MAX_AREAS", "READ_BUFFER_SIZE", "WRITE_BUFFER_SIZE",
		"ALLOW_ORIGINS", "MAX_CHANNELS", "MAX_CLIENTS_PER_CHANNEL",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

// TestLoadSampleEnv 는 커밋된 sample.env 가 로드·검증되는지 확인합니다.
func TestLoadSampleEnv(t *testing.T) {
	clearConfigEnv(t)
	root := projectRoot(t)
	cfg, err := config.Load(filepath.Join(root, "sample.env"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("addr = %q", cfg.ListenAddr)
	}
	if cfg.Network != "tcp" {
		t.Fatalf("network = %q", cfg.Network)
	}
	if cfg.WSPath != "/ws" || cfg.HealthPath != "/health" {
		t.Fatalf("paths = %q %q", cfg.WSPath, cfg.HealthPath)
	}
	if cfg.MaxClientsPerArea != 500 || cfg.MaxChannels != 20000 {
		t.Fatalf("limits area=%d ch=%d", cfg.MaxClientsPerArea, cfg.MaxChannels)
	}
}

// TestCopySampleToDotEnv 는 sample.env → 임시 .env 복사 후 Load 를 검증합니다.
func TestCopySampleToDotEnv(t *testing.T) {
	clearConfigEnv(t)
	root := projectRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "sample.env"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dst)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName == "" {
		t.Fatal("expected SERVER_NAME")
	}
}

// TestPartialEnvUsesDefaults 는 일부 키만 있을 때 Default 가 채워지는지 확인합니다.
func TestPartialEnvUsesDefaults(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("LISTEN_ADDR=:9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("addr = %q", cfg.ListenAddr)
	}
	def := config.Default()
	if cfg.WSPath != def.WSPath {
		t.Fatalf("ws default = %q", cfg.WSPath)
	}
}

// TestInvalidIntegerFails 는 잘못된 정수 env 가 Load 오류를 내는지 확인합니다.
func TestInvalidIntegerFails(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("MAX_AREAS=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected error")
	}
}

// TestOSEnvOverridesFile 은 이미 설정된 OS env 가 파일 값보다 우선함을 검증합니다.
func TestOSEnvOverridesFile(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LISTEN_ADDR=:1111\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LISTEN_ADDR", ":2222")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":2222" {
		t.Fatalf("got %q", cfg.ListenAddr)
	}
}

// TestRejectNonTCP 는 NETWORK=udp 등 비 TCP 를 Validate 가 거절하는지 확인합니다.
func TestRejectNonTCP(t *testing.T) {
	c := config.Default()
	c.Network = "udp"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

// TestLoadEmptyUsesDefaults 는 path="" 일 때 Default 기반 로드를 검증합니다.
func TestLoadEmptyUsesDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network != "tcp" {
		t.Fatalf("network = %q", cfg.Network)
	}
}
