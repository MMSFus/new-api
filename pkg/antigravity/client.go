package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const DefaultBaseURL = "https://daily-cloudcode-pa.googleapis.com"
const RedirectURI = "http://localhost:8085/callback"
const UserAgent = "antigravity/1.23.2 linux/x64"
const tokenURL = "https://oauth2.googleapis.com/token"

// OAuth client credentials are deployment configuration for both login and refresh.
func OAuthClientID() string { return strings.TrimSpace(os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_ID")) }

func ValidateOAuthConfig() error {
	if OAuthClientID() == "" || strings.TrimSpace(os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET")) == "" {
		return errors.New("Configure ANTIGRAVITY_OAUTH_CLIENT_ID and ANTIGRAVITY_OAUTH_CLIENT_SECRET")
	}
	return nil
}

// NewClient reuses the host transport but never follows credential-bearing redirects.
func NewClient(base *http.Client) *http.Client {
	c := *base
	c.Timeout = 30 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}
func PKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}
func AuthorizationURL(state, challenge string) string {
	q := url.Values{"client_id": {OAuthClientID()}, "redirect_uri": {RedirectURI}, "response_type": {"code"}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "access_type": {"offline"}, "prompt": {"consent"}, "scope": {"https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/cclog https://www.googleapis.com/auth/experimentsandconfigs"}}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
}
func Callback(raw string) (code, state string, err error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "http" || u.Host != "localhost:8085" || u.Path != "/callback" || u.User != nil || u.Fragment != "" {
		return "", "", errors.New("Paste the complete Antigravity callback URL")
	}
	q := u.Query()
	if q.Get("error") != "" || len(q["code"]) != 1 || len(q["state"]) != 1 || q.Get("code") == "" || q.Get("state") == "" {
		return "", "", errors.New("Antigravity callback is missing code or state")
	}
	return q.Get("code"), q.Get("state"), nil
}
func Exchange(ctx context.Context, client *http.Client, code, verifier string) (Credential, error) {
	return token(ctx, client, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {RedirectURI}}, Credential{Type: "antigravity"})
}
func Refresh(ctx context.Context, client *http.Client, c Credential) (Credential, error) {
	if c.RefreshToken == "" {
		return c, errors.New("Antigravity credential requires a refresh token")
	}
	return token(ctx, client, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}}, c)
}
func token(ctx context.Context, client *http.Client, form url.Values, c Credential) (Credential, error) {
	form.Set("client_id", OAuthClientID())
	if err := ValidateOAuthConfig(); err != nil {
		return c, err
	}
	secret := strings.TrimSpace(os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET"))
	form.Set("client_secret", secret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return c, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return c, errors.New("Antigravity OAuth request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c, fmt.Errorf("Antigravity OAuth rejected the request (HTTP %d)", resp.StatusCode)
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if common.DecodeJson(io.LimitReader(resp.Body, 1<<20), &payload) != nil || payload.AccessToken == "" || payload.ExpiresIn <= 0 || payload.ExpiresIn > 86400*7 {
		return c, errors.New("Invalid Antigravity OAuth response")
	}
	c.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		c.RefreshToken = payload.RefreshToken
	}
	c.ExpiresAt = time.Now().Unix() + payload.ExpiresIn
	return c, nil
}
func RPC(ctx context.Context, client *http.Client, base, method string, c Credential, body any, out any) error {
	data, err := common.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1internal:"+method, bytes.NewReader(data))
	if err != nil {
		return errors.New("Invalid Antigravity API address")
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Antigravity API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Antigravity %s failed (HTTP %d)", method, resp.StatusCode)
	}
	if common.DecodeJson(io.LimitReader(resp.Body, 8<<20), out) != nil {
		return errors.New("Invalid Antigravity API response")
	}
	return nil
}
func DiscoverProject(ctx context.Context, client *http.Client, base string, c Credential) (string, error) {
	metadata := map[string]string{"ideType": "ANTIGRAVITY"}
	var load struct {
		Project any `json:"cloudaicompanionProject"`
		Allowed []struct {
			ID      string `json:"id"`
			Default bool   `json:"isDefault"`
		} `json:"allowedTiers"`
	}
	if err := RPC(ctx, client, base, "loadCodeAssist", c, map[string]any{"metadata": metadata}, &load); err != nil {
		return "", err
	}
	if p := projectID(load.Project); p != "" {
		return p, nil
	}
	tier := "free-tier"
	for _, v := range load.Allowed {
		if v.Default && v.ID != "" {
			tier = v.ID
			break
		}
	}
	for attempt := range 5 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
		}
		var onboard struct {
			Done     bool `json:"done"`
			Response struct {
				Project any `json:"cloudaicompanionProject"`
			} `json:"response"`
		}
		if err := RPC(ctx, client, base, "onboardUser", c, map[string]any{"tier_id": tier, "metadata": map[string]string{"ide_type": "ANTIGRAVITY", "ide_name": "antigravity", "ide_version": "1.23.2"}}, &onboard); err != nil {
			return "", err
		}
		if p := projectID(onboard.Response.Project); onboard.Done && p != "" {
			return p, nil
		}
	}
	return "", errors.New("Antigravity project provisioning is pending; try again later")
}
func projectID(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case map[string]any:
		s, _ := v["id"].(string)
		return s
	}
	return ""
}
func Models(ctx context.Context, client *http.Client, base string, c Credential) ([]string, error) {
	var out struct {
		Models map[string]any `json:"models"`
	}
	if err := RPC(ctx, client, base, "fetchAvailableModels", c, map[string]string{"project": c.ProjectID}, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Models))
	for id := range out.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
