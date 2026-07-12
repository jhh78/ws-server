package config

import (
	"fmt"
	"strings"
)

// Log modes for system / access sinks.
const (
	LogModeOff  = "off"
	LogModeFile = "file"
	LogModeDB   = "db"
)

// Supported LOG_DB_DRIVER values (normalized).
const (
	LogDBSQLite     = "sqlite"
	LogDBMySQL      = "mysql"
	LogDBPostgres   = "postgres"
)

// LogConfig controls system and access logging.
// System and access each choose file or database independently.
type LogConfig struct {
	// SystemMode: off | file | db
	SystemMode string
	// AccessMode: off | file | db
	AccessMode string

	SystemFile string // used when SystemMode=file
	AccessFile string // used when AccessMode=file

	// DB settings when either mode is db
	// DBDriver: sqlite | mysql | postgres (aliases: postgresql, pg, sqlite3)
	DBDriver string
	// DBDSN examples:
	//   sqlite:    logs/ws-server.db
	//   mysql:     user:pass@tcp(127.0.0.1:3306)/ws_logs?parseTime=true
	//   postgres:  postgres://user:pass@127.0.0.1:5432/ws_logs?sslmode=disable
	DBDSN string
}

func DefaultLog() LogConfig {
	return LogConfig{
		SystemMode: LogModeFile,
		AccessMode: LogModeFile,
		SystemFile: "logs/system.log",
		AccessFile: "logs/access.log",
		DBDriver:   LogDBSQLite,
		DBDSN:      "logs/ws-server.db",
	}
}

func LoadLog() (LogConfig, error) {
	d := DefaultLog()
	c := LogConfig{
		SystemMode: strings.ToLower(envString("SYSTEM_LOG_MODE", d.SystemMode)),
		AccessMode: strings.ToLower(envString("ACCESS_LOG_MODE", d.AccessMode)),
		SystemFile: envString("SYSTEM_LOG_FILE", d.SystemFile),
		AccessFile: envString("ACCESS_LOG_FILE", d.AccessFile),
		DBDriver:   NormalizeLogDBDriver(envString("LOG_DB_DRIVER", d.DBDriver)),
		DBDSN:      envString("LOG_DB_DSN", d.DBDSN),
	}
	return c, c.Validate()
}

// NormalizeLogDBDriver maps aliases to canonical driver names.
func NormalizeLogDBDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sqlite", "sqlite3":
		return LogDBSQLite
	case "mysql", "mariadb":
		return LogDBMySQL
	case "postgres", "postgresql", "pg", "pgsql":
		return LogDBPostgres
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func (c LogConfig) Validate() error {
	if err := validateLogMode("SYSTEM_LOG_MODE", c.SystemMode); err != nil {
		return err
	}
	if err := validateLogMode("ACCESS_LOG_MODE", c.AccessMode); err != nil {
		return err
	}
	if c.SystemMode == LogModeFile && strings.TrimSpace(c.SystemFile) == "" {
		return fmt.Errorf("SYSTEM_LOG_FILE is required when SYSTEM_LOG_MODE=file")
	}
	if c.AccessMode == LogModeFile && strings.TrimSpace(c.AccessFile) == "" {
		return fmt.Errorf("ACCESS_LOG_FILE is required when ACCESS_LOG_MODE=file")
	}
	if c.SystemMode == LogModeDB || c.AccessMode == LogModeDB {
		switch c.DBDriver {
		case LogDBSQLite, LogDBMySQL, LogDBPostgres:
		default:
			return fmt.Errorf("LOG_DB_DRIVER must be sqlite|mysql|postgres (got %q)", c.DBDriver)
		}
		if strings.TrimSpace(c.DBDSN) == "" {
			return fmt.Errorf("LOG_DB_DSN is required when using db log mode")
		}
	}
	return nil
}

func validateLogMode(name, mode string) error {
	switch mode {
	case LogModeOff, LogModeFile, LogModeDB:
		return nil
	default:
		return fmt.Errorf("%s must be off|file|db (got %q)", name, mode)
	}
}

// NeedsDB is true if any sink uses the database.
func (c LogConfig) NeedsDB() bool {
	return c.SystemMode == LogModeDB || c.AccessMode == LogModeDB
}
