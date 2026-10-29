package kimi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestImportRegionalCredentials(t *testing.T) {
	for _, tc := range []struct{ name, raw, region, wantRegion, auth string }{
		{"CPA mainland", `{"type":"kimi","refresh_token":"rotate","expired":"2030-01-01T00:00:00Z"}`, "", Mainland, "oauth"},
		{"CPA international", `{"type":"kimi-ai","domain":"kimi.ai","access_token":"access","timestamp":"old"}`, "", Global, "oauth"},
		{"sub coding key", `{"type":"sub2api-data","accounts":[{"platform":"claude","expired":true,"credentials":{"api_key":"skip"}},{"platform":"kimi","type":"apikey","expires_at":1,"name":"coding","credentials":{"api_key":"coding-key","account_mode":"coding","base_url":"https://api.kimi.ai/coding"}}]}`, "", Global, "api_key"},
		{"official file", `{"access_token":"access","refresh_token":"refresh","expires_at":2000000000}`, Mainland, Mainland, "oauth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts, err := Import(tc.raw, tc.region)
			require.NoError(t, err)
			require.Len(t, accounts, 1)
			c := accounts[0]
			assert.Equal(t, tc.wantRegion, c.Region)
			assert.Equal(t, tc.auth, c.AuthType)
			restored, err := Parse(c.Key())
			require.NoError(t, err)
			assert.Equal(t, c, restored)
			if tc.name == "sub coding key" {
				assert.Zero(t, c.ExpiresAt)
				assert.Equal(t, "coding", c.Name)
			}
		})
	}
}
func TestImportRejectsConflictAndUntrustedTargets(t *testing.T) {
	for _, raw := range []string{
		`{"type":"kimi","domain":"kimi.ai","refresh_token":"secret"}`,
		`{"type":"kimi-ai","base_url":"https://evil.example/coding/v1","refresh_token":"secret"}`,
		`{"platform":"kimi","credentials":{"api_key":"secret","account_mode":"payg"}}`,
		`{"refresh_token":42}`, `{"refresh_token":"secret","expires_at":1.5}`,
		`{"refresh_token":"secret","api_base_urls":[]}`,
		`{"refresh_token":"secret","region":"unknown"}`,
		`{"refresh_token":"secret","access_token":"bad\r\nheader"}`,
	} {
		_, err := Import(raw, Mainland)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret")
	}
	for _, base := range []string{"http://api.kimi.ai/coding", "https://api.kimi.ai/coding/v1?token=secret", "https://user@api.kimi.ai/coding", "https://api.kimi.ai:444/coding", "https://api.kimi.ai/v1", "https://api.kimi.ai/coding%2fv1"} {
		_, _, err := OfficialBase(base)
		require.Error(t, err)
	}
}
func TestPreviewKeepsValidEntriesAndReportsInvalidOnes(t *testing.T) {
	raw := "{\"type\":\"kimi\",\"refresh_token\":\"one\"}\n{\"platform\":\"kimi\",\"credentials\":{\"api_key\":\"payg-secret\",\"account_mode\":\"payg\"}}\n{\"type\":\"kimi\",\"refresh_token\":\"one\"}"
	accounts, issues, err := Preview(raw, Mainland)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Error, "Moonshot")
	assert.NotContains(t, issues[0].Error, "payg-secret")
	_, err = Import(raw, Mainland)
	require.Error(t, err)
	_, _, err = Preview(strings.Repeat(" ", MaxImportBytes+1), Mainland)
	require.Error(t, err)
	_, _, err = Preview(`[[[[[[{"refresh_token":"too-deep"}]]]]]]`, Mainland)
	require.Error(t, err)
}
func TestOAuthContractRotationAndRegionIsolation(t *testing.T) {
	for _, region := range []string{Mainland, Global} {
		t.Run(region, func(t *testing.T) {
			c := Credential{Region: region, DeviceID: "persistent-device", RefreshToken: "old-refresh"}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, c.AuthHost()+"/api/oauth/token", r.URL.String())
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Equal(t, "persistent-device", r.Header.Get("X-Msh-Device-Id"))
				require.NoError(t, r.ParseForm())
				assert.Equal(t, ClientID, r.Form.Get("client_id"))
				assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
				assert.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
				return response(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer"}`, 200), nil
			})}
			renewed, err := Token(context.Background(), client, c, "")
			require.NoError(t, err)
			assert.Equal(t, "new-refresh", renewed.RefreshToken)
			assert.Equal(t, region, renewed.Region)
			assert.InDelta(t, time.Now().Unix()+3600, renewed.ExpiresAt, 2)
			client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(`{"access_token":"newer","expires_in":3600}`, 200), nil
			})
			next, err := Token(context.Background(), client, renewed, "")
			require.NoError(t, err)
			assert.Equal(t, renewed.ID, next.ID)
			assert.Equal(t, "new-refresh", next.RefreshToken)
		})
	}
}
func TestDeviceAuthorizationAndSafeErrors(t *testing.T) {
	c := Credential{Region: Global}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "auth.kimi.ai", r.URL.Host)
		return response(`{"device_code":"private-device","user_code":"ABCD","verification_uri":"https://auth.kimi.ai/device","expires_in":600,"interval":1}`, 200), nil
	})}
	device, err := StartDevice(context.Background(), client, c)
	require.NoError(t, err)
	assert.Equal(t, 5, device.Interval)
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"device_code":"x","user_code":"x","verification_uri":"https://evil.example/secret","expires_in":600}`, 200), nil
	})
	_, err = StartDevice(context.Background(), client, c)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"error":"slow_down","error_description":"secret"}`, 400), nil
	})
	_, err = Token(context.Background(), client, c, "device")
	var oauthErr *OAuthError
	require.ErrorAs(t, err, &oauthErr)
	assert.Equal(t, "slow_down", oauthErr.Code)
	assert.NotContains(t, err.Error(), "secret")
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		r := response("secret", 302)
		r.Header.Set("Location", "https://evil.example")
		return r, nil
	})
	_, err = Token(context.Background(), client, c, "device")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
}
