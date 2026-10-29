package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
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

// Bounded lock stripes coordinate saved and draft grants without retaining
// credential IDs in a process-wide map. Hash collisions only serialize grants.
var kimiRefreshLocks = func() [64]chan struct{} {
	var locks [64]chan struct{}
	for i := range locks {
		locks[i] = make(chan struct{}, 1)
	}
	return locks
}()
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
	if channelID <= 0 {
		c, err := kimi.Parse(raw)
		if err != nil {
			return c, err
		}
		if c.AuthType == "oauth" && (c.NeedsRefresh() || rejected != "" && c.Token() == rejected) {
			return c, errors.New("Kimi preview requires a session credential draft")
		}
		return c, nil
	}
	return resolveKimiCredential(ctx, raw, settings, rejected, "saved", func(original kimi.Credential) (kimi.Credential, error) {
		return storedKimiCredential(channelID, original)
	}, func(before, after kimi.Credential) error {
		if err := model.RotateKimiCredential(before, after); err != nil {
			return err
		}
		model.InitChannelCache()
		return nil
	})
}

// Draft resolution persists rotated tokens before model metadata is requested.
func ResolveKimiDraftModels(ctx context.Context, raw string, settings dto.ChannelSettings, scope string, read func(kimi.Credential) (kimi.Credential, error), persist func(kimi.Credential, kimi.Credential) error) ([]kimi.Model, error) {
	c, err := resolveKimiCredential(ctx, raw, settings, "", scope, read, persist)
	if err != nil {
		return nil, err
	}
	models, err := KimiModels(ctx, c, settings)
	var rejected *kimi.MetadataError
	if errors.As(err, &rejected) && rejected.Status == http.StatusUnauthorized && c.AuthType == "oauth" {
		c, err = resolveKimiCredential(ctx, c.Key(), settings, c.AccessToken, scope, read, persist)
		if err != nil {
			return nil, err
		}
		return KimiModels(ctx, c, settings)
	}
	return models, err
}

func resolveKimiCredential(ctx context.Context, raw string, settings dto.ChannelSettings, rejected, scope string, read func(kimi.Credential) (kimi.Credential, error), persist func(kimi.Credential, kimi.Credential) error) (kimi.Credential, error) {
	original, err := kimi.Parse(raw)
	if err != nil {
		return original, err
	}
	c, err := read(original)
	if err != nil {
		return c, err
	}
	if !c.NeedsRefresh() && (rejected == "" || c.Token() != rejected) || c.AuthType == "api_key" {
		return c, nil
	}
	result := kimiRefreshGroup.DoChan(scope+":"+c.Region+":"+c.ID, func() (any, error) {
		grantCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		release, verifyLease, err := acquireKimiRefreshLock(grantCtx, c.Region+":"+c.ID, cancel)
		if err != nil {
			return nil, err
		}
		defer release()
		latest, err := read(original)
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
			current, readErr := read(latest)
			if readErr == nil && current.RefreshToken != latest.RefreshToken && !current.NeedsRefresh() {
				return current, nil
			}
			return nil, err
		}
		if grantCtx.Err() != nil || verifyLease() != nil {
			return nil, errors.New("Kimi renewal lost its lease; authorize again")
		}
		if err := persist(latest, renewed); err != nil {
			return nil, errors.New("Failed to save renewed Kimi credential; authorize again")
		}
		return renewed, nil
	})
	if strings.HasPrefix(scope, "draft:") {
		// Keep the draft reserved until the grant has finished persisting, even
		// when the metadata deadline fires while upstream is returning a token.
		r := <-result
		if r.Err != nil {
			return c, r.Err
		}
		return r.Val.(kimi.Credential), nil
	}
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
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(id))
		lock := kimiRefreshLocks[hash.Sum32()%uint32(len(kimiRefreshLocks))]
		select {
		case lock <- struct{}{}:
			return func() { <-lock }, func() error { return ctx.Err() }, nil
		case <-ctx.Done():
			return nil, nil, errors.New("Timed out waiting for Kimi renewal")
		}
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
