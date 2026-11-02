package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeTieredQuota_ClampOnOverflow guards the billing-safety invariant
// that an oversized tiered settlement clamps to the single-request max instead of
// wrapping into a credit, and that the saturation event is surfaced on the
// result so callers can record it for admin auditing.
func TestComputeTieredQuota_ClampOnOverflow(t *testing.T) {
	// exprOutput = p * 1e12 = 1e21; quotaBeforeGroup = 1e21 / 1e6 * 5e5 = 5e20,
	// which far exceeds the supported single-request range and must saturate.
	exprStr := `tier("base", p * 1000000000000)`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1.0,
		QuotaPerUnit: 500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 1_000_000_000})
	require.NoError(t, err)

	assert.Equal(t, common.MaxQuota, result.ActualQuotaAfterGroup, "oversized quota must clamp, never wrap negative")
	require.NotNil(t, result.Clamp, "clamp event must be surfaced so it can be audited")
	assert.Equal(t, common.QuotaClampOverflow, result.Clamp.Kind)
	assert.Equal(t, common.MaxQuota, result.Clamp.Clamped)
}

// TestComputeTieredQuota_NoClampInRange confirms an in-range settlement leaves
// Clamp nil, so the audit path is a no-op in the common case.
func TestComputeTieredQuota_NoClampInRange(t *testing.T) {
	exprStr := `tier("base", p * 2 + c * 10)`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1.0,
		QuotaPerUnit: 500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 1000, C: 500})
	require.NoError(t, err)
	assert.Nil(t, result.Clamp, "in-range settlement must not report a clamp")
}

// The frozen currency factor scales both token and task conversion; a snapshot
// serialized before the fields existed has no factor and settles as USD.
func TestComputeTieredQuotaCurrencyFactor(t *testing.T) {
	tokenExpr := `tier("base", p * 16)`
	taskExpr := `tier("base", u("seconds") * 0.8)`
	legacyJSON := `{"billing_mode":"tiered_expr","expr_string":"tier(\"base\", p * 16)","group_ratio":1,"quota_per_unit":500000,"expr_version":1}`

	var legacy billingexpr.BillingSnapshot
	require.NoError(t, common.UnmarshalJsonStr(legacyJSON, &legacy))
	legacy.ExprHash = billingexpr.ExprHashString(legacy.ExprString)

	tests := []struct {
		name      string
		snap      *billingexpr.BillingSnapshot
		params    billingexpr.TokenParams
		request   billingexpr.RequestInput
		wantQuota int
	}{
		{"legacy snapshot without currency fields", &legacy, billingexpr.TokenParams{P: 1000}, billingexpr.RequestInput{}, 8000},
		{"token CNY factor", &billingexpr.BillingSnapshot{ExprString: tokenExpr, ExprHash: billingexpr.ExprHashString(tokenExpr), GroupRatio: 1, QuotaPerUnit: 500_000, CurrencyCode: "CNY", CurrencyUSDFactor: 0.125},
			billingexpr.TokenParams{P: 1000}, billingexpr.RequestInput{}, 1000},
		{"task USD", &billingexpr.BillingSnapshot{ExprString: taskExpr, ExprHash: billingexpr.ExprHashString(taskExpr), GroupRatio: 1, QuotaPerUnit: 500_000, TaskUsageBilling: true},
			billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"seconds": 10.0}}, 4_000_000},
		{"task CUSTOM factor", &billingexpr.BillingSnapshot{ExprString: taskExpr, ExprHash: billingexpr.ExprHashString(taskExpr), GroupRatio: 1, QuotaPerUnit: 500_000, TaskUsageBilling: true, CurrencyCode: "CUSTOM", CurrencyUSDFactor: 0.5},
			billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"seconds": 10.0}}, 2_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := billingexpr.ComputeTieredQuotaWithRequest(tt.snap, tt.params, tt.request)
			require.NoError(t, err)
			assert.Equal(t, tt.wantQuota, result.ActualQuotaAfterGroup)
		})
	}
	assert.Equal(t, 1.0, legacy.EffectiveCurrencyUSDFactor())
}
