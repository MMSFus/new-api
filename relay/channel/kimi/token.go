package kimi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// 网页版会话 token 刷新协议（与 Kimi 网页前端一致）：
//
//	POST {auth}/api/account.gateway.v1.AuthService/RefreshToken
//	{"refresh_token": "..."}  →  200 {"accessToken": "...", "refreshToken": "..."}
//
// access_token 有效期约 900 秒，refresh_token 约 90 天；服务端不吊销旧
// refresh_token。进程内按 refresh_token 缓存 access_token 避免每请求刷新。

type tokenEntry struct {
	accessToken string
	expiresAt   time.Time
}

var tokenCache sync.Map // refreshTokenHash -> *tokenEntry

// refreshSkew 在 access_token 过期前预留的刷新余量。
const refreshSkew = 60 * time.Second

// accessTokenLifetime 是网页协议 access_token 的标称有效期（900s），
// 响应不携带过期时间，按签发时间保守估算。
const accessTokenLifetime = 15 * time.Minute

// browserUA 用于通过 Cloudflare 的浏览器签名校验（非浏览器客户端会被 1010 拦截）。
const browserUA = "Mozilla/5.0 (X11; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0"

// EnsureAccessToken 返回可用 access_token，缓存未命中或临近过期时按
// 网页协议刷新。客户端构建与 relay 请求一致，继承渠道代理等传输配置。
func EnsureAccessToken(info *relaycommon.RelayInfo, key *KeyConfig, hosts Hosts) (string, error) {
	cacheKey := refreshTokenCacheKey(key.RefreshToken)
	if cached, ok := tokenCache.Load(cacheKey); ok {
		entry := cached.(*tokenEntry)
		if time.Until(entry.expiresAt) > refreshSkew {
			return entry.accessToken, nil
		}
	}
	return refreshAccessToken(info, key, hosts, cacheKey)
}

// refreshMu 串行化刷新，避免并发请求对同一 refresh_token 的刷新风暴。
var refreshMu sync.Mutex

func refreshAccessToken(info *relaycommon.RelayInfo, key *KeyConfig, hosts Hosts, cacheKey string) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()
	// 双检：等待锁期间其他 goroutine 可能已完成刷新。
	if cached, ok := tokenCache.Load(cacheKey); ok {
		entry := cached.(*tokenEntry)
		if time.Until(entry.expiresAt) > refreshSkew {
			return entry.accessToken, nil
		}
	}

	body, err := common.Marshal(map[string]string{"refresh_token": key.RefreshToken})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost,
		hosts.Auth+"/api/account.gateway.v1.AuthService/RefreshToken",
		strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", hosts.Site)
	req.Header.Set("Referer", hosts.Site+"/")
	req.Header.Set("User-Agent", browserUA)

	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return "", fmt.Errorf("kimi channel: build http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("kimi channel: refresh request failed: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("kimi channel: read refresh response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kimi channel: refresh token rejected (HTTP %d)", resp.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := common.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("kimi channel: invalid refresh response: %w", err)
	}
	if result.AccessToken == "" {
		return "", errors.New("kimi channel: refresh response missing accessToken")
	}
	tokenCache.Store(cacheKey, &tokenEntry{
		accessToken: result.AccessToken,
		expiresAt:   time.Now().Add(accessTokenLifetime),
	})
	return result.AccessToken, nil
}

func refreshTokenCacheKey(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(sum[:16])
}
