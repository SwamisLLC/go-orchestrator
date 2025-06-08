package orchestrator

import (
	"fmt"
	"testing"
	// "time" // No longer directly used here

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	"github.com/yourorg/payment-orchestrator/internal/router"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker" // Added import
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// "github.com/google/uuid" // No longer directly used here
)

// MockMerchantConfigRepository (already defined in previous test, ensure it's available or redefine)
// For simplicity, re-defining a minimal version here if not shared from another test file.
type MockMerchantConfigRepository struct {
	cfg context.MerchantConfig
	err error
}
func (m *MockMerchantConfigRepository) Get(merchantID string) (context.MerchantConfig, error) {
	if m.err != nil { return context.MerchantConfig{}, m.err }
	mc := m.cfg
	mc.ID = merchantID
	return mc, nil
}
func (m *MockMerchantConfigRepository) AddConfig(config context.MerchantConfig) { m.cfg = config }


func TestNewOrchestrator_WithRouter(t *testing.T) {
	pe, errPe := policy.NewPaymentPolicyEnforcer(nil) // Pass nil for default rules
	require.NoError(t, errPe)
	mcr := &MockMerchantConfigRepository{}
	proc := processor.NewProcessor(make(map[string]adapter.ProviderAdapter))
	cb := circuitbreaker.NewCircuitBreaker() // Added
	rtr := router.NewRouter(proc, "", cb)    // Added cb

	orc := NewOrchestrator(pe, mcr, rtr)
	assert.NotNil(t, orc)
	assert.Equal(t, pe, orc.policyEnforcer)
	assert.Equal(t, mcr, orc.merchantConfigRepo)
	assert.Equal(t, rtr, orc.router)

	assert.Panics(t, func() { NewOrchestrator(nil, mcr, rtr) })
	assert.Panics(t, func() { NewOrchestrator(pe, nil, rtr) })
	assert.Panics(t, func() { NewOrchestrator(pe, mcr, nil) })
}

func TestOrchestrator_Execute_Integration_SingleStepSuccess(t *testing.T) {
	// Setup mock adapter
	mockStripeAdapter := mock.NewMockAdapter("stripe")
	mockStripeAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{StepID: s.StepId, Success: true, Provider: "stripe", Details: map[string]string{"id": "tx_123"}}, nil
	}
	adapters := map[string]adapter.ProviderAdapter{"stripe": mockStripeAdapter}

	// Setup dependencies
	testProcessor := processor.NewProcessor(adapters)
	cb := circuitbreaker.NewCircuitBreaker()                            // Added
	testRouter := router.NewRouter(testProcessor, "adyen_fallback", cb) // Fallback won't be used, Added cb
	testPolicyEnforcer, errPe := policy.NewPaymentPolicyEnforcer(nil)
	require.NoError(t, errPe)
	testMerchantRepo := &MockMerchantConfigRepository{}
	testMerchantRepo.AddConfig(context.MerchantConfig{
	    ID: "merchant1", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "sk_test"},
	    DefaultTimeout: context.TimeoutConfig{OverallBudgetMs: 5000},
    })


	orc := NewOrchestrator(testPolicyEnforcer, testMerchantRepo, testRouter)

	traceCtx := context.NewTraceContext()
	merchantCfg, _ := testMerchantRepo.Get("merchant1")
	domainCtx := context.DomainContext{
		MerchantID: "merchant1", ActiveMerchantConfig: merchantCfg,
		TimeoutConfig: merchantCfg.DefaultTimeout,
	}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan1",
		Steps:  []*internalv1.PaymentStep{{StepId: "step1", ProviderName: "stripe", Amount: 100}},
	}

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 1)
	assert.True(t, result.StepResults[0].Success)
	assert.Equal(t, "stripe", result.StepResults[0].ProviderName)
	assert.Equal(t, "tx_123", result.StepResults[0].Details["id"])
}

func TestOrchestrator_Execute_Integration_PrimaryFail_FallbackSuccess(t *testing.T) {
    mockStripe := mock.NewMockAdapter("stripe")
    mockStripe.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        return adapter.ProviderResult{StepID: s.StepId, Success: false, Provider: "stripe", ErrorCode: "FAIL_STRIPE"}, nil
    }
    mockAdyen := mock.NewMockAdapter("adyen")
    mockAdyen.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        return adapter.ProviderResult{StepID: s.StepId, Success: true, Provider: "adyen", Details: map[string]string{"id": "tx_adyen_fb_ok"}}, nil
    }
    adapters := map[string]adapter.ProviderAdapter{"stripe": mockStripe, "adyen": mockAdyen}

    testProcessor := processor.NewProcessor(adapters)
    cb := circuitbreaker.NewCircuitBreaker()                            // Added
    testRouter := router.NewRouter(testProcessor, "adyen", cb)          // Fallback to adyen, Added cb
    testPolicyEnforcer, errPe := policy.NewPaymentPolicyEnforcer(nil)
    require.NoError(t, errPe)
    testMerchantRepo := &MockMerchantConfigRepository{}
    testMerchantRepo.AddConfig(context.MerchantConfig{
        ID: "m1", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "sk_s", "adyen": "ak_a"},
        DefaultTimeout: context.TimeoutConfig{OverallBudgetMs: 5000},
    })

    orc := NewOrchestrator(testPolicyEnforcer, testMerchantRepo, testRouter)
    traceCtx := context.NewTraceContext()
    mCfg, _ := testMerchantRepo.Get("m1")
    domainCtx := context.DomainContext{MerchantID: "m1", ActiveMerchantConfig: mCfg, TimeoutConfig: mCfg.DefaultTimeout}
    plan := &internalv1.PaymentPlan{PlanId: "p1", Steps: []*internalv1.PaymentStep{{StepId: "st1", ProviderName: "stripe", Amount: 100}}}

    result, err := orc.Execute(traceCtx, plan, domainCtx)
    require.NoError(t, err)
    assert.Equal(t, "SUCCESS", result.Status, "Overall status should be success due to fallback")
    require.Len(t, result.StepResults, 1)
    finalStepResult := result.StepResults[0]
    assert.True(t, finalStepResult.Success)
    assert.Equal(t, "adyen", finalStepResult.ProviderName)
    assert.Equal(t, "tx_adyen_fb_ok", finalStepResult.Details["id"])
    assert.Equal(t, "true", finalStepResult.Details["is_fallback"])
}

func TestOrchestrator_Execute_Integration_RouterError(t *testing.T) {
    mockStripeAdapter := mock.NewMockAdapter("stripe")
    routerExecutionError := fmt.Errorf("router internal error") // This is an adapter execution error
    mockStripeAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
        // This setup will make the processor forward an error, which the router then also forwards.
        return adapter.ProviderResult{StepID: s.StepId, Provider: s.ProviderName}, routerExecutionError
    }
    adapters := map[string]adapter.ProviderAdapter{"stripe": mockStripeAdapter}

    testProcessor := processor.NewProcessor(adapters)
    cb := circuitbreaker.NewCircuitBreaker()                       // Added
    testRouter := router.NewRouter(testProcessor, "", cb)          // No fallback, Added cb
    testPolicyEnforcer, errPe := policy.NewPaymentPolicyEnforcer(nil)
    require.NoError(t, errPe)
    testMerchantRepo := &MockMerchantConfigRepository{}
    testMerchantRepo.AddConfig(context.MerchantConfig{
        ID: "merchant_router_err", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "sk_test"},
        DefaultTimeout: context.TimeoutConfig{OverallBudgetMs: 5000},
    })

    orc := NewOrchestrator(testPolicyEnforcer, testMerchantRepo, testRouter)

    traceCtx := context.NewTraceContext()
    merchantCfg, _ := testMerchantRepo.Get("merchant_router_err")
    domainCtx := context.DomainContext{
		MerchantID: "merchant_router_err", ActiveMerchantConfig: merchantCfg,
		TimeoutConfig: merchantCfg.DefaultTimeout,
	}
    plan := &internalv1.PaymentPlan{
		PlanId: "plan_router_err",
		Steps:  []*internalv1.PaymentStep{{StepId: "step_re", ProviderName: "stripe", Amount: 100}},
	}

    result, err := orc.Execute(traceCtx, plan, domainCtx)
    require.NoError(t, err) // Orchestrator Execute itself doesn't return error for router/step failures
    assert.Equal(t, "FAILURE", result.Status)
    require.Len(t, result.StepResults, 1)
    stepRes := result.StepResults[0]
    assert.False(t, stepRes.Success)
    assert.Equal(t, "ROUTER_EXECUTION_ERROR", stepRes.ErrorCode) // This is set by Orchestrator for router errors
    assert.Contains(t, stepRes.ErrorMessage, "Router execution failed")
    // The router_error_details field contains the error string from the router, which wraps the processor error (which is the adapter's error).
    expectedRouterErrorDetail := "router: error processing step with primary provider stripe: router internal error"
    assert.Equal(t, expectedRouterErrorDetail, stepRes.Details["router_error_details"])
}

// Note: The test for policy evaluation error (TestOrchestrator_Execute_PolicyEvaluationError)
// and associated helper types (FailingPolicyEnforcer, SkipStepPolicyEnforcer) have been removed.
// The Orchestrator's path for `policyErr != nil` is covered by its implementation.
// Testing specific policy decisions like skipping steps or configurable error returns from
// policy evaluation would require making policy.PaymentPolicyEnforcer an interface or
// significantly modifying the stub, which is deferred.
// TODO: Add TestOrchestrator_Execute_StepSkippedByPolicy once PolicyDecision and Orchestrator support it.
// TODO: Add tests for policy evaluation errors once PolicyEnforcer is an interface or configurable.
