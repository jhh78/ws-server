package test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jhh78/ws-server/config"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

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

func TestRejectNonTCP(t *testing.T) {
	c := config.Default()
	c.Network = "udp"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

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
