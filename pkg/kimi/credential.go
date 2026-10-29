// Package kimi implements the Kimi Code credential and regional API contract.
package kimi

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	Mainland       = "mainland-cn"
	Global         = "global"
	ClientID       = "17e5f671-d194-4dfb-9706-5516cb48c098"
	MaxImportBytes = 8 << 20
	MaxAccounts    = 500
)

type Credential struct {
	SchemaVersion int    `json:"schema_version"`
	Provider      string `json:"provider"`
	ID            string `json:"credential_id"`
	Region        string `json:"region"`
	AuthType      string `json:"auth_type"`
	AccountMode   string `json:"account_mode"`
	DeviceID      string `json:"device_id,omitempty"`
	AccessToken   string `json:"access_token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	ExpiresAt     int64  `json:"expires_at,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Name          string `json:"name,omitempty"`
}

func (c Credential) Key() string { b, _ := common.Marshal(c); return string(b) }
func (c Credential) Token() string {
	if c.AuthType == "api_key" {
		return c.APIKey
	}
	return c.AccessToken
}
func (c Credential) NeedsRefresh() bool {
	return c.AuthType == "oauth" && (c.AccessToken == "" || c.ExpiresAt > 0 && c.ExpiresAt <= time.Now().Add(time.Minute).Unix())
}
func (c Credential) AuthHost() string {
	if c.Region == Global {
		return "https://auth.kimi.ai"
	}
	return "https://auth.kimi.com"
}
func (c Credential) BaseURL() string {
	if c.Region == Global {
		return "https://api.kimi.ai/coding/v1"
	}
	return "https://api.kimi.com/coding/v1"
}

func NormalizeRegion(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cn", Mainland:
		return Mainland, nil
	case Global:
		return Global, nil
	}
	return "", errors.New("Select a valid Kimi region")
}

// OfficialBase accepts only regional Coding endpoints, never arbitrary import targets.
func OfficialBase(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Port() != "" && u.Port() != "443" {
		return "", "", errors.New("Invalid Kimi Coding API address")
	}
	region := Mainland
	switch strings.ToLower(u.Hostname()) {
	case "api.kimi.com":
	case "api.kimi.ai":
		region = Global
	default:
		return "", "", errors.New("Invalid Kimi Coding API address")
	}
	switch strings.TrimRight(u.Path, "/") {
	case "/coding", "/coding/v1":
	default:
		return "", "", errors.New("Invalid Kimi Coding API path")
	}
	c := Credential{Region: region}
	return c.BaseURL(), region, nil
}

func Parse(raw string) (Credential, error) {
	var c Credential
	if common.UnmarshalJsonStr(raw, &c) != nil || c.SchemaVersion != 1 || c.Provider != "kimi" {
		return c, errors.New("Legacy Kimi web credentials require Kimi Code authorization or JSON import")
	}
	return validate(c)
}

func validate(c Credential) (Credential, error) {
	var err error
	c.Region, err = NormalizeRegion(c.Region)
	if err != nil {
		return c, err
	}
	if c.AccountMode != "coding" {
		return c, errors.New("Kimi PAYG credentials belong in a Moonshot channel")
	}
	if c.AuthType != "oauth" && c.AuthType != "api_key" {
		return c, errors.New("Invalid Kimi authentication type")
	}
	for _, s := range []string{c.AccessToken, c.RefreshToken, c.APIKey, c.DeviceID, c.ID, c.Name, c.Scope, c.TokenType} {
		if len(s) > 65536 || strings.ContainsAny(s, "\r\n\x00") {
			return c, errors.New("Invalid Kimi credential field")
		}
	}
	if c.ExpiresAt < 0 || c.ExpiresAt > 253402300799 {
		return c, errors.New("Invalid Kimi token expiry")
	}
	if c.AuthType == "api_key" {
		if c.APIKey == "" || c.RefreshToken != "" || c.AccessToken != "" {
			return c, errors.New("Invalid Kimi API key credential")
		}
	} else {
		if c.APIKey != "" || c.AccessToken == "" && c.RefreshToken == "" {
			return c, errors.New("Kimi OAuth token is missing")
		}
		if c.RefreshToken == "" && c.ExpiresAt > 0 && c.ExpiresAt <= time.Now().Unix() {
			return c, errors.New("Kimi access token expired; authorize again")
		}
	}
	if c.TokenType != "" && !strings.EqualFold(c.TokenType, "Bearer") {
		return c, errors.New("Unsupported Kimi token type")
	}
	if c.ID == "" {
		secret := c.RefreshToken
		if secret == "" {
			secret = c.Token()
		}
		sum := sha256.Sum256([]byte(c.Region + "\x00" + c.AuthType + "\x00" + secret))
		c.ID = fmt.Sprintf("%x", sum[:])
	}
	if len(c.ID) != 64 {
		return c, errors.New("Invalid Kimi credential identity")
	}
	for _, r := range c.ID {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return c, errors.New("Invalid Kimi credential identity")
		}
	}
	if c.DeviceID == "" {
		c.DeviceID = c.ID
	}
	c.SchemaVersion = 1
	c.Provider = "kimi"
	return c, nil
}

// Import accepts CPA files, official token files and sub2api export envelopes.
// It is atomic: invalid Kimi entries fail with an index, other providers are skipped.
func Import(raw, defaultRegion string) ([]Credential, error) {
	return parseImport(raw, defaultRegion, nil)
}

type ImportIssue struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// Preview validates each account without silently accepting an invalid entry.
// Only explicit selection in the dashboard applies the valid credentials.
func Preview(raw, defaultRegion string) ([]Credential, []ImportIssue, error) {
	issues := make([]ImportIssue, 0)
	credentials, err := parseImport(raw, defaultRegion, &issues)
	return credentials, issues, err
}

func parseImport(raw, defaultRegion string, issues *[]ImportIssue) ([]Credential, error) {
	if len(raw) > MaxImportBytes {
		return nil, errors.New("Kimi import exceeds size limit")
	}
	var result []Credential
	count := 0
	var walk func(any, int, string) error
	var visit func(any, int, string) error
	limitError := errors.New("Kimi import exceeds account or nesting limit")
	walk = func(v any, depth int, name string) error {
		if depth > 5 {
			return limitError
		}
		if object, ok := v.(map[string]any); ok && name == "" {
			name, _ = object["name"].(string)
		}
		err := visit(v, depth, name)
		if err != nil && issues != nil && !errors.Is(err, limitError) {
			// Error messages contain field names only, never imported values.
			*issues = append(*issues, ImportIssue{ID: len(*issues) + 1, Name: name, Error: err.Error()})
			if len(*issues) > MaxAccounts {
				return limitError
			}
			return nil
		}
		return err
	}
	visit = func(v any, depth int, name string) error {
		if depth > 5 {
			return errors.New("Kimi import nesting exceeds limit")
		}
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				if err := walk(item, depth+1, ""); err != nil {
					return err
				}
			}
			return nil
		case map[string]any:
			str := func(k string) string { s, _ := x[k].(string); return s }
			platform := str("platform")
			typ := strings.ToLower(str("type"))
			provider := str("provider")
			if platform != "" && platform != "kimi" || provider != "" && provider != "kimi" {
				return nil
			}
			if typ != "" && typ != "kimi" && typ != "kimi-ai" && typ != "kimi.ai" && typ != "oauth" && typ != "apikey" && typ != "api_key" && typ != "sub2api-data" && typ != "sub2api-bundle" {
				return nil
			}
			for _, field := range []string{"platform", "type", "provider", "region", "domain", "account_mode", "auth_type", "access_token", "refresh_token", "api_key", "device_id", "credential_id", "name", "scope", "token_type", "base_url", "baseUrl", "expired"} {
				if value, ok := x[field]; ok {
					if _, valid := value.(string); !valid {
						return errors.New("Invalid Kimi credential field type")
					}
				}
			}
			for _, key := range []string{"data", "accounts"} {
				if nested, ok := x[key]; ok {
					return walk(nested, depth+1, "")
				}
			}
			if nested, ok := x["credentials"]; ok {
				child, valid := nested.(map[string]any)
				if !valid {
					return errors.New("Invalid Kimi credentials object")
				}
				for _, field := range []string{"region", "domain", "base_url", "account_mode", "auth_type"} {
					if parent, exists := x[field]; exists {
						if own, exists := child[field]; exists && own != parent {
							return errors.New("Conflicting Kimi credential fields")
						}
						child[field] = parent
					}
				}
				return walk(child, depth+1, str("name"))
			}
			count++
			if count > MaxAccounts {
				return limitError
			}
			if name == "" {
				name = str("name")
			}
			c := Credential{SchemaVersion: 1, Provider: "kimi", Region: defaultRegion, AccountMode: str("account_mode"), AuthType: str("auth_type"), AccessToken: str("access_token"), RefreshToken: str("refresh_token"), APIKey: str("api_key"), DeviceID: str("device_id"), Name: name, Scope: str("scope"), TokenType: str("token_type")}
			if str("schema_version") != "" {
				return errors.New("Invalid Kimi schema version")
			}
			if version, ok := x["schema_version"]; ok {
				if version != float64(1) {
					return errors.New("Unsupported Kimi schema version")
				}
				// Imported identities are derived from the verified regional secret.
				// Never trust an arbitrary external credential_id to alias a stored account.
			}
			if c.AccountMode == "" {
				c.AccountMode = "coding"
			}
			var evidence string
			merge := func(r string) error {
				normalized, err := NormalizeRegion(r)
				if err != nil {
					return err
				}
				if evidence != "" && evidence != normalized {
					return errors.New("Conflicting Kimi regions")
				}
				evidence = normalized
				return nil
			}
			if r := str("region"); r != "" {
				if err := merge(r); err != nil {
					return err
				}
			}
			if typ == "kimi-ai" || typ == "kimi.ai" {
				if err := merge(Global); err != nil {
					return err
				}
			}
			// CPA's explicit type denotes its regional provider; plain official files have no type.
			if typ == "kimi" {
				if err := merge(Mainland); err != nil {
					return err
				}
			}
			if domain := str("domain"); domain != "" {
				r := Mainland
				if domain == "kimi.ai" {
					r = Global
				} else if domain != "kimi.com" {
					return errors.New("Invalid Kimi domain")
				}
				if err := merge(r); err != nil {
					return err
				}
			}
			for _, field := range []string{"base_url", "baseUrl"} {
				if base := str(field); base != "" {
					_, r, err := OfficialBase(base)
					if err != nil {
						return err
					}
					if err = merge(r); err != nil {
						return err
					}
				}
			}
			if rawBases, exists := x["api_base_urls"]; exists {
				bases, ok := rawBases.(map[string]any)
				if !ok {
					return errors.New("Invalid Kimi protocol address")
				}
				for _, v := range bases {
					base, ok := v.(string)
					if !ok {
						return errors.New("Invalid Kimi protocol address")
					}
					_, r, err := OfficialBase(base)
					if err != nil {
						return err
					}
					if err = merge(r); err != nil {
						return err
					}
				}
			}
			if protocol, exists := x["api_protocol"]; exists {
				p, ok := protocol.(string)
				if !ok || p != "" && p != "openai" && p != "anthropic" && p != "responses" && p != "openai_responses" && p != "kimi" {
					return errors.New("Invalid Kimi protocol")
				}
			}
			if evidence != "" {
				c.Region = evidence
			}
			if c.AuthType == "" {
				if c.APIKey != "" {
					c.AuthType = "api_key"
				} else {
					c.AuthType = "oauth"
				}
			}
			if expiry, ok := x["expires_at"]; ok {
				switch e := expiry.(type) {
				case float64:
					if e < 0 || e > 253402300799 || e != float64(int64(e)) {
						return errors.New("Invalid Kimi token expiry")
					}
					c.ExpiresAt = int64(e)
				case string:
					n, err := strconv.ParseInt(e, 10, 64)
					if err != nil {
						return errors.New("Invalid Kimi token expiry")
					}
					c.ExpiresAt = n
				default:
					return errors.New("Invalid Kimi token expiry")
				}
			}
			if expiry := str("expired"); expiry != "" {
				t, err := time.Parse(time.RFC3339, expiry)
				if err != nil {
					return errors.New("Invalid Kimi token expiry")
				}
				if c.ExpiresAt != 0 && c.ExpiresAt != t.Unix() {
					return errors.New("Conflicting Kimi token expiry")
				}
				c.ExpiresAt = t.Unix()
			}
			normalized, err := validate(c)
			if err != nil {
				return err
			}
			result = append(result, normalized)
			return nil
		default:
			return errors.New("Invalid Kimi credential JSON")
		}
	}
	var v any
	if common.UnmarshalJsonStr(raw, &v) == nil {
		if err := walk(v, 0, ""); err != nil {
			return nil, err
		}
	} else {
		for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if common.UnmarshalJsonStr(line, &v) != nil {
				return nil, errors.New("Invalid Kimi credential JSON")
			}
			if err := walk(v, 0, ""); err != nil {
				return nil, err
			}
		}
	}
	if len(result) == 0 && (issues == nil || len(*issues) == 0) {
		return nil, errors.New("No Kimi credentials found")
	}
	seen := map[string]bool{}
	out := make([]Credential, 0, len(result))
	for _, c := range result {
		secret := c.RefreshToken
		if secret == "" {
			secret = c.Token()
		}
		sum := sha256.Sum256([]byte(c.Region + "\x00" + c.AuthType + "\x00" + secret))
		id := fmt.Sprintf("%x", sum)
		if !seen[id] {
			out = append(out, c)
			seen[id] = true
		}
	}
	return out, nil
}
