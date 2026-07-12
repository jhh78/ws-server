package config

import (
	"fmt"
	"strings"
)

// 로그 모드 상수 (SYSTEM_LOG_MODE / ACCESS_LOG_MODE).
const (
	// LogModeOff 는 해당 싱크를 끕니다.
	LogModeOff = "off"
	// LogModeFile 은 텍스트 파일에 기록합니다.
	LogModeFile = "file"
	// LogModeDB 는 LOG_DB_DRIVER / LOG_DB_DSN 의 DB 에 기록합니다.
	LogModeDB = "db"
)

// LOG_DB_DRIVER 정규화 후 지원 값.
const (
	// LogDBSQLite 는 modernc.org/sqlite 파일 DB.
	LogDBSQLite = "sqlite"
	// LogDBMySQL 은 MySQL / MariaDB.
	LogDBMySQL = "mysql"
	// LogDBPostgres 는 PostgreSQL.
	LogDBPostgres = "postgres"
)

// LogConfig 는 시스템·액세스 로그 각각 및 공통 DB 연결을 제어합니다.
//
// 시스템과 액세스는 서로 다른 모드(file/db/off)를 가질 수 있습니다.
// sample.env 에 SYSTEM_* / ACCESS_* 를 각각 등록합니다.
type LogConfig struct {
	// SystemMode 는 시스템 로그 모드: off | file | db.
	SystemMode string
	// AccessMode 는 액세스 로그 모드: off | file | db.
	AccessMode string

	// SystemFile 은 SystemMode=file 일 때 경로.
	SystemFile string
	// AccessFile 은 AccessMode=file 일 때 경로.
	AccessFile string

	// DBDriver 는 sqlite | mysql | postgres (별칭: postgresql, pg, sqlite3, mariadb).
	DBDriver string
	// DBDSN 은 드라이버별 연결 문자열.
	//
	// 예:
	//   sqlite:    logs/ws-server.db
	//   mysql:     user:pass@tcp(127.0.0.1:3306)/ws_logs?parseTime=true
	//   postgres:  postgres://user:pass@127.0.0.1:5432/ws_logs?sslmode=disable
	DBDSN string
}

// DefaultLog 는 sample.env 기본과 동일한 로그 설정을 반환합니다.
//
// Returns:
//   - LogConfig: file 모드 + sqlite DSN 기본값
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

// LoadLog 는 OS 환경변수에서 LogConfig 를 읽고 Validate 합니다.
//
// Returns:
//   - LogConfig: 로드된 설정
//   - error: 모드/파일/드라이버/DSN 검증 실패
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

// NormalizeLogDBDriver 는 드라이버 별칭을 정규 이름으로 매핑합니다.
//
// Parameters:
//   - raw: 사용자 입력 드라이버 문자열
//
// Returns:
//   - string: sqlite | mysql | postgres 또는 lower-trim 원본
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

// Validate 는 로그 모드·파일 경로·DB 드라이버/DSN 을 검사합니다.
//
// Returns:
//   - error: 위반 시 설명 메시지
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

// validateLogMode 는 off|file|db 인지 확인합니다.
//
// Parameters:
//   - name: 오류 메시지용 변수명
//   - mode: 검사할 모드 값
func validateLogMode(name, mode string) error {
	switch mode {
	case LogModeOff, LogModeFile, LogModeDB:
		return nil
	default:
		return fmt.Errorf("%s must be off|file|db (got %q)", name, mode)
	}
}

// NeedsDB 는 시스템 또는 액세스가 db 모드이면 true 입니다.
//
// Returns:
//   - bool: DB 연결이 필요하면 true
func (c LogConfig) NeedsDB() bool {
	return c.SystemMode == LogModeDB || c.AccessMode == LogModeDB
}
