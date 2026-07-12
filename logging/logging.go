// Package logging 은 시스템·액세스 로그를 파일 및/또는 SQL DB(sqlite|mysql|postgres)에 기록합니다.
//
// 모드(off|file|db)는 config.LogConfig 의 SYSTEM_LOG_* / ACCESS_LOG_* 로 각각 지정합니다.
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

// Level 은 시스템 로그 심각도입니다.
type Level string

const (
	// LevelInfo 는 일반 정보.
	LevelInfo Level = "INFO"
	// LevelWarn 는 경고.
	LevelWarn Level = "WARN"
	// LevelError 는 오류.
	LevelError Level = "ERROR"
	// LevelDebug 는 디버그 (예약).
	LevelDebug Level = "DEBUG"
)

// Logger 는 애플리케이션 로깅 퍼사드입니다.
//
// 동시 호출은 내부 뮤텍스로 직렬화됩니다. Close 이후 쓰기는 무시됩니다.
type Logger struct {
	cfg LogConfigView

	mu     sync.Mutex
	sysF   *os.File
	accF   *os.File
	db     *dbStore
	closed bool
}

// LogConfigView 는 로거가 사용하는 설정 부분 집합입니다 (테스트 결합 완화).
type LogConfigView struct {
	// SystemMode 는 off | file | db.
	SystemMode string
	// AccessMode 는 off | file | db.
	AccessMode string
	// SystemFile 은 file 모드 시스템 로그 경로.
	SystemFile string
	// AccessFile 은 file 모드 액세스 로그 경로.
	AccessFile string
	// DBDriver 는 sqlite | mysql | postgres.
	DBDriver string
	// DBDSN 은 드라이버별 DSN.
	DBDSN string
}

// FromConfig 는 config.LogConfig 를 LogConfigView 로 변환합니다.
//
// Parameters:
//   - c: 로드된 로그 설정
//
// Returns:
//   - LogConfigView: 로거용 뷰
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

// New 는 cfg 에 따라 파일·DB 싱크를 열고 Logger 를 반환합니다.
//
// Parameters:
//   - cfg: 시스템/액세스 모드 및 경로·DSN
//
// Returns:
//   - *Logger: 사용 가능한 로거
//   - error: 파일/DB 오픈·마이그레이션 실패
func New(cfg config.LogConfig) (*Logger, error) {
	return newLogger(FromConfig(cfg))
}

// newLogger 는 LogConfigView 로 싱크를 구성합니다.
//
// Parameters:
//   - cfg: 뷰 설정
//
// Returns:
//   - *Logger: 구성된 로거
//   - error: 오픈 실패 (부분 오픈 시 정리 후 반환)
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

// openAppend 는 경로에 append 모드 파일을 엽니다. 부모 디렉터리를 만듭니다.
//
// Parameters:
//   - path: 로그 파일 경로
//
// Returns:
//   - *os.File: 쓰기용 파일
//   - error: MkdirAll 또는 OpenFile 실패
func openAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// path may be bare filename in cwd
		if filepath.Dir(path) != "." {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// Close 는 파일·DB 싱크를 플러시·닫습니다. 멱등입니다.
//
// Returns:
//   - error: 첫 번째 닫기 오류 (있을 경우)
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

// System 은 시스템 로그 한 줄을 기록합니다 (SYSTEM_LOG_MODE 에 따라 file 및/또는 db).
//
// Parameters:
//   - level: INFO / WARN / ERROR / DEBUG
//   - component: 모듈 태그 (예: server, client)
//   - message: 본문
//   - detail: 선택적 부가 문자열 (공백으로 이어 붙임)
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

// Info 는 System(LevelInfo, ...) 단축 호출입니다.
//
// Parameters:
//   - component: 모듈 태그
//   - message: 본문
//   - detail: 부가 정보
func (l *Logger) Info(component, message string, detail ...string) {
	l.System(LevelInfo, component, message, detail...)
}

// Warn 는 System(LevelWarn, ...) 단축 호출입니다.
//
// Parameters:
//   - component: 모듈 태그
//   - message: 본문
//   - detail: 부가 정보
func (l *Logger) Warn(component, message string, detail ...string) {
	l.System(LevelWarn, component, message, detail...)
}

// Error 는 System(LevelError, ...) 단축 호출입니다.
//
// Parameters:
//   - component: 모듈 태그
//   - message: 본문
//   - detail: 부가 정보
func (l *Logger) Error(component, message string, detail ...string) {
	l.System(LevelError, component, message, detail...)
}

// AccessEntry 는 액세스 로그 한 건(연결·프로토콜 액션)입니다.
type AccessEntry struct {
	// ClientID 는 서버 발급 연결 ID.
	ClientID string
	// RemoteAddr 는 피어 주소.
	RemoteAddr string
	// Action 은 connect, disconnect, join, leave, send, whisper, ping, error, upgrade_fail 등.
	Action string
	// Type 은 Envelope.type.
	Type string
	// Scope 는 area | channel.
	Scope string
	// Target 은 에리어/채널 ID.
	Target string
	// ChannelKind 는 party | guild | whisper | custom.
	ChannelKind string
	// Detail 은 부가 설명 (오류 메시지, whisper peer 등).
	Detail string
}

// Access 는 액세스 로그를 기록합니다 (ACCESS_LOG_MODE 에 따라 file 및/또는 db).
//
// Parameters:
//   - e: 액세스 엔트리
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

// emptyDash 는 빈 문자열을 "-" 로 바꿉니다 (파일 로그 가독성).
//
// Parameters:
//   - s: 원본
//
// Returns:
//   - string: 비어 있으면 "-", 아니면 s
func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Nop 은 모든 출력을 버리는 로거를 반환합니다 (테스트용).
//
// Returns:
//   - *Logger: SystemMode/AccessMode 가 off 인 로거
func Nop() *Logger {
	return &Logger{cfg: LogConfigView{
		SystemMode: config.LogModeOff,
		AccessMode: config.LogModeOff,
	}}
}
