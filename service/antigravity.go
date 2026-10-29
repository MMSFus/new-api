package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/antigravity"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

var antigravityRefreshGroup singleflight.Group

// Keys are hashes, and token values stay only in bounded process memory. The
// persisted channel retains the refresh token, so restart needs no migration.
var antigravityCredentials = hot.NewHotCache[string, antigravity.Credential](hot.LRU, 4096).WithTTL(45 * time.Minute).WithJanitor().Build()

func AntigravityHTTPClient(settings dto.ChannelSettings) (*http.Client, error) {
	base, err := GetHttpClientWithProxySettings(settings.Proxy, settings)
	if err != nil {
		return nil, err
	}
	if base == nil {
		base = &http.Client{}
	}
	return antigravity.NewClient(base), nil
}

// ResolveAntigravityCredential serializes refresh per credential and transport.
// Cache refreshed tokens; persistence separately compares the original channel
// key so concurrent administrator edits and multi-key offsets remain authoritative.
func ResolveAntigravityCredential(ctx context.Context, raw, base string, settings dto.ChannelSettings) (antigravity.Credential, error) {
	return resolveAntigravityCredential(ctx, raw, base, settings, false, "")
}

// RefreshAntigravityCredential retries a rejected access token once, sharing any
// newer credential already obtained by another in-flight request.
func RefreshAntigravityCredential(ctx context.Context, raw, base string, settings dto.ChannelSettings, rejected string) (antigravity.Credential, error) {
	return resolveAntigravityCredential(ctx, raw, base, settings, true, rejected)
}

func resolveAntigravityCredential(ctx context.Context, raw, base string, settings dto.ChannelSettings, force bool, rejected string) (antigravity.Credential, error) {
	creds, err := antigravity.Import(raw)
	if err != nil || len(creds) != 1 {
		return antigravity.Credential{}, errors.New("Invalid Antigravity channel credential")
	}
	original := creds[0]
	sum := sha256.Sum256([]byte(raw + "\x00" + base + "\x00" + settings.Proxy))
	key := fmt.Sprintf("%x", sum)
	if c, ok, _ := antigravityCredentials.Get(key); ok && !force && !c.NeedsRefresh() && c.ProjectID != "" {
		return c, nil
	}
	value, err, _ := antigravityRefreshGroup.Do(key, func() (any, error) {
		c := original
		if cached, ok, _ := antigravityCredentials.Get(key); ok {
			c = cached
		}
		client, err := AntigravityHTTPClient(settings)
		if err != nil {
			return nil, err
		}
		if c.NeedsRefresh() || (force && c.AccessToken == rejected) {
			c, err = antigravity.Refresh(ctx, client, c)
			if err != nil {
				return nil, err
			}
		}
		if c.ProjectID == "" {
			c.ProjectID, err = antigravity.DiscoverProject(ctx, client, base, c)
			if err != nil {
				return nil, err
			}
		}
		antigravityCredentials.Set(key, c)
		return c, nil
	})
	if err != nil {
		return antigravity.Credential{}, err
	}
	return value.(antigravity.Credential), nil
}

func FetchAntigravityModels(ctx context.Context, ch *model.Channel) ([]string, error) {
	keys, err := antigravity.Import(ch.Key)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(ch.GetBaseURL(), "/")
	if base == "" {
		base = antigravity.DefaultBaseURL
	}
	settings := ch.GetSetting()
	c, err := ResolveAntigravityCredential(ctx, keys[0].Key(), base, settings)
	if err != nil {
		return nil, err
	}
	client, err := AntigravityHTTPClient(settings)
	if err != nil {
		return nil, err
	}
	if err := PersistAntigravityCredential(ch.Id, keys[0].Key(), c); err != nil {
		return nil, err
	}
	return antigravity.Models(ctx, client, base, c)
}

func NormalizeAntigravityKeys(raw string) (string, int, error) {
	keys, err := antigravity.Import(raw)
	if err != nil {
		return "", 0, err
	}
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k.Key()
	}
	return strings.Join(lines, "\n"), len(lines), nil
}

func PersistAntigravityCredential(channelID int, selected string, credential antigravity.Credential) error {
	if channelID <= 0 {
		return nil
	}
	keys, err := antigravity.Import(selected)
	if err != nil || len(keys) != 1 {
		return errors.New("Invalid Antigravity channel credential")
	}
	if keys[0].Key() == credential.Key() {
		return nil
	}
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return err
	}
	// Only replace the selected original key. A concurrently edited credential
	// cannot be overwritten, and disabled-key indexes remain in the same order.
	lines := strings.Split(ch.Key, "\n")
	found := false
	for i, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(selected) {
			lines[i] = credential.Key()
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	changed, err := model.ReplaceChannelCredential(channelID, ch.Key, strings.Join(lines, "\n"))
	if err != nil {
		return errors.New("Failed to save refreshed Antigravity credential")
	}
	if changed {
		model.InitChannelCache()
	}
	return nil
}
