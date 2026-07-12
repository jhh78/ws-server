package test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhh78/ws-server/config"
	"github.com/jhh78/ws-server/logging"
	_ "modernc.org/sqlite"
)

func TestSystemAndAccessLogFileMode(t *testing.T) {
	dir := t.TempDir()
	sysPath := filepath.Join(dir, "system.log")
	accPath := filepath.Join(dir, "access.log")

	lg, err := logging.New(config.LogConfig{
		SystemMode: config.LogModeFile,
		AccessMode: config.LogModeFile,
		SystemFile: sysPath,
		AccessFile: accPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	lg.Info("test", "hello system")
	lg.Access(logging.AccessEntry{
		ClientID: "c1", RemoteAddr: "1.2.3.4:5", Action: "connect",
	})
	_ = lg.Close()

	sys, err := os.ReadFile(sysPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sys), "hello system") {
		t.Fatalf("system log = %s", sys)
	}
	acc, err := os.ReadFile(accPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(acc), "action=connect") {
		t.Fatalf("access log = %s", acc)
	}
}

func TestSystemAndAccessLogDBMode(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "log.db")

	lg, err := logging.New(config.LogConfig{
		SystemMode: config.LogModeDB,
		AccessMode: config.LogModeDB,
		DBDriver:   "sqlite",
		DBDSN:      dsn,
	})
	if err != nil {
		t.Fatal(err)
	}

	lg.Error("dbtest", "boom", "detail-x")
	lg.Access(logging.AccessEntry{
		ClientID: "c9", Action: "join", Scope: "area", Target: "m1",
	})
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var msg string
	if err := db.QueryRow(`SELECT message FROM system_logs WHERE level='ERROR' LIMIT 1`).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if msg != "boom" {
		t.Fatalf("message = %q", msg)
	}

	var action, target string
	if err := db.QueryRow(`SELECT action, target FROM access_logs LIMIT 1`).Scan(&action, &target); err != nil {
		t.Fatal(err)
	}
	if action != "join" || target != "m1" {
		t.Fatalf("access = %s %s", action, target)
	}
}

func TestNormalizeLogDBDriver(t *testing.T) {
	cases := map[string]string{
		"sqlite":       config.LogDBSQLite,
		"SQLite3":      config.LogDBSQLite,
		"mysql":        config.LogDBMySQL,
		"MariaDB":      config.LogDBMySQL,
		"postgres":     config.LogDBPostgres,
		"postgresql":   config.LogDBPostgres,
		"pg":           config.LogDBPostgres,
	}
	for in, want := range cases {
		if got := config.NormalizeLogDBDriver(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestLogDBDriverValidation(t *testing.T) {
	c := config.DefaultLog()
	c.SystemMode = config.LogModeDB
	c.AccessMode = config.LogModeOff
	c.DBDriver = "oracle"
	c.DBDSN = "x"
	if err := c.Validate(); err == nil {
		t.Fatal("expected reject unknown driver")
	}
	for _, d := range []string{"sqlite", "mysql", "postgres"} {
		c.DBDriver = config.NormalizeLogDBDriver(d)
		if err := c.Validate(); err != nil {
			t.Fatalf("driver %s: %v", d, err)
		}
	}
}

func TestLogConfigFromEnv(t *testing.T) {
	clearConfigEnv(t)
	// also clear log keys
	for _, k := range []string{
		"SYSTEM_LOG_MODE", "ACCESS_LOG_MODE", "SYSTEM_LOG_FILE", "ACCESS_LOG_FILE",
		"LOG_DB_DRIVER", "LOG_DB_DSN",
	} {
		_ = os.Unsetenv(k)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `
SERVER_NAME=t
LISTEN_ADDR=:18080
SYSTEM_LOG_MODE=db
ACCESS_LOG_MODE=file
ACCESS_LOG_FILE=` + filepath.Join(dir, "a.log") + `
LOG_DB_DSN=` + filepath.Join(dir, "x.db") + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.SystemMode != config.LogModeDB {
		t.Fatalf("system mode = %q", cfg.Log.SystemMode)
	}
	if cfg.Log.AccessMode != config.LogModeFile {
		t.Fatalf("access mode = %q", cfg.Log.AccessMode)
	}

	// open logger with mixed modes
	lg, err := logging.New(cfg.Log)
	if err != nil {
		t.Fatal(err)
	}
	lg.Info("mix", "ok")
	lg.Access(logging.AccessEntry{Action: "ping"})
	_ = lg.Close()

	// wait a tick for fs
	time.Sleep(10 * time.Millisecond)
	if _, err := os.Stat(cfg.Log.AccessFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Log.DBDSN); err != nil {
		t.Fatal(err)
	}
}
