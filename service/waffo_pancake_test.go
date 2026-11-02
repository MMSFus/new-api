package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	testPancakeMerchantID = "MER_AbCdEfGhIjKlMnOpQrStUv"
	testPancakeStoreID    = "STO_AbCdEfGhIjKlMnOpQrStUv"
	testPancakeTopUpID    = "PROD_TopUpAAAAAAAAAAAAAAAAA"
	testPancakePlanID     = "PROD_PlanBBBBBBBBBBBBBBBBBB"
)

// fakePancakeAPI records every SDK call by path and answers with canned
// envelopes. graphql is the raw `data` of the price query.
type fakePancakeAPI struct {
	mu        sync.Mutex
	bodies    map[string][]map[string]any
	graphql   string
	failPaths map[string]bool
}

func (f *fakePancakeAPI) calls(path string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[path]
}

func newFakePancakeAPI(t *testing.T, graphql string) *fakePancakeAPI {
	t.Helper()
	f := &fakePancakeAPI{bodies: map[string][]map[string]any{}, graphql: graphql, failPaths: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = common.Unmarshal(raw, &body)
		f.mu.Lock()
		f.bodies[r.URL.Path] = append(f.bodies[r.URL.Path], body)
		fail := f.failPaths[r.URL.Path]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"message":"rejected by test"}]}`)
			return
		}
		switch r.URL.Path {
		case "/v1/graphql":
			_, _ = io.WriteString(w, `{"data":`+f.graphql+`}`)
		case "/v1/actions/auth/issue-session-token":
			_, _ = io.WriteString(w, `{"data":{"token":"tok","expiresAt":"z"}}`)
		case "/v1/actions/checkout/create-session":
			_, _ = io.WriteString(w, `{"data":{"sessionId":"cs_1","checkoutUrl":"https://pay.example/cs_1","expiresAt":"z"}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{"product":{"id":"`+testPancakeTopUpID+`","prices":{},"media":[],"metadata":{},"status":"active"}}}`)
		}
	}))
	t.Cleanup(srv.Close)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	prev := struct {
		baseURL, merchant, key, store, product, currency string
	}{waffoPancakeBaseURL, setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeStoreID, setting.WaffoPancakeProductID, setting.WaffoPancakeCurrency}
	t.Cleanup(func() {
		waffoPancakeBaseURL = prev.baseURL
		setting.WaffoPancakeMerchantID = prev.merchant
		setting.WaffoPancakePrivateKey = prev.key
		setting.WaffoPancakeStoreID = prev.store
		setting.WaffoPancakeProductID = prev.product
		setting.WaffoPancakeCurrency = prev.currency
	})
	waffoPancakeBaseURL = srv.URL
	setting.WaffoPancakeMerchantID = testPancakeMerchantID
	setting.WaffoPancakePrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	setting.WaffoPancakeStoreID = testPancakeStoreID
	setting.WaffoPancakeProductID = testPancakeTopUpID
	setting.WaffoPancakeCurrency = "USD"
	return f
}

// setupPancakeSaveDB provides the option table and plan table SaveWaffoPancakeConfig touches.
func setupPancakeSaveDB(t *testing.T, plans ...model.SubscriptionPlan) {
	t.Helper()
	prevDB, prevMap := model.DB, common.OptionMap
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SubscriptionPlan{}))
	for i := range plans {
		require.NoError(t, db.Create(&plans[i]).Error)
	}
	model.DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = prevDB
		common.OptionMap = prevMap
		_ = sqlDB.Close()
	})
}

func savedOption(t *testing.T, key string) string {
	t.Helper()
	var option model.Option
	if err := model.DB.Where("key = ?", key).First(&option).Error; err != nil {
		return ""
	}
	return option.Value
}

func TestValidateWaffoPancakeCurrency(t *testing.T) {
	cases := []struct {
		code  string
		valid bool
	}{
		{"USD", true},
		{"JPY", true},
		{"BHD", true},
		{"usd", false},
		{"XXX", false},
		{"ABC", false},
		{"US", false},
		{"USDT", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			err := setting.ValidateWaffoPancakeCurrency(tc.code)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestFormatWaffoPancakeAmountUsesCurrencyMinorUnits(t *testing.T) {
	cases := []struct {
		currency  string
		amount    string
		formatted string
		minCharge string
	}{
		{"USD", "29.999", "30.00", "0.01"},
		{"USD", "9.9", "9.90", "0.01"},
		{"JPY", "1234.5", "1235", "1"},
		{"JPY", "1000", "1000", "1"},
		{"KRW", "0.4", "0", "1"},
		{"BHD", "1.2345", "1.235", "0.001"},
	}
	for _, tc := range cases {
		t.Run(tc.currency+"_"+tc.amount, func(t *testing.T) {
			amount := decimal.RequireFromString(tc.amount)
			assert.Equal(t, tc.formatted, setting.FormatWaffoPancakeAmount(amount, tc.currency))
			assert.Equal(t, tc.minCharge, setting.WaffoPancakeMinChargeAmount(tc.currency).String())
		})
	}
}

func TestCreateWaffoPancakeCheckoutSessionSendsConfiguredCurrency(t *testing.T) {
	api := newFakePancakeAPI(t, `{}`)
	setting.WaffoPancakeCurrency = "JPY"

	_, err := CreateWaffoPancakeCheckoutSession(context.Background(), &WaffoPancakeCreateSessionParams{
		ProductID:               testPancakeTopUpID,
		BuyerIdentity:           WaffoPancakeBuyerIdentityFromUserID(7),
		PriceSnapshot:           &WaffoPancakePriceSnapshot{Amount: "1500", TaxCategory: "saas"},
		OrderMerchantExternalID: "WAFFO_PANCAKE-7-1-abc",
	})
	require.NoError(t, err)

	calls := api.calls("/v1/actions/checkout/create-session")
	require.Len(t, calls, 1)
	assert.Equal(t, "JPY", calls[0]["currency"])
	assert.Equal(t, map[string]any{"amount": "1500", "taxCategory": "saas"}, calls[0]["priceSnapshot"])
}

func TestCreateWaffoPancakePrimaryProductSeedsPriceInConfiguredCurrency(t *testing.T) {
	api := newFakePancakeAPI(t, `{}`)
	setting.WaffoPancakeCurrency = "JPY"

	_, err := CreateWaffoPancakePrimaryProduct(context.Background(), testPancakeMerchantID, setting.WaffoPancakePrivateKey, testPancakeStoreID, "")
	require.NoError(t, err)

	calls := api.calls("/v1/actions/onetime-product/create-product")
	require.Len(t, calls, 1)
	assert.Equal(t, map[string]any{"JPY": map[string]any{"amount": "1", "taxCategory": "saas"}}, calls[0]["prices"])
}

func TestSaveWaffoPancakeConfigRejectsInvalidCurrencyWithoutSaving(t *testing.T) {
	api := newFakePancakeAPI(t, `{}`)
	setupPancakeSaveDB(t)

	for _, code := range []string{"usd", "XXX", "ZZZ", "DOLLAR"} {
		_, err := SaveWaffoPancakeConfig(context.Background(), testPancakeMerchantID, "", "", code, testPancakeStoreID, testPancakeTopUpID)
		require.ErrorIs(t, err, ErrWaffoPancakeInvalidCurrency, code)
	}
	assert.Equal(t, "USD", setting.WaffoPancakeCurrency)
	assert.Empty(t, savedOption(t, "WaffoPancakeCurrency"))
	assert.Empty(t, api.calls("/v1/graphql"))
}

const pancakeProductsWithUSDOnly = `{"stores":[{"onetimeProducts":[
	{"id":"` + testPancakeTopUpID + `","prices":[{"currency":"USD","priceInfo":{"amount":"1.00","taxCategory":"saas"}}]},
	{"id":"` + testPancakePlanID + `","prices":[{"currency":"USD","priceInfo":{"amount":"9.99","taxCategory":"saas"}}]}
]}]}`

func TestSaveWaffoPancakeConfigAddsNewCurrencyPriceToBoundProducts(t *testing.T) {
	api := newFakePancakeAPI(t, pancakeProductsWithUSDOnly)
	setupPancakeSaveDB(t,
		model.SubscriptionPlan{Id: 1, Title: "Pro", PriceAmount: 1500, Currency: "USD", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, WaffoPancakeProductId: testPancakePlanID},
		model.SubscriptionPlan{Id: 2, Title: "Epay only", PriceAmount: 5, Currency: "USD", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1},
	)

	result, err := SaveWaffoPancakeConfig(context.Background(), testPancakeMerchantID, "", "", "JPY", testPancakeStoreID, testPancakeTopUpID)
	require.NoError(t, err)

	assert.Equal(t, "JPY", setting.WaffoPancakeCurrency)
	assert.Equal(t, "JPY", savedOption(t, "WaffoPancakeCurrency"))
	require.NotNil(t, result)
	assert.Equal(t, 1, result.PlanCount)
	assert.Equal(t, []string{testPancakeTopUpID, testPancakePlanID}, result.UpdatedProducts)

	updates := api.calls("/v1/actions/onetime-product/update-product")
	require.Len(t, updates, 2)
	assert.Equal(t, map[string]any{
		"USD": map[string]any{"amount": "1.00", "taxCategory": "saas"},
		"JPY": map[string]any{"amount": "1", "taxCategory": "saas"},
	}, updates[0]["prices"], "existing USD price is kept")
	assert.Equal(t, map[string]any{"amount": "1500", "taxCategory": "saas"}, updates[1]["prices"].(map[string]any)["JPY"])
	assert.Len(t, api.calls("/v1/actions/onetime-product/publish-product"), 2)
}

func TestSaveWaffoPancakeConfigKeepsOldCurrencyWhenPriceSyncFails(t *testing.T) {
	api := newFakePancakeAPI(t, pancakeProductsWithUSDOnly)
	api.failPaths["/v1/actions/onetime-product/publish-product"] = true
	setupPancakeSaveDB(t)

	_, err := SaveWaffoPancakeConfig(context.Background(), testPancakeMerchantID, "", "", "JPY", testPancakeStoreID, testPancakeTopUpID)

	var syncErr *WaffoPancakePriceSyncError
	require.ErrorAs(t, err, &syncErr)
	assert.Contains(t, syncErr.Error(), testPancakeTopUpID)
	assert.Equal(t, "USD", setting.WaffoPancakeCurrency)
	assert.Empty(t, savedOption(t, "WaffoPancakeCurrency"))
}

func TestSaveWaffoPancakeConfigSkipsSyncWhenCurrencyUnchanged(t *testing.T) {
	api := newFakePancakeAPI(t, pancakeProductsWithUSDOnly)
	setupPancakeSaveDB(t)

	result, err := SaveWaffoPancakeConfig(context.Background(), testPancakeMerchantID, "", "", "", testPancakeStoreID, testPancakeTopUpID)
	require.NoError(t, err)

	assert.Nil(t, result)
	assert.Equal(t, "USD", savedOption(t, "WaffoPancakeCurrency"), "blank currency keeps the current value")
	assert.Empty(t, api.calls("/v1/graphql"))
}
