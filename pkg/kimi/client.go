package kimi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// NewClient reuses transport settings while preventing credential-bearing redirects.
func NewClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	c := *base
	c.Timeout = 30 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}
func Headers(h http.Header, c Credential) {
	h.Set("Authorization", "Bearer "+c.Token())
	h.Set("User-Agent", "new-api/"+common.Version)
	h.Set("X-Msh-Platform", "new-api")
	h.Set("X-Msh-Version", common.Version)
	h.Set("X-Msh-Device-Id", c.DeviceID)
}

type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type OAuthError struct {
	Code   string
	Status int
}

func (e *OAuthError) Error() string {
	switch e.Code {
	case "authorization_pending", "slow_down", "access_denied", "expired_token", "invalid_grant":
		return "Kimi OAuth: " + e.Code
	}
	return "Kimi OAuth request failed"
}

func postForm(ctx context.Context, client *http.Client, c Credential, path string, values url.Values, out any) error {
	values.Set("client_id", ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.AuthHost()+path, strings.NewReader(values.Encode()))
	if err != nil {
		return errors.New("Invalid Kimi OAuth request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	Headers(req.Header, c)
	req.Header.Del("Authorization")
	resp, err := NewClient(client).Do(req)
	if err != nil {
		return errors.New("Kimi OAuth connection failed")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return errors.New("Invalid Kimi OAuth response")
	}
	var failure struct {
		Error string `json:"error"`
	}
	_ = common.Unmarshal(b, &failure)
	if failure.Error != "" || resp.StatusCode != 200 {
		return &OAuthError{Code: failure.Error, Status: resp.StatusCode}
	}
	if common.Unmarshal(b, out) != nil {
		return errors.New("Invalid Kimi OAuth response")
	}
	return nil
}
func StartDevice(ctx context.Context, client *http.Client, c Credential) (DeviceAuthorization, error) {
	var d DeviceAuthorization
	err := postForm(ctx, client, c, "/api/oauth/device_authorization", url.Values{}, &d)
	if err != nil {
		return d, err
	}
	if d.DeviceCode == "" || d.UserCode == "" || d.ExpiresIn <= 0 || d.ExpiresIn > 3600 || d.Interval < 0 || d.Interval > 300 {
		return d, errors.New("Invalid Kimi device authorization")
	}
	expected := strings.TrimPrefix(c.AuthHost(), "https://")
	for _, raw := range []string{d.VerificationURI, d.VerificationURIComplete} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || (u.Host != expected && u.Host != strings.Replace(expected, "auth.", "www.", 1) && u.Host != strings.Replace(expected, "auth.", "code.", 1)) || u.User != nil || u.Fragment != "" {
			return d, errors.New("Invalid Kimi verification address")
		}
	}
	if d.VerificationURI == "" && d.VerificationURIComplete == "" {
		return d, errors.New("Kimi verification address is missing")
	}
	d.Interval = max(d.Interval, 5)
	return d, nil
}
func Token(ctx context.Context, client *http.Client, c Credential, deviceCode string) (Credential, error) {
	values := url.Values{}
	if deviceCode != "" {
		values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		values.Set("device_code", deviceCode)
	} else {
		if c.RefreshToken == "" {
			return c, errors.New("Kimi credential cannot be renewed; authorize again")
		}
		values.Set("grant_type", "refresh_token")
		values.Set("refresh_token", c.RefreshToken)
	}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		Type    string `json:"token_type"`
		Scope   string `json:"scope"`
	}
	if err := postForm(ctx, client, c, "/api/oauth/token", values, &result); err != nil {
		return c, err
	}
	if result.Access == "" || result.Expires <= 0 || result.Expires > 365*86400 {
		return c, errors.New("Invalid Kimi token response")
	}
	c.AccessToken = result.Access
	if result.Refresh != "" {
		c.RefreshToken = result.Refresh
	}
	if deviceCode != "" && c.RefreshToken == "" {
		return c, errors.New("Kimi OAuth response is missing refresh token")
	}
	c.ExpiresAt = time.Now().Unix() + result.Expires
	c.TokenType = result.Type
	c.Scope = result.Scope
	c.AuthType = "oauth"
	c.AccountMode = "coding"
	return validate(c)
}

type MetadataError struct{ Status int }

func (e *MetadataError) Error() string { return "Kimi metadata request failed" }

type Model struct {
	ID              string `json:"id"`
	Protocol        string `json:"protocol"`
	SupportsToolUse *bool  `json:"supports_tool_use,omitempty"`
	Reasoning       struct {
		Type   string `json:"type"`
		Effort struct {
			Valid   []string `json:"valid"`
			Default string   `json:"default"`
		} `json:"effort"`
	} `json:"reasoning"`
	Modalities struct {
		Input []string `json:"input"`
	} `json:"modalities"`
}

func Models(ctx context.Context, client *http.Client, c Credential) ([]Model, error) {
	b, err := Get(ctx, client, c, "/models")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []Model `json:"data"`
	}
	if common.Unmarshal(b, &result) != nil || len(result.Data) == 0 {
		return nil, errors.New("Invalid Kimi model response")
	}
	return result.Data, nil
}
func Get(ctx context.Context, client *http.Client, c Credential, path string) ([]byte, error) {
	if path != "/models" && path != "/usages" {
		return nil, errors.New("Unsupported Kimi metadata request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	Headers(req.Header, c)
	resp, err := NewClient(client).Do(req)
	if err != nil {
		return nil, errors.New("Kimi metadata connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &MetadataError{Status: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return nil, errors.New("Invalid Kimi metadata response")
	}
	return b, nil
}
