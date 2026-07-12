package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadState 는 LoadFile 결과입니다.
//
// 필드:
//   - fromFile: 파일에서 OS 에 새로 설정한 키→값
//   - skippedOS: OS 에 이미 있어 파일 값을 적용하지 않은 키→파일 값
type loadState struct {
	fromFile  map[string]string
	skippedOS map[string]string
}

// LoadFile 은 KEY=VALUE 줄을 읽어 프로세스 환경에 넣습니다.
//
// 이미 OS 에 설정된 변수는 덮어쓰지 않습니다.
//
// Parameters:
//   - path: env 파일 절대/상대 경로
//
// Returns:
//   - loadState: 파일 적용·스킵 맵
//   - error: open/scan/형식/setenv 오류
func LoadFile(path string) (loadState, error) {
	st := loadState{
		fromFile:  make(map[string]string),
		skippedOS: make(map[string]string),
	}

	f, err := os.Open(path)
	if err != nil {
		return st, fmt.Errorf("open env file %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return st, fmt.Errorf("%s:%d: invalid line (expected KEY=VALUE)", path, lineNo)
		}
		key = strings.TrimSpace(key)
		val = unquote(strings.TrimSpace(val))
		if key == "" {
			return st, fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		if !isValidEnvKey(key) {
			return st, fmt.Errorf("%s:%d: invalid key %q", path, lineNo, key)
		}
		if _, exists := os.LookupEnv(key); exists {
			st.skippedOS[key] = val
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return st, fmt.Errorf("setenv %s: %w", key, err)
		}
		st.fromFile[key] = val
	}
	if err := sc.Err(); err != nil {
		return st, fmt.Errorf("read env file %s: %w", path, err)
	}
	return st, nil
}

// lookupNonEmpty 는 OS 환경변수 중 공백이 아닌 값을 조회합니다.
//
// Parameters:
//   - key: 환경 변수 이름
//
// Returns:
//   - string: trim 된 값
//   - bool: 존재하고 비어 있지 않으면 true
func lookupNonEmpty(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

// envString 은 OS env 또는 기본 문자열을 반환합니다.
//
// Parameters:
//   - key: 환경 변수 이름
//   - def: 부재·공백 시 기본값
func envString(key, def string) string {
	if v, ok := lookupNonEmpty(key); ok {
		return v
	}
	return def
}

// envIntStrict 는 OS env 를 정수로 파싱합니다. 키가 없으면 def, 잘못된 형식이면 error.
//
// Parameters:
//   - key: 환경 변수 이름
//   - def: 미설정 시 기본값
//
// Returns:
//   - int: 파싱 결과 또는 def
//   - error: 키가 있으나 정수가 아닐 때
func envIntStrict(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def, fmt.Errorf("%s=%q is not a valid integer", key, v)
	}
	return n, nil
}

// unquote 는 양쪽 큰따옴표 또는 작은따옴표를 제거합니다.
//
// Parameters:
//   - val: 원본 값
//
// Returns:
//   - string: 따옴표 제거 결과 (조건 불일치 시 원본)
func unquote(val string) string {
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1]
		}
	}
	return val
}

// isValidEnvKey 는 환경 변수 키 형식을 검사합니다 ([A-Za-z_][A-Za-z0-9_]*).
//
// Parameters:
//   - key: 검사할 키
//
// Returns:
//   - bool: 유효하면 true
func isValidEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
			continue
		case i > 0 && r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}
