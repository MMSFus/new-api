package ratio_setting

import (
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const exposedDataTTL = 30 * time.Second

type exposedCache struct {
	data      gin.H
	expiresAt time.Time
}

var (
	exposedData atomic.Value
	rebuildMu   sync.Mutex
)

func InvalidateExposedDataCache() {
	exposedData.Store((*exposedCache)(nil))
}

func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	maps.Copy(dst, src)
	return dst
}

func GetExposedData() gin.H {
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	newData := gin.H{
		"model_ratio":                     usdEffectivePrices(GetModelRatioCopy()),
		"completion_ratio":                GetCompletionRatioCopy(),
		"cache_ratio":                     GetCacheRatioCopy(),
		"create_cache_ratio":              GetCreateCacheRatioCopy(),
		"model_price":                     usdEffectivePrices(GetModelPriceCopy()),
		ModelPriceCurrencyOptionSyncField: GetModelPriceCurrencyCopy(),
	}
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
	return cloneGinH(newData)
}

// usdEffectivePrices converts exported ratios/prices of CNY/CUSTOM-marked
// models to USD so receivers, which treat synced prices as USD, stay correct.
// An entry whose exchange rate is unusable is omitted rather than exported
// with a wrong value.
func usdEffectivePrices(values map[string]float64) map[string]float64 {
	for name, value := range values {
		_, factor, err := ResolveModelPriceCurrency(name)
		if err != nil {
			delete(values, name)
			continue
		}
		if factor != 1 {
			values[name] = value * factor
		}
	}
	return values
}
