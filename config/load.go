package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type loadState struct {
	fromFile  map[string]string
	skippedOS map[string]string
}

// LoadFile reads KEY=VALUE lines into the process environment without
// overwriting variables already set in the OS environment.
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

func envString(key, def string) string {
	if v, ok := lookupNonEmpty(key); ok {
		return v
	}
	return def
}

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

func unquote(val string) string {
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1]
		}
	}
	return val
}

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
