package k01fixture

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strings"
	"sync"

	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

const FixtureFileEnv = "TMP_20261106_K01_FIXTURE_FILE"

type Reply struct {
	Status   int
	Body     []byte
	IsStream bool
}

type cacheEntry struct {
	path    string
	size    int64
	modTime int64
	replies map[string]Reply
	err     error
}

var fixtureCache struct {
	sync.Mutex
	entry  cacheEntry
	logged map[string]struct{}
}

func ValidateFile(path string) error {
	_, err := loadFile(path)
	return err
}

// Lookup loads and matches a request. shouldLog is true once per failed file version.
func Lookup(path string, request []byte) (Reply, bool, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Reply{}, false, markLogOnce("fixture-path-unset"), fmt.Errorf("%s is not set", FixtureFileEnv)
	}
	key, err := CanonicalJSON(request)
	if err != nil {
		return Reply{}, false, true, fmt.Errorf("canonicalize request: %w", err)
	}
	entry, fingerprint, err := loadFileVersion(path)
	if err != nil {
		return Reply{}, false, markLogOnce(fingerprint), err
	}
	reply, ok := entry[key]
	return reply, ok, false, nil
}

func loadFile(path string) (map[string]Reply, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%s is not set", FixtureFileEnv)
	}
	replies, _, err := loadFileVersion(path)
	return replies, err
}

func loadFileVersion(path string) (map[string]Reply, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, path + ":stat", fmt.Errorf("stat fixture file: %w", err)
	}
	fingerprint := fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
	fixtureCache.Lock()
	defer fixtureCache.Unlock()
	if fixtureCache.entry.path == path && fixtureCache.entry.size == info.Size() && fixtureCache.entry.modTime == info.ModTime().UnixNano() {
		return fixtureCache.entry.replies, fingerprint, fixtureCache.entry.err
	}
	data, err := os.ReadFile(path)
	var replies map[string]Reply
	if err == nil {
		replies, err = Parse(data)
	}
	if err != nil {
		err = fmt.Errorf("load fixture file: %w", err)
	}
	fixtureCache.entry = cacheEntry{path: path, size: info.Size(), modTime: info.ModTime().UnixNano(), replies: replies, err: err}
	return replies, fingerprint, err
}

func markLogOnce(fingerprint string) bool {
	fixtureCache.Lock()
	defer fixtureCache.Unlock()
	if fixtureCache.logged == nil {
		fixtureCache.logged = make(map[string]struct{})
	}
	if _, exists := fixtureCache.logged[fingerprint]; exists {
		return false
	}
	fixtureCache.logged[fingerprint] = struct{}{}
	return true
}

func Parse(data []byte) (map[string]Reply, error) {
	replies := make(map[string]Reply)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record struct {
			Status   *int            `json:"status"`
			Request  json.RawMessage `json:"request"`
			Response json.RawMessage `json:"response"`
		}
		if err := common.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("invalid K01 fixture JSONL at line %d: %w", lineNumber+1, err)
		}
		status := 200
		if record.Status != nil {
			status = *record.Status
		}
		if status < 100 || status > 599 {
			return nil, fmt.Errorf("invalid K01 fixture status at line %d: %d is outside 100-599", lineNumber+1, status)
		}
		if len(record.Request) == 0 || len(record.Response) == 0 {
			return nil, fmt.Errorf("invalid K01 fixture at line %d: request and response are required", lineNumber+1)
		}
		requestKey, err := CanonicalJSON(record.Request)
		if err != nil {
			return nil, fmt.Errorf("invalid K01 fixture request at line %d: %w", lineNumber+1, err)
		}
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := common.Unmarshal(record.Request, &request); err != nil {
			return nil, fmt.Errorf("invalid K01 fixture request at line %d: %w", lineNumber+1, err)
		}
		body := bytes.TrimSpace(record.Response)
		if common.GetJsonType(record.Response) == "string" {
			var response string
			if err := common.Unmarshal(record.Response, &response); err != nil {
				return nil, fmt.Errorf("invalid K01 fixture response at line %d: %w", lineNumber+1, err)
			}
			body = []byte(response)
		}
		if _, exists := replies[requestKey]; !exists {
			replies[requestKey] = Reply{Status: status, Body: bytes.Clone(body), IsStream: request.Stream}
		}
	}
	return replies, nil
}

func CanonicalJSON(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty JSON")
	}
	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := common.Unmarshal(trimmed, &object); err != nil {
			return "", err
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		var builder strings.Builder
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteByte(',')
			}
			encodedKey, err := common.Marshal(key)
			if err != nil {
				return "", err
			}
			value, err := CanonicalJSON(object[key])
			if err != nil {
				return "", err
			}
			builder.Write(encodedKey)
			builder.WriteByte(':')
			builder.WriteString(value)
		}
		builder.WriteByte('}')
		return builder.String(), nil
	case '[':
		var array []json.RawMessage
		if err := common.Unmarshal(trimmed, &array); err != nil {
			return "", err
		}
		var builder strings.Builder
		builder.WriteByte('[')
		for index, value := range array {
			if index > 0 {
				builder.WriteByte(',')
			}
			canonical, err := CanonicalJSON(value)
			if err != nil {
				return "", err
			}
			builder.WriteString(canonical)
		}
		builder.WriteByte(']')
		return builder.String(), nil
	case '"':
		var value string
		if err := common.Unmarshal(trimmed, &value); err != nil {
			return "", err
		}
		encoded, err := common.Marshal(value)
		return string(encoded), err
	default:
		var value json.RawMessage
		if err := common.Unmarshal(trimmed, &value); err != nil {
			return "", err
		}
		if common.GetJsonType(value) == "number" {
			rat, ok := new(big.Rat).SetString(string(trimmed))
			if !ok {
				return "", fmt.Errorf("invalid JSON number %q", trimmed)
			}
			return rat.RatString(), nil
		}
		return string(trimmed), nil
	}
}
