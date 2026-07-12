package logging

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhh78/ws-server/config"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type dbStore struct {
	db     *sql.DB
	driver string // config.LogDBSQLite | LogDBMySQL | LogDBPostgres
}

func openDB(driver, dsn string) (*dbStore, error) {
	driver = config.NormalizeLogDBDriver(driver)
	sqlName, err := sqlDriverName(driver)
	if err != nil {
		return nil, err
	}

	if driver == config.LogDBSQLite {
		if dir := filepath.Dir(dsn); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}

	db, err := sql.Open(sqlName, dsn)
	if err != nil {
		return nil, err
	}
	if driver == config.LogDBSQLite {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
	}

	// Fail fast if remote DB is unreachable
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", driver, err)
	}

	s := &dbStore{db: db, driver: driver}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func sqlDriverName(canonical string) (string, error) {
	switch canonical {
	case config.LogDBSQLite:
		return "sqlite", nil
	case config.LogDBMySQL:
		return "mysql", nil
	case config.LogDBPostgres:
		return "postgres", nil
	default:
		return "", fmt.Errorf("unsupported LOG_DB_DRIVER %q (use sqlite|mysql|postgres)", canonical)
	}
}

func (s *dbStore) migrate() error {
	for _, q := range createTableSQL(s.driver) {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w\nSQL: %s", err, q)
		}
	}
	// Indexes: ignore "already exists" style errors across engines
	for _, q := range createIndexSQL(s.driver) {
		if _, err := s.db.Exec(q); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "already exists") ||
				strings.Contains(msg, "duplicate key") ||
				strings.Contains(msg, "duplicate") {
				continue
			}
			return fmt.Errorf("migrate index: %w\nSQL: %s", err, q)
		}
	}
	return nil
}

func createTableSQL(driver string) []string {
	switch driver {
	case config.LogDBMySQL:
		return []string{
			`CREATE TABLE IF NOT EXISTS system_logs (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at VARCHAR(40) NOT NULL,
  level VARCHAR(16) NOT NULL,
  component VARCHAR(64),
  message TEXT NOT NULL,
  detail TEXT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS access_logs (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at VARCHAR(40) NOT NULL,
  client_id VARCHAR(64),
  remote_addr VARCHAR(128),
  action VARCHAR(32) NOT NULL,
  type VARCHAR(32),
  scope VARCHAR(32),
  target VARCHAR(255),
  channel_kind VARCHAR(32),
  detail TEXT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
	case config.LogDBPostgres:
		return []string{
			`CREATE TABLE IF NOT EXISTS system_logs (
  id BIGSERIAL PRIMARY KEY,
  created_at TEXT NOT NULL,
  level TEXT NOT NULL,
  component TEXT,
  message TEXT NOT NULL,
  detail TEXT
)`,
			`CREATE TABLE IF NOT EXISTS access_logs (
  id BIGSERIAL PRIMARY KEY,
  created_at TEXT NOT NULL,
  client_id TEXT,
  remote_addr TEXT,
  action TEXT NOT NULL,
  type TEXT,
  scope TEXT,
  target TEXT,
  channel_kind TEXT,
  detail TEXT
)`,
		}
	default: // sqlite
		return []string{
			`CREATE TABLE IF NOT EXISTS system_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  level TEXT NOT NULL,
  component TEXT,
  message TEXT NOT NULL,
  detail TEXT
)`,
			`CREATE TABLE IF NOT EXISTS access_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  client_id TEXT,
  remote_addr TEXT,
  action TEXT NOT NULL,
  type TEXT,
  scope TEXT,
  target TEXT,
  channel_kind TEXT,
  detail TEXT
)`,
		}
	}
}

func createIndexSQL(driver string) []string {
	// IF NOT EXISTS works on SQLite and PostgreSQL; MySQL 8.0+ also supports it for indexes in some versions.
	// We still tolerate duplicate errors in migrate().
	if driver == config.LogDBMySQL {
		return []string{
			`CREATE INDEX idx_system_logs_created ON system_logs (created_at)`,
			`CREATE INDEX idx_access_logs_created ON access_logs (created_at)`,
		}
	}
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_system_logs_created ON system_logs (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_created ON access_logs (created_at)`,
	}
}

func (s *dbStore) insertSystem(t time.Time, level, component, message, detail string) error {
	q := s.rebind(`INSERT INTO system_logs (created_at, level, component, message, detail) VALUES (?, ?, ?, ?, ?)`)
	_, err := s.db.Exec(q, t.Format(time.RFC3339Nano), level, component, message, detail)
	return err
}

func (s *dbStore) insertAccess(t time.Time, e AccessEntry) error {
	q := s.rebind(`INSERT INTO access_logs (created_at, client_id, remote_addr, action, type, scope, target, channel_kind, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := s.db.Exec(q,
		t.Format(time.RFC3339Nano),
		e.ClientID, e.RemoteAddr, e.Action, e.Type, e.Scope, e.Target, e.ChannelKind, e.Detail,
	)
	return err
}

// rebind converts ? placeholders to $1,$2,... for PostgreSQL.
func (s *dbStore) rebind(q string) string {
	if s.driver != config.LogDBPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(fmt.Sprintf("%d", n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func (s *dbStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
