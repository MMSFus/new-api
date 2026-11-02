package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	exchangeRateSyncTickInterval   = time.Minute
	exchangeRateRequestTimeout     = 10 * time.Second
	exchangeRateMaxResponseBytes   = 1 << 20
	exchangeRateOpenERAPIURL       = "https://open.er-api.com/v6/latest/USD"
	exchangeRateOpenERAPISource    = "open.er-api.com"
	minAcceptedUSDToCNYRate        = 1.0
	maxAcceptedUSDToCNYRate        = 100.0
	exchangeRateSyncOptionKeyAt    = operation_setting.ExchangeRateSettingName + ".last_synced_at"
	exchangeRateSyncOptionKeyErr   = operation_setting.ExchangeRateSettingName + ".last_error"
	exchangeRateSyncOptionKeySrc   = operation_setting.ExchangeRateSettingName + ".last_source"
	exchangeRateUSDOptionKey       = "USDExchangeRate"
	exchangeRateRequestFailedKey   = "Exchange rate request failed"
	exchangeRateInvalidResponseKey = "Exchange rate source returned an invalid response"
	exchangeRateOutOfRangeKey      = "Synced exchange rate is outside the accepted range"
	exchangeRateSaveFailedKey      = "Failed to save synced exchange rate"
)

var (
	errExchangeRateInvalidResponse = errors.New("invalid exchange rate response")

	exchangeRateSyncOnce    sync.Once
	exchangeRateSyncRunning atomic.Bool

	// DefaultExchangeRateSource is the source used by the scheduler and the manual sync endpoint.
	DefaultExchangeRateSource ExchangeRateSource = &OpenERAPISource{URL: exchangeRateOpenERAPIURL}
)

// ExchangeRateSource fetches how many CNY one USD buys.
type ExchangeRateSource interface {
	Name() string
	FetchUSDToCNY(ctx context.Context) (float64, error)
}

// OpenERAPISource reads rates.CNY from the open.er-api.com USD endpoint.
// Client is optional; the general outbound client is used when it is nil.
type OpenERAPISource struct {
	URL    string
	Client *http.Client
}

func (s *OpenERAPISource) Name() string {
	return exchangeRateOpenERAPISource
}

func (s *OpenERAPISource) FetchUSDToCNY(ctx context.Context) (float64, error) {
	client := s.Client
	if client == nil {
		client = GetHttpClient()
	}
	if client == nil {
		return 0, errors.New("http client is not initialized")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, exchangeRateMaxResponseBytes+1))
	if err != nil {
		return 0, err
	}
	if len(body) > exchangeRateMaxResponseBytes {
		return 0, fmt.Errorf("%w: response body exceeds %d bytes", errExchangeRateInvalidResponse, exchangeRateMaxResponseBytes)
	}
	var payload struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("%w: %v", errExchangeRateInvalidResponse, err)
	}
	if payload.Result != "success" {
		return 0, fmt.Errorf("%w: result=%q", errExchangeRateInvalidResponse, payload.Result)
	}
	rate, ok := payload.Rates["CNY"]
	if !ok {
		return 0, fmt.Errorf("%w: rates.CNY is missing", errExchangeRateInvalidResponse)
	}
	return rate, nil
}

// ExchangeRateSyncResult is the state after one sync attempt.
type ExchangeRateSyncResult struct {
	USDExchangeRate float64 `json:"usd_exchange_rate"`
	LastSyncedAt    int64   `json:"last_synced_at"`
	LastError       string  `json:"last_error"`
	LastSource      string  `json:"last_source"`
}

// SyncUSDExchangeRate fetches the rate from source and saves it through the
// option update path, so the database, this node and other nodes see the same
// value as a manual edit. On any failure USDExchangeRate keeps its value and
// the returned error is a *common.Message whose key is also stored as last_error.
// Only USDExchangeRate is written; custom currency rates are untouched.
func SyncUSDExchangeRate(ctx context.Context, source ExchangeRateSource) (*ExchangeRateSyncResult, error) {
	if !exchangeRateSyncRunning.CompareAndSwap(false, true) {
		return nil, common.NewMessage("Exchange rate sync is already running")
	}
	defer exchangeRateSyncRunning.Store(false)

	result := &ExchangeRateSyncResult{
		USDExchangeRate: operation_setting.USDExchangeRate,
		LastSyncedAt:    time.Now().Unix(),
		LastSource:      source.Name(),
	}

	fetchCtx, cancel := context.WithTimeout(ctx, exchangeRateRequestTimeout)
	defer cancel()
	rate, err := source.FetchUSDToCNY(fetchCtx)
	switch {
	case errors.Is(err, errExchangeRateInvalidResponse):
		result.LastError = exchangeRateInvalidResponseKey
	case err != nil:
		result.LastError = exchangeRateRequestFailedKey
	case math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= minAcceptedUSDToCNYRate || rate >= maxAcceptedUSDToCNYRate:
		result.LastError = exchangeRateOutOfRangeKey
		err = fmt.Errorf("rate %v is outside (%v, %v)", rate, minAcceptedUSDToCNYRate, maxAcceptedUSDToCNYRate)
	}

	values := map[string]string{
		exchangeRateSyncOptionKeyAt:  strconv.FormatInt(result.LastSyncedAt, 10),
		exchangeRateSyncOptionKeyErr: result.LastError,
		exchangeRateSyncOptionKeySrc: result.LastSource,
	}
	if result.LastError == "" {
		values[exchangeRateUSDOptionKey] = strconv.FormatFloat(rate, 'f', -1, 64)
	}
	if saveErr := model.UpdateOptionsBulk(values); saveErr != nil {
		logger.LogError(ctx, common.LogText("failed to save exchange rate sync result: %v", saveErr))
		if result.LastError == "" {
			// Nothing was committed, so the old rate is still in effect.
			result.LastError = exchangeRateSaveFailedKey
			err = saveErr
			delete(values, exchangeRateUSDOptionKey)
			values[exchangeRateSyncOptionKeyErr] = result.LastError
			if retryErr := model.UpdateOptionsBulk(values); retryErr != nil {
				logger.LogError(ctx, common.LogText("failed to save exchange rate sync status: %v", retryErr))
			}
		}
	}

	if result.LastError != "" {
		logger.LogWarn(ctx, common.LogText("exchange rate sync from %s failed: %s: %v", result.LastSource, result.LastError, err))
		return result, common.NewMessage(result.LastError)
	}
	result.USDExchangeRate = rate
	logger.LogInfo(ctx, common.LogText("exchange rate synced from %s: USDExchangeRate=%v", result.LastSource, rate))
	return result, nil
}

// StartExchangeRateAutoSyncTask checks every minute on the master node whether
// a scheduled sync is due. Enabling, disabling or changing the interval takes
// effect on the next check without a restart.
func StartExchangeRateAutoSyncTask() {
	exchangeRateSyncOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ctx := context.Background()
			logger.LogInfo(ctx, common.LogText("exchange rate auto-sync task started: tick=%s", exchangeRateSyncTickInterval))
			ticker := time.NewTicker(exchangeRateSyncTickInterval)
			defer ticker.Stop()
			for range ticker.C {
				common.OptionMapRWMutex.RLock()
				setting := *operation_setting.GetExchangeRateSetting()
				common.OptionMapRWMutex.RUnlock()
				if !setting.AutoSyncEnabled {
					continue
				}
				interval := time.Duration(operation_setting.NormalizeExchangeRateSyncInterval(setting.SyncIntervalHours)) * time.Hour
				if time.Since(time.Unix(setting.LastSyncedAt, 0)) < interval {
					continue
				}
				_, _ = SyncUSDExchangeRate(ctx, DefaultExchangeRateSource)
			}
		})
	})
}
