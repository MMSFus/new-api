package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/kimi"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

var kimiRefreshGroup singleflight.Group
var kimiModelGroup singleflight.Group
var kimiModels = hot.NewHotCache[string, []kimi.Model](hot.LRU, 1024).WithTTL(5 * time.Minute).WithJanitor().Build()

func KimiHTTPClient(settings dto.ChannelSettings) (*http.Client, error) {
	client, err := GetHttpClientWithProxySettings(settings.Proxy, settings)
	if err != nil {
		return nil, err
	}
	return kimi.NewClient(client), nil
}
func NormalizeKimiKeys(raw string) (string, int, error) {
	// Already normalized keys retain their stable identity across token rotation.
	var credentials []kimi.Credential
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		c, err := kimi.Parse(line)
		if err != nil {
			credentials = nil
			break
		}
		credentials = append(credentials, c)
	}
	var err error
	if len(credentials) == 0 {
		credentials, err = kimi.Import(raw, "")
	}
	if err != nil {
		return "", 0, err
	}
	lines := make([]string, len(credentials))
	for i, c := range credentials {
		lines[i] = c.Key()
	}
	return strings.Join(lines, "\n"), len(lines), nil
}
func storedKimiCredential(channelID int, original kimi.Credential) (kimi.Credential, error) {
	if channelID <= 0 {
		return original, nil
	}
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return original, errors.New("Failed to read Kimi credential")
	}
	for line := range strings.SplitSeq(ch.Key, "\n") {
		c, err := kimi.Parse(line)
		if err == nil && c.ID == original.ID && c.Region == original.Region {
			return c, nil
		}
	}
	return original, errors.New("Kimi channel credential changed; retry the request")
}

// ResolveKimiCredential rereads shared storage inside the grant lock. No token
// cache may override the database or resurrect a credential removed by an admin.
func ResolveKimiCredential(ctx context.Context, channelID int, raw string, settings dto.ChannelSettings, rejected string) (kimi.Credential, error) {
	original, err := kimi.Parse(raw)
	if err != nil {
		return original, err
	}
	c, err := storedKimiCredential(channelID, original)
	if err != nil {
		return c, err
	}
	if !c.NeedsRefresh() && (rejected == "" || c.Token() != rejected) || c.AuthType == "api_key" {
		return c, nil
	}
	if channelID <= 0 {
		return c, errors.New("Save the Kimi channel before renewing its OAuth credential")
	}
	result := kimiRefreshGroup.DoChan(c.Region+":"+c.ID, func() (any, error) {
		grantCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		release, verifyLease, err := acquireKimiRefreshLock(grantCtx, c.Region+":"+c.ID, cancel)
		if err != nil {
			return nil, err
		}
		defer release()
		latest, err := storedKimiCredential(channelID, original)
		if err != nil {
			return nil, err
		}
		if !latest.NeedsRefresh() && (rejected == "" || latest.Token() != rejected) {
			return latest, nil
		}
		client, err := KimiHTTPClient(settings)
		if err != nil {
			return nil, err
		}
		renewed, err := kimi.Token(grantCtx, client, latest, "")
		if err != nil {
			current, readErr := storedKimiCredential(channelID, latest)
			if readErr == nil && current.RefreshToken != latest.RefreshToken && !current.NeedsRefresh() {
				return current, nil
			}
			return nil, err
		}
		if grantCtx.Err() != nil || verifyLease() != nil {
			return nil, errors.New("Kimi renewal lost its lease; authorize again")
		}
		if err := model.RotateKimiCredential(latest, renewed); err != nil {
			return nil, errors.New("Failed to save renewed Kimi credential; authorize again")
		}
		model.InitChannelCache()
		return renewed, nil
	})
	select {
	case <-ctx.Done():
		return c, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return c, r.Err
		}
		return r.Val.(kimi.Credential), nil
	}
}

// OAuth without Redis is supported only in a single gateway process. Redis
// leases never fall back to a local lock on outage or contention.
func acquireKimiRefreshLock(ctx context.Context, id string, cancel context.CancelFunc) (func(), func() error, error) {
	if !common.RedisEnabled {
		if !common.IsMasterNode {
			return nil, nil, errors.New("Kimi OAuth renewal on worker nodes requires Redis")
		}
		return func() {}, func() error { return ctx.Err() }, nil // singleflight serializes this process
	}
	key := "kimi:refresh:" + id
	owner := common.GetRandomString(32)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, err := common.RDB.SetNX(ctx, key, owner, 90*time.Second).Result()
		if err != nil {
			return nil, nil, errors.New("Kimi renewal coordination is unavailable")
		}
		if ok {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, errors.New("Timed out waiting for Kimi renewal")
		case <-ticker.C:
		}
	}
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-tick.C:
				n, err := common.RDB.Eval(ctx, logExportLockRenewScript, []string{key}, owner, (90 * time.Second).Milliseconds()).Int()
				if err != nil || n != 1 {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
			close(stop)
			releaseCtx, end := context.WithTimeout(context.Background(), 2*time.Second)
			defer end()
			_ = common.RDB.Eval(releaseCtx, logExportLockReleaseScript, []string{key}, owner).Err()
		}, func() error {
			n, err := common.RDB.Eval(ctx, logExportLockRenewScript, []string{key}, owner, (90 * time.Second).Milliseconds()).Int()
			if err != nil || n != 1 {
				return errors.New("Kimi renewal lost its lease")
			}
			return nil
		}, nil
}

func KimiModels(ctx context.Context, c kimi.Credential, settings dto.ChannelSettings) ([]kimi.Model, error) {
	// Regional identity and transport isolate account capabilities for five minutes.
	key := c.Region + ":" + c.ID + ":" + settings.Proxy
	if models, ok, _ := kimiModels.Get(key); ok {
		return models, nil
	}
	result := kimiModelGroup.DoChan(key, func() (any, error) {
		client, err := KimiHTTPClient(settings)
		if err != nil {
			return nil, err
		}
		models, err := kimi.Models(ctx, client, c)
		if err == nil {
			kimiModels.Set(key, models)
		}
		return models, err
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]kimi.Model), nil
	}
}

// ResolveKimiModels also renews an unexpired credential explicitly rejected by
// model discovery, before inference has had a chance to perform its own retry.
func ResolveKimiModels(ctx context.Context, channelID int, raw string, settings dto.ChannelSettings) (kimi.Credential, []kimi.Model, error) {
	c, err := ResolveKimiCredential(ctx, channelID, raw, settings, "")
	if err != nil {
		return c, nil, err
	}
	models, err := KimiModels(ctx, c, settings)
	var rejected *kimi.MetadataError
	if errors.As(err, &rejected) && rejected.Status == http.StatusUnauthorized && c.AuthType == "oauth" {
		c, err = ResolveKimiCredential(ctx, channelID, raw, settings, c.AccessToken)
		if err != nil {
			return c, nil, err
		}
		models, err = KimiModels(ctx, c, settings)
	}
	return c, models, err
}

func FetchKimiModels(ctx context.Context, ch *model.Channel) ([]string, error) {
	key, _, apiErr := ch.GetNextEnabledKey()
	if apiErr != nil {
		return nil, apiErr
	}
	_, models, err := ResolveKimiModels(ctx, ch.Id, key, ch.GetSetting())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}
func ValidateKimiBase(base string, c kimi.Credential) error {
	if strings.TrimSpace(base) == "" {
		return nil
	}
	_, region, err := kimi.OfficialBase(base)
	if err != nil {
		return err
	}
	if region != c.Region {
		return fmt.Errorf("Kimi channel address conflicts with credential region")
	}
	return nil
}
