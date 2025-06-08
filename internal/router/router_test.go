package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker" // Import
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Updated helper to include CircuitBreaker
func newTestRouterWithMocks(adapters map[string]adapter.ProviderAdapter, fallbackProvider string, cb *circuitbreaker.CircuitBreaker) *Router {
	if cb == nil {
		cb = circuitbreaker.NewCircuitBreaker() // Default CB if nil for simplicity in older tests
	}
	testProcessor := processor.NewProcessor(adapters)
	return NewRouter(testProcessor, fallbackProvider, cb)
}

func TestNewRouter_WithCircuitBreaker(t *testing.T) {
	mockProcessor := processor.NewProcessor(make(map[string]adapter.ProviderAdapter))
	cb := circuitbreaker.NewCircuitBreaker()
	r := NewRouter(mockProcessor, "fallback_stripe", cb)
	require.NotNil(t, r)
	assert.Equal(t, mockProcessor, r.processor)
	assert.Equal(t, "fallback_stripe", r.primaryFallbackProvider)
	assert.Equal(t, cb, r.circuitBreaker)

	assert.Panics(t, func() { NewRouter(nil, "fb", cb) })
	assert.Panics(t, func() { NewRouter(mockProcessor, "fb", nil) }) // CB is now mandatory
}

func TestRouter_ExecuteStep_NilStep(t *testing.T) {
	// Pass a real CB to NewRouter via the helper
	r := newTestRouterWithMocks(make(map[string]adapter.ProviderAdapter), "", circuitbreaker.NewCircuitBreaker())
	_, err := r.ExecuteStep(context.NewTraceContext(), nil, context.StepExecutionContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router: payment step cannot be nil")
}

func TestRouter_ExecuteStep_PrimarySuccess_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: true, Provider: "stripe", Details: map[string]string{"provider_transaction_id": "tx_primary_ok"}}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter}
	cb := circuitbreaker.NewCircuitBreaker()
	router := newTestRouterWithMocks(adapters, "adyen_fallback", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "stripe", result.ProviderName)
	assert.Equal(t, "tx_primary_ok", result.Details["provider_transaction_id"])
	assert.Equal(t, circuitbreaker.Closed, cb.GetState("stripe")) // Should be closed after success
	// Cannot directly assert consecutiveFailures as it's an internal field of circuitbreaker from another package
}

func TestRouter_ExecuteStep_PrimaryFails_FallbackSuccess_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_FAIL"}, nil
	}
	fallbackAdapter := mock.NewMockAdapter("adyen_fallback")
	fallbackAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		require.Equal(t, "adyen_fallback", s.ProviderName)
		providerDetails := make(map[string]string)
		if s.Metadata != nil { for k, v := range s.Metadata { providerDetails[k] = v } }
		providerDetails["provider_transaction_id"] = "tx_fallback_ok"
		return adapter.ProviderResult{StepID: s.StepId, Success: true, Provider: "adyen_fallback", Details: providerDetails}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter, "adyen_fallback": fallbackAdapter}
	cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute, 1) // 1 failure opens CB
	router := newTestRouterWithMocks(adapters, "adyen_fallback", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s2", ProviderName: "stripe", Amount: 200, Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "adyen_fallback", result.ProviderName)
	assert.Equal(t, "tx_fallback_ok", result.Details["provider_transaction_id"])
	assert.Equal(t, "true", result.Details["is_fallback"])
	assert.Equal(t, "stripe", result.Details["original_provider_attempted"])
	assert.Equal(t, "s2", result.Details["fallback_of_step_id"])
	assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe")) // Stripe failed, CB should open
	assert.Equal(t, circuitbreaker.Closed, cb.GetState("adyen_fallback")) // Fallback succeeded
}


func TestRouter_ExecuteStep_SlaBudgetExceeded(t *testing.T) {
    adapters := map[string]adapter.ProviderAdapter{}
    cb := circuitbreaker.NewCircuitBreaker()
    router := newTestRouterWithMocks(adapters, "", cb)

    traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_sla", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: MinRemainingBudgetMs - 1}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "TIMEOUT_BUDGET_EXCEEDED", result.ErrorCode)
	assert.Contains(t, result.ErrorMessage, "SLA budget exhausted")
}

func TestRouter_ExecuteStep_PrimaryCircuitOpen_NoFallback(t *testing.T) {
    primaryAdapter := mock.NewMockAdapter("stripe")
    adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter}
    cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute, 1)

    cb.RecordFailure("stripe")
    require.Equal(t, circuitbreaker.Open, cb.GetState("stripe"))

    router := newTestRouterWithMocks(adapters, "", cb) // No fallback

    traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_cb_open", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

    result, err := router.ExecuteStep(traceCtx, step, stepCtx)
    require.NoError(t, err)
    require.NotNil(t, result)
    assert.False(t, result.Success)
    assert.Equal(t, "CIRCUIT_OPEN", result.ErrorCode)
    assert.Equal(t, "stripe", result.ProviderName)
    assert.Contains(t, result.ErrorMessage, "Circuit open for primary provider: stripe")
}

func TestRouter_ExecuteStep_PrimaryCircuitOpen_FallbackSuccess(t *testing.T) {
    primaryAdapter := mock.NewMockAdapter("stripe")
    fallbackAdapter := mock.NewMockAdapter("adyen")
    fallbackAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: true, Provider: "adyen", Details: map[string]string{"provider_transaction_id": "tx_fb_cb_ok"}}, nil
	}
    adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter, "adyen": fallbackAdapter}
    cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute, 1)

    cb.RecordFailure("stripe")
    require.Equal(t, circuitbreaker.Open, cb.GetState("stripe"))
    require.Equal(t, circuitbreaker.Closed, cb.GetState("adyen"))

    router := newTestRouterWithMocks(adapters, "adyen", cb)

    traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_cb_fb", ProviderName: "stripe", Amount: 100, Metadata: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

    result, err := router.ExecuteStep(traceCtx, step, stepCtx)
    require.NoError(t, err)
    require.NotNil(t, result)
    assert.True(t, result.Success)
    assert.Equal(t, "adyen", result.ProviderName)
    assert.Equal(t, "tx_fb_cb_ok", result.Details["provider_transaction_id"])
    assert.Equal(t, "true", result.Details["is_fallback"])
    assert.Equal(t, circuitbreaker.Closed, cb.GetState("adyen"))
}

func TestRouter_ExecuteStep_PrimaryFails_FallbackCircuitOpen(t *testing.T) {
    primaryAdapter := mock.NewMockAdapter("stripe")
    primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_FAIL"}, nil
	}
    fallbackAdapter := mock.NewMockAdapter("adyen")

    adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter, "adyen": fallbackAdapter}
    cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute, 1)

    cb.RecordFailure("adyen")
    require.Equal(t, circuitbreaker.Open, cb.GetState("adyen"))

    router := newTestRouterWithMocks(adapters, "adyen", cb)

    traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_fb_cb_open", ProviderName: "stripe", Amount: 100, Metadata: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

    result, err := router.ExecuteStep(traceCtx, step, stepCtx)
    require.NoError(t, err)
    require.NotNil(t, result)
    assert.False(t, result.Success, "Should return primary failure as fallback CB is open")
    assert.Equal(t, "stripe", result.ProviderName)
    assert.Equal(t, "STRIPE_FAIL", result.ErrorCode)
    assert.Equal(t, "Circuit open for fallback provider: adyen", result.Details["fallback_attempt_skipped_reason"])
    assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe")) // Stripe failure recorded, CB opens
}

func TestRouter_ExecuteStep_RecordsToCircuitBreaker(t *testing.T) {
    cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute, 1)

    stripeAdapter := mock.NewMockAdapter("stripe")
    stripeAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        return adapter.ProviderResult{StepID:s.StepId, Success:true, Provider:"stripe"}, nil
    }
    router := newTestRouterWithMocks(map[string]adapter.ProviderAdapter{"stripe": stripeAdapter}, "adyen", cb)
    step := &internalv1.PaymentStep{StepId: "s_ps", ProviderName: "stripe"}
    stepCtx := context.StepExecutionContext{RemainingBudgetMs: 1000}
    _, _ = router.ExecuteStep(context.NewTraceContext(), step, stepCtx)
    assert.Equal(t, circuitbreaker.Closed, cb.GetState("stripe"))
	// Cannot assert internal cb.providers["stripe"].consecutiveFailures from outside the package

    adyenAdapter := mock.NewMockAdapter("adyen")
    adyenAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        return adapter.ProviderResult{StepID:s.StepId, Success:false, Provider:"adyen"}, nil
    }
    paypalAdapterForScen2 := mock.NewMockAdapter("paypal") // Adapter for Paypal fallback
    paypalAdapterForScen2.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        return adapter.ProviderResult{StepID:s.StepId, Success:true, Provider:"paypal"}, nil // Paypal succeeds
    }
    router = newTestRouterWithMocks(
        map[string]adapter.ProviderAdapter{
            "adyen": adyenAdapter,
            "paypal": paypalAdapterForScen2, // Ensure paypal adapter is in the map
        },
        "paypal", // Fallback provider is paypal
        cb,
    )
    step = &internalv1.PaymentStep{StepId: "s_pf", ProviderName: "adyen", Metadata: make(map[string]string)} // Adyen fails
    _, _ = router.ExecuteStep(context.NewTraceContext(), step, stepCtx)
    assert.Equal(t, circuitbreaker.Open, cb.GetState("adyen"))
    assert.Equal(t, circuitbreaker.Closed, cb.GetState("paypal"), "Paypal CB should be Closed after successful fallback in Adyen scenario")


    // Scenario 3: Primary fails (e.g. new_primary), fallback Paypal succeeds
    // Note: cb state for "paypal" is already Closed from scenario 2.
    newPrimaryAdapter := mock.NewMockAdapter("new_primary")
    newPrimaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
         return adapter.ProviderResult{StepID:s.StepId, Success:false, Provider:"new_primary"}, nil
    }
    paypalAdapter := mock.NewMockAdapter("paypal")
    paypalAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
         return adapter.ProviderResult{StepID:s.StepId, Success:true, Provider:"paypal"}, nil
    }
    router = newTestRouterWithMocks(
        map[string]adapter.ProviderAdapter{"new_primary": newPrimaryAdapter, "paypal": paypalAdapter},
        "paypal", cb,
    )
    step = &internalv1.PaymentStep{StepId: "s_fs", ProviderName: "new_primary", Metadata: make(map[string]string)}
     _, _ = router.ExecuteStep(context.NewTraceContext(), step, stepCtx)
    assert.Equal(t, circuitbreaker.Open, cb.GetState("new_primary"))
    assert.Equal(t, circuitbreaker.Closed, cb.GetState("paypal"))
}


// Old tests from previous router_test.go, ensure they are updated or covered by new tests.
// For brevity, only one is shown here as an example of needed updates.
// The rest of the tests (NoFallbackConfigured, FallbackIsSameAsPrimary, etc.) would need similar CB integration.

func TestRouter_ExecuteStep_PrimaryFails_NoFallbackConfigured_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_GENERIC_ERROR"}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter}
	cb := circuitbreaker.NewCircuitBreaker()
	router := newTestRouterWithMocks(adapters, "", cb) // No fallback provider

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s3", ProviderName: "stripe", Amount: 300}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "stripe", result.ProviderName)
	assert.Equal(t, "STRIPE_GENERIC_ERROR", result.ErrorCode)
	assert.Equal(t, circuitbreaker.Closed, cb.GetState("stripe")) // Default threshold is 5, so 1 failure doesn't open.
}

// Other tests like FallbackIsSameAsPrimary, FallbackAlsoFails, ProcessorError cases
// should also be updated to use newTestRouterWithMocks(..., circuitbreaker.NewCircuitBreaker())
// and add assertions for CB state where relevant.
// The existing tests for ProcessorError cases are particularly important as they test router's error propagation.
// TestRouter_ExecuteStep_PrimaryProcessorError_WithCB
// TestRouter_ExecuteStep_FallbackProcessorError_WithCB

func TestRouter_ExecuteStep_PrimaryFails_FallbackIsSameAsPrimary_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_FAIL_NO_RETRY_ON_SAME"}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter}
	cb := circuitbreaker.NewCircuitBreakerWithSettings(1, 1*time.Minute,1)
	router := newTestRouterWithMocks(adapters, "stripe", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_no_fb", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "stripe", result.ProviderName)
	assert.Equal(t, "STRIPE_FAIL_NO_RETRY_ON_SAME", result.ErrorCode)
	assert.Empty(t, result.Details["is_fallback"])
	assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe"))
}

func TestRouter_ExecuteStep_PrimaryFails_FallbackAlsoFails_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_FAIL_1"}, nil
	}
	fallbackAdapter := mock.NewMockAdapter("adyen_fb")
	fallbackAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		providerDetails := make(map[string]string)
		if s.Metadata != nil { for k,v := range s.Metadata { providerDetails[k]=v } }
		providerDetails["provider_transaction_id"] = "tx_fb_failed"
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "adyen_fb", ErrorCode: "ADYEN_FAIL_2", Details: providerDetails}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter, "adyen_fb": fallbackAdapter}
	cb := circuitbreaker.NewCircuitBreakerWithSettings(1,1*time.Minute,1)
	router := newTestRouterWithMocks(adapters, "adyen_fb", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s4", ProviderName: "stripe", Amount: 400, Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "adyen_fb", result.ProviderName)
	assert.Equal(t, "ADYEN_FAIL_2", result.ErrorCode)
	assert.Equal(t, "true", result.Details["is_fallback"])
	assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe"))
	assert.Equal(t, circuitbreaker.Open, cb.GetState("adyen_fb"))
}

func TestRouter_ExecuteStep_PrimaryProcessorError_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	processorError := fmt.Errorf("processor critical error")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Provider: "stripe"}, processorError
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter}
	cb := circuitbreaker.NewCircuitBreakerWithSettings(1,1*time.Minute,1)
	router := newTestRouterWithMocks(adapters, "adyen_fallback", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_proc_err", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router: error processing step with primary provider stripe")
	assert.Contains(t, err.Error(), "processor critical error")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "stripe", result.ProviderName)
	assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe")) // Failure recorded for processor error
}

func TestRouter_ExecuteStep_FallbackProcessorError_WithCB(t *testing.T) {
	primaryAdapter := mock.NewMockAdapter("stripe")
	primaryAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "STRIPE_INITIAL_FAIL"}, nil
	}
	fallbackAdapter := mock.NewMockAdapter("adyen_fb")
	fallbackProcessorError := fmt.Errorf("fallback processor critical error")
	fallbackAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Provider: "adyen_fb"}, fallbackProcessorError
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": primaryAdapter, "adyen_fb": fallbackAdapter}
	cb := circuitbreaker.NewCircuitBreakerWithSettings(1,1*time.Minute,1)
	router := newTestRouterWithMocks(adapters, "adyen_fb", cb)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s_fb_proc_err", ProviderName: "stripe", Amount: 100, Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 1000}

	result, err := router.ExecuteStep(traceCtx, step, stepCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router: error processing step with fallback provider adyen_fb")
	assert.Contains(t, err.Error(), "fallback processor critical error")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "adyen_fb", result.ProviderName)
	assert.Equal(t, circuitbreaker.Open, cb.GetState("stripe"))
	assert.Equal(t, circuitbreaker.Open, cb.GetState("adyen_fb"))
}
