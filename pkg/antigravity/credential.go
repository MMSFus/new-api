package antigravity

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const MaxImportBytes = 8 << 20
const MaxImportAccounts = 500

// Credential is the canonical channel key. Imports deliberately exclude remote
// URLs and proxy configuration: a backup must not choose where tokens are sent.
type Credential struct {
	Type         string `json:"type"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Email        string `json:"email,omitempty"`
	Name         string `json:"name,omitempty"`
}

func (c Credential) Key() string { b, _ := common.Marshal(c); return string(b) }
func (c Credential) Identity() [32]byte {
	token := c.RefreshToken
	if token == "" {
		token = c.AccessToken
	}
	return sha256.Sum256([]byte(token))
}
func (c Credential) NeedsRefresh() bool {
	return c.AccessToken == "" || (c.RefreshToken != "" && c.ExpiresAt == 0) || (c.ExpiresAt > 0 && c.ExpiresAt <= time.Now().Add(time.Minute).Unix())
}

// Import accepts CPA auth files, sub2api exports/API envelopes, account arrays,
// and the compact JSON-lines representation used by multi-key channels.
func Import(raw string) ([]Credential, error) {
	if len(raw) > MaxImportBytes {
		return nil, errors.New("Antigravity import exceeds size limit")
	}
	raw = strings.TrimSpace(raw)
	var value any
	if err := common.Unmarshal([]byte(raw), &value); err != nil {
		var values []any
		for line := range strings.SplitSeq(raw, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var v any
			if common.Unmarshal([]byte(line), &v) != nil {
				return nil, errors.New("Invalid Antigravity credential JSON")
			}
			values = append(values, v)
			if len(values) > MaxImportAccounts {
				return nil, errors.New("Too many Antigravity accounts")
			}
		}
		if len(values) == 0 {
			return nil, errors.New("Invalid Antigravity credential JSON")
		}
		value = values
	}
	var entries []map[string]any
	if err := collectAccounts(value, 0, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("No Antigravity accounts found")
	}
	out := make([]Credential, 0, len(entries))
	seen := map[[32]byte]bool{}
	for i, v := range entries {
		c, err := importAccount(v)
		if err != nil {
			return nil, fmt.Errorf("Antigravity account %d: %w", i+1, err)
		}
		id := c.Identity()
		if !seen[id] {
			out = append(out, c)
			seen[id] = true
		}
	}
	return out, nil
}

func collectAccounts(v any, depth int, out *[]map[string]any) error {
	if depth > 5 {
		return errors.New("Invalid Antigravity export structure")
	}
	switch v := v.(type) {
	case []any:
		for _, item := range v {
			if err := collectAccounts(item, depth+1, out); err != nil {
				return err
			}
		}
	case map[string]any:
		if accounts, ok := v["accounts"]; ok {
			return collectAccounts(accounts, depth+1, out)
		}
		if data, ok := v["data"]; ok {
			return collectAccounts(data, depth+1, out)
		}
		// A mixed sub2api backup may contain other platforms. Only explicit
		// Antigravity accounts are imported; malformed matching accounts fail atomically.
		if platform, ok := v["platform"].(string); ok && platform != "antigravity" {
			return nil
		}
		if metadata, ok := v["metadata"].(map[string]any); ok {
			v = metadata
		}
		*out = append(*out, v)
		if len(*out) > MaxImportAccounts {
			return errors.New("Too many Antigravity accounts")
		}
	default:
		return errors.New("Invalid Antigravity export structure")
	}
	return nil
}

func importAccount(v map[string]any) (Credential, error) {
	c := Credential{Type: "antigravity"}
	if platform, ok := v["platform"].(string); ok {
		if platform != "antigravity" || text(v, "type") != "oauth" {
			return c, errors.New("Expected an Antigravity OAuth account")
		}
	} else if kind := text(v, "type"); kind != "" && kind != "antigravity" {
		return c, errors.New("Expected an Antigravity credential")
	}
	c.Name = text(v, "name")
	if creds, ok := v["credentials"].(map[string]any); ok {
		v = creds
	}
	c.AccessToken = text(v, "access_token")
	c.RefreshToken = text(v, "refresh_token")
	c.ProjectID = text(v, "project_id")
	c.Email = text(v, "email")
	if c.AccessToken == "" && c.RefreshToken == "" {
		return c, errors.New("An access token or refresh token is required")
	}
	for _, token := range []string{c.AccessToken, c.RefreshToken} {
		if len(token) > 32768 || strings.ContainsFunc(token, func(r rune) bool { return r <= 32 || r == 127 }) {
			return c, errors.New("Invalid token")
		}
	}
	for _, field := range []string{"expires_at", "expired"} {
		if val, ok := v[field]; ok && val != nil && val != "" {
			var err error
			c.ExpiresAt, err = parseExpiry(val)
			if err != nil {
				return c, errors.New("Invalid credential expiry")
			}
			break
		}
	}
	if c.ExpiresAt > 0 && c.ExpiresAt <= time.Now().Unix() && c.RefreshToken == "" {
		return c, errors.New("Expired credential requires a refresh token")
	}
	return c, nil
}
func text(v map[string]any, k string) string { s, _ := v[k].(string); return strings.TrimSpace(s) }
func parseExpiry(v any) (int64, error) {
	b, err := common.Marshal(v)
	if err != nil {
		return 0, err
	}
	var s string
	if common.Unmarshal(b, &s) == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Unix(), nil
		}
	} else {
		s = string(b)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 253402300799 {
		return 0, errors.New("invalid expiry")
	}
	return n, nil
}
