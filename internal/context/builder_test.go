package context

import (
	"testing"
	"time"
	go_std_context "context" // Aliased import for standard context

	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTraceContext(t *testing.T) {
	tc := NewTraceContext(go_std_context.Background()) // Updated
	assert.NotEmpty(t, tc.TraceID, "TraceID should not be empty")
	assert.NotEmpty(t, tc.SpanID, "SpanID should not be empty")
	assert.NotNil(t, tc.Baggage, "Baggage should be initialized")
	assert.NotNil(t, tc.Context(), "stdCtx should be initialized")
}

func TestTraceContext_NewSpan(t *testing.T) {
	tc := NewTraceContext(go_std_context.Background()) // Updated
	initialSpanID := tc.SpanID
	newSpanID := tc.NewSpan()
	assert.NotEmpty(t, newSpanID, "New SpanID should not be empty")
	assert.NotEqual(t, initialSpanID, newSpanID, "New SpanID should be different from initial")
	assert.Equal(t, newSpanID, tc.SpanID, "TraceContext SpanID should be updated to new SpanID")
}

func TestBuildDomainContext(t *testing.T) {
	merchantCfg := MerchantConfig{
		ID:            "merchant123",
		PolicyVersion: 1,
		UserPrefs:     map[string]string{"theme": "dark"},
		DefaultTimeout: TimeoutConfig{OverallBudgetMs: 5000, ProviderTimeoutMs: 2000},
		DefaultRetryPolicy: RetryPolicy{MaxAttempts: 3},
		FeatureFlags:  map[string]bool{"newFeature": true},
		ProviderAPIKeys: map[string]string{"stripe": "sk_test_123"},
	}
	extReq := &orchestratorexternalv1.ExternalRequest{
		MerchantId: "merchant123",
		Amount:     1000,
		Currency:   "USD",
	}

	domainCtx, err := BuildDomainContext(extReq, merchantCfg)
	require.NoError(t, err)

	assert.Equal(t, "merchant123", domainCtx.MerchantID)
	assert.Equal(t, 1, domainCtx.MerchantPolicyVersion)
	assert.Equal(t, "dark", domainCtx.UserPreferences["theme"])
	assert.Equal(t, int64(5000), domainCtx.TimeoutConfig.OverallBudgetMs)
	assert.Equal(t, 3, domainCtx.RetryPolicy.MaxAttempts)
	assert.True(t, domainCtx.GetFeatureFlag("newFeature"))
	assert.False(t, domainCtx.GetFeatureFlag("missingFeature"))
	assert.Equal(t, "sk_test_123", domainCtx.GetProviderAPIKey("stripe"))
	assert.Empty(t, domainCtx.GetProviderAPIKey("unknown_provider"))
	assert.Equal(t, merchantCfg, domainCtx.ActiveMerchantConfig)
}

func TestDeriveStepContext(t *testing.T) {
	traceCtx := NewTraceContext(go_std_context.Background()) // Updated
	domainCtx := DomainContext{
		MerchantID: "merchant123",
		RetryPolicy: RetryPolicy{MaxAttempts: 2},
		TimeoutConfig: TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: MerchantConfig{
			ProviderAPIKeys: map[string]string{"stripe": "sk_test_stripe"},
		},
	}
	overallTimeout := domainCtx.TimeoutConfig.OverallBudgetMs
	startTimeForDomain := time.Now().Add(-1 * time.Second) // Pretend domain context was created 1 sec ago
	attemptNumber := 1

	stepCtx := DeriveStepContext(traceCtx, domainCtx, "stripe", overallTimeout, startTimeForDomain, attemptNumber)

	assert.Equal(t, traceCtx.TraceID, stepCtx.TraceID)
	assert.NotEmpty(t, stepCtx.SpanID)
	assert.NotEqual(t, traceCtx.SpanID, stepCtx.SpanID, "Step context should have a new span ID")
	assert.WithinDuration(t, time.Now(), stepCtx.StartTime, 100*time.Millisecond)
	assert.True(t, stepCtx.RemainingBudgetMs < overallTimeout, "Remaining budget should be less than overall")
	assert.True(t, stepCtx.RemainingBudgetMs > 0, "Remaining budget should be positive")
	assert.Equal(t, 2, stepCtx.StepRetryPolicy.MaxAttempts)
	assert.Equal(t, "sk_test_stripe", stepCtx.ProviderCredentials.APIKey)
	assert.Equal(t, attemptNumber, stepCtx.AttemptNumber)


	// Test with budget nearly exhausted
	attemptNumber = 2
	almostExhaustedStartTime := time.Now().Add(-time.Duration(overallTimeout-100) * time.Millisecond)
	stepCtxExhausted := DeriveStepContext(traceCtx, domainCtx, "stripe", overallTimeout, almostExhaustedStartTime, attemptNumber)
	assert.True(t, stepCtxExhausted.RemainingBudgetMs > 0 && stepCtxExhausted.RemainingBudgetMs <= 100, "Remaining budget should be small but positive")
	assert.Equal(t, attemptNumber, stepCtxExhausted.AttemptNumber)

	// Test with budget fully exhausted
	attemptNumber = 3
	fullyExhaustedStartTime := time.Now().Add(-time.Duration(overallTimeout+100) * time.Millisecond)
	stepCtxFullyExhausted := DeriveStepContext(traceCtx, domainCtx, "stripe", overallTimeout, fullyExhaustedStartTime, attemptNumber)
	assert.Equal(t, int64(0), stepCtxFullyExhausted.RemainingBudgetMs, "Remaining budget should be zero if overdue")
	assert.Equal(t, attemptNumber, stepCtxFullyExhausted.AttemptNumber)
}

func TestContextBuilder_BuildContexts(t *testing.T) {
	repo := NewInMemoryMerchantConfigRepository()
	cfg1 := MerchantConfig{
		ID:            "merchant1",
		PolicyVersion: 1,
		DefaultTimeout: TimeoutConfig{OverallBudgetMs: 3000},
		DefaultRetryPolicy: RetryPolicy{MaxAttempts: 1},
	}
	repo.AddConfig(cfg1)

	builder := NewContextBuilder(repo)

	t.Run("ValidRequest", func(t *testing.T) {
		extReq := &orchestratorexternalv1.ExternalRequest{
			RequestId:  "req1",
			MerchantId: "merchant1",
			Amount:     100,
			Currency:   "USD",
		}
		// Pass nil for parent context to NewTraceContext, it will use context.Background()
		traceCtx, domainCtx, err := builder.BuildContexts(extReq)
		require.NoError(t, err)

		assert.NotEmpty(t, traceCtx.TraceID)
		assert.NotEmpty(t, traceCtx.SpanID)
		assert.NotNil(t, traceCtx.Context()) // Check that stdCtx is initialized
		assert.Equal(t, "merchant1", domainCtx.MerchantID)
		assert.Equal(t, 1, domainCtx.MerchantPolicyVersion)
		assert.Equal(t, cfg1.DefaultTimeout, domainCtx.TimeoutConfig)
		assert.Equal(t, cfg1.DefaultRetryPolicy, domainCtx.RetryPolicy)
	})

	t.Run("NilRequest", func(t *testing.T) {
		_, _, err := builder.BuildContexts(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "external request cannot be nil")
	})

	t.Run("MerchantNotFound", func(t *testing.T) {
		extReq := &orchestratorexternalv1.ExternalRequest{
			RequestId:  "req2",
			MerchantId: "unknown_merchant",
		}
		_, _, err := builder.BuildContexts(extReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get merchant config")
		assert.Contains(t, err.Error(), "merchant config not found for ID: unknown_merchant")
	})
}

// This test was for the old InMemoryMerchantConfigRepository.
// It's adapted slightly for the new one which returns (*MerchantConfig, error) from GetConfig.
func TestInMemoryMerchantConfigRepository(t *testing.T) {
	repo := NewInMemoryMerchantConfigRepository()
	cfg1 := MerchantConfig{ID: "m1", DefaultProvider: "stripe"}
	cfg2 := MerchantConfig{ID: "m2", DefaultProvider: "adyen"}

	repo.AddConfig(cfg1)
	repo.AddConfig(cfg2)

	res1, err1 := repo.Get("m1") // Changed GetConfig to Get
	require.NoError(t, err1)
	// No NotNil check needed as Get returns MerchantConfig value, not pointer
	assert.Equal(t, cfg1, res1)

	res2, err2 := repo.Get("m2") // Changed GetConfig to Get
	require.NoError(t, err2)
	assert.Equal(t, cfg2, res2)

	_, err3 := repo.Get("m3") // Changed GetConfig to Get
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "merchant config not found for ID: m3")
}

// Ensure testify and other necessary imports are available.
// May need to run `go get github.com/stretchr/testify/assert github.com/stretchr/testify/require github.com/google/uuid`
// in the subtask environment if not already present in go.mod
