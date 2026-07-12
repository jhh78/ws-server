// Package logging writes system and access logs to text files and/or SQLite.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jhh78/ws-server/config"
)

// Level for system logs.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
	LevelDebug Level = "DEBUG"
)

// Logger is the application logging facade.
type Logger struct {
	cfg LogConfigView

	mu     sync.Mutex
	sysF   *os.File
	accF   *os.File
	db     *dbStore
	closed bool
}

// LogConfigView is the subset of config used by the logger (avoids import cycles in tests).
type LogConfigView struct {
	SystemMode string
	AccessMode string
	SystemFile string
	AccessFile string
	DBDriver   string
	DBDSN      string
}

// FromConfig maps config.LogConfig.
func FromConfig(c config.LogConfig) LogConfigView {
	return LogConfigView{
		SystemMode: c.SystemMode,
		AccessMode: c.AccessMode,
		SystemFile: c.SystemFile,
		AccessFile: c.AccessFile,
		DBDriver:   c.DBDriver,
		DBDSN:      c.DBDSN,
	}
}

// New opens file and/or DB sinks according to cfg.
func New(cfg config.LogConfig) (*Logger, error) {
	return newLogger(FromConfig(cfg))
}

func newLogger(cfg LogConfigView) (*Logger, error) {
	l := &Logger{cfg: cfg}

	needDB := cfg.SystemMode == config.LogModeDB || cfg.AccessMode == config.LogModeDB
	if needDB {
		store, err := openDB(cfg.DBDriver, cfg.DBDSN)
		if err != nil {
			return nil, fmt.Errorf("log db: %w", err)
		}
		l.db = store
	}

	if cfg.SystemMode == config.LogModeFile {
		f, err := openAppend(cfg.SystemFile)
		if err != nil {
			_ = l.Close()
			return nil, fmt.Errorf("system log file: %w", err)
		}
		l.sysF = f
	}
	if cfg.AccessMode == config.LogModeFile {
		f, err := openAppend(cfg.AccessFile)
		if err != nil {
			_ = l.Close()
			return nil, fmt.Errorf("access log file: %w", err)
		}
		l.accF = f
	}

	return l, nil
}

func openAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// path may be bare filename in cwd
		if filepath.Dir(path) != "." {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// Close flushes and closes sinks.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	var first error
	if l.sysF != nil {
		if err := l.sysF.Close(); err != nil && first == nil {
			first = err
		}
		l.sysF = nil
	}
	if l.accF != nil {
		if err := l.accF.Close(); err != nil && first == nil {
			first = err
		}
		l.accF = nil
	}
	if l.db != nil {
		if err := l.db.Close(); err != nil && first == nil {
			first = err
		}
		l.db = nil
	}
	return first
}

// System writes a system log line (file and/or db per SYSTEM_LOG_MODE).
func (l *Logger) System(level Level, component, message string, detail ...string) {
	if l == nil {
		return
	}
	det := ""
	if len(detail) > 0 {
		det = strings.Join(detail, " ")
	}
	now := time.Now().UTC()
	line := fmt.Sprintf("%s [%s] [%s] %s", now.Format(time.RFC3339), level, component, message)
	if det != "" {
		line += " | " + det
	}
	line += "\n"

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}

	switch l.cfg.SystemMode {
	case config.LogModeFile:
		if l.sysF != nil {
			_, _ = l.sysF.WriteString(line)
		}
	case config.LogModeDB:
		if l.db != nil {
			_ = l.db.insertSystem(now, string(level), component, message, det)
		}
	}
}

// Info is System(INFO, ...).
func (l *Logger) Info(component, message string, detail ...string) {
	l.System(LevelInfo, component, message, detail...)
}

// Warn is System(WARN, ...).
func (l *Logger) Warn(component, message string, detail ...string) {
	l.System(LevelWarn, component, message, detail...)
}

// Error is System(ERROR, ...).
func (l *Logger) Error(component, message string, detail ...string) {
	l.System(LevelError, component, message, detail...)
}

// AccessEntry is one access-log record (connection / protocol action).
type AccessEntry struct {
	ClientID    string
	RemoteAddr  string
	Action      string // connect, disconnect, join, leave, send, whisper, ping, error, upgrade_fail
	Type        string
	Scope       string
	Target      string
	ChannelKind string
	Detail      string
}

// Access writes an access log (file and/or db per ACCESS_LOG_MODE).
func (l *Logger) Access(e AccessEntry) {
	if l == nil {
		return
	}
	now := time.Now().UTC()
	// Combined-ish text line for file mode
	line := fmt.Sprintf("%s client=%s remote=%s action=%s type=%s scope=%s target=%s channel_kind=%s detail=%s\n",
		now.Format(time.RFC3339),
		emptyDash(e.ClientID),
		emptyDash(e.RemoteAddr),
		emptyDash(e.Action),
		emptyDash(e.Type),
		emptyDash(e.Scope),
		emptyDash(e.Target),
		emptyDash(e.ChannelKind),
		emptyDash(e.Detail),
	)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}

	switch l.cfg.AccessMode {
	case config.LogModeFile:
		if l.accF != nil {
			_, _ = l.accF.WriteString(line)
		}
	case config.LogModeDB:
		if l.db != nil {
			_ = l.db.insertAccess(now, e)
		}
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Nop returns a logger that discards everything (tests).
func Nop() *Logger {
	return &Logger{cfg: LogConfigView{
		SystemMode: config.LogModeOff,
		AccessMode: config.LogModeOff,
	}}
}
