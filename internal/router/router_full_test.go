package router_test

import (
	// "fmt" // No longer used after removing/refactoring some test assertions
	"testing"
	"time"
	go_std_context "context" // Re-add for context.Background()

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	custom_context "github.com/yourorg/payment-orchestrator/internal/context" // aliased to custom_context
	"github.com/yourorg/payment-orchestrator/internal/router"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	protos "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// --- MockProcessor (local definition for this test package) ---
type FullTestMockProcessor struct {
	mock.Mock
}

func (m *FullTestMockProcessor) ProcessSingleStep(traceCtx custom_context.TraceContext, stepCtx custom_context.StepExecutionContext, step *protos.PaymentStep, selectedAdapter adapter.ProviderAdapter) (*protos.StepResult, error) {
	args := m.Called(traceCtx, stepCtx, step, selectedAdapter) // Added traceCtx
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	res, _ := args.Get(0).(*protos.StepResult)
	return res, args.Error(1)
}

// --- Test Helper Functions (local definition) ---
func fullTestSuccessfulStepResult(stepID, providerName string) *protos.StepResult {
	return &protos.StepResult{StepId: stepID, Success: true, ProviderName: providerName, Details: map[string]string{"transaction_id": "txn_full_success_" + providerName}}
}

func fullTestFailedStepResult(stepID, providerName string, errorCode string) *protos.StepResult {
	return &protos.StepResult{StepId: stepID, Success: false, ProviderName: providerName, ErrorCode: errorCode, ErrorMessage: "Full Test Failure: " + errorCode, Details: map[string]string{"transaction_id": "txn_full_fail_" + providerName}}
}

// --- Test Suite Setup Helper ---
func setupRouterForFullFeatureTests(t *testing.T, cbConfig circuitbreaker.Config, routerCfg router.RouterConfig, adapters map[string]adapter.ProviderAdapter) (*router.Router, *FullTestMockProcessor, *circuitbreaker.CircuitBreaker) {
	mockProc := new(FullTestMockProcessor)
	cb := circuitbreaker.NewCircuitBreaker(cbConfig)
	r, err := router.NewRouter(mockProc, adapters, routerCfg, cb)
	require.NoError(t, err, "NewRouter should not error during setup")
	require.NotNil(t, r, "Router instance should not be nil")
	return r, mockProc, cb
}

// --- Full-Feature Test Cases ---

func TestRouter_Full_CircuitBreaker_Opens_Then_HalfOpen_Then_Closes(t *testing.T) {
	cbCfg := circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 100 * time.Millisecond}
	routerCfg := router.RouterConfig{PrimaryProviderName: "p1", FallbackProviderName: "p2"}
	adapters := map[string]adapter.ProviderAdapter{
		"p1": &adaptermock.MockAdapter{Name: "p1"},
		"p2": &adaptermock.MockAdapter{Name: "p2"},
	}
	r, mockProc, cb := setupRouterForFullFeatureTests(t, cbCfg, routerCfg, adapters)
	step := &protos.PaymentStep{StepId: "s1", Amount: 100, Currency: "USD"}
	traceCtx := custom_context.NewTraceContext(go_std_context.Background()) // Create TraceContext
	ctx := custom_context.StepExecutionContext{RemainingBudgetMs: 1000, TraceID: traceCtx.GetTraceID()} // Use TraceID from traceCtx

	// Phase 1: Primary fails, CB for p1 opens. Fallback p2 is used and succeeds.
	mockProc.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step, adapters["p1"]).Return(fullTestFailedStepResult(step.StepId, "p1", "P1_FAIL_OPENS_CB"), nil).Once()
	mockProc.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step, adapters["p2"]).Return(fullTestSuccessfulStepResult(step.StepId, "p2"), nil).Once()

	result, err := r.ExecuteStep(traceCtx, ctx, step) // Pass traceCtx
	require.NoError(t, err)
	assert.True(t, result.Success, "Fallback should succeed")
	assert.Equal(t, "p2", result.ProviderName)
	mockProc.AssertExpectations(t)
	statusP1, _ := cb.GetProviderStatus("p1")
	assert.Equal(t, circuitbreaker.StateOpen, statusP1, "CB for p1 should be Open")

	// Reset mock for the next phase
	mockProc.ExpectedCalls = nil
	mockProc.Calls = nil

	// Phase 2: Attempt step again. Primary p1's circuit is Open. Router skips p1, tries p2. p2 succeeds.
	mockProc.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step, adapters["p2"]).Return(fullTestSuccessfulStepResult(step.StepId, "p2"), nil).Once()

	result, err = r.ExecuteStep(traceCtx, ctx, step) // Pass traceCtx
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "p2", result.ProviderName)
	mockProc.AssertExpectations(t)
	statusP1, _ = cb.GetProviderStatus("p1")
	assert.Equal(t, circuitbreaker.StateOpen, statusP1, "CB for p1 should still be Open")
	statusP2, _ := cb.GetProviderStatus("p2")
	assert.Equal(t, circuitbreaker.StateClosed, statusP2, "CB for p2 should be Closed")


	mockProc.ExpectedCalls = nil
	mockProc.Calls = nil

	// Phase 3: Wait for ResetTimeout. p1's circuit becomes HalfOpen. Router tries p1. p1 succeeds. Circuit for p1 closes.
	time.Sleep(cbCfg.ResetTimeout + 20*time.Millisecond)
	mockProc.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step, adapters["p1"]).Return(fullTestSuccessfulStepResult(step.StepId, "p1"), nil).Once()

	result, err = r.ExecuteStep(traceCtx, ctx, step) // Pass traceCtx
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "p1", result.ProviderName)
	mockProc.AssertExpectations(t)
	statusP1, _ = cb.GetProviderStatus("p1")
	assert.Equal(t, circuitbreaker.StateClosed, statusP1, "CB for p1 should be Closed after success in HalfOpen")
}

func TestRouter_Full_BothPrimaryAndFallbackSLABudgetExceeded(t *testing.T) {
    cbCfg := circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 1 * time.Minute} // CB not expected to trip
    routerCfg := router.RouterConfig{PrimaryProviderName: "p1", FallbackProviderName: "p2"}
    adapters := map[string]adapter.ProviderAdapter{
        "p1": &adaptermock.MockAdapter{Name: "p1"},
        "p2": &adaptermock.MockAdapter{Name: "p2"},
    }
    r, mockProc, _ := setupRouterForFullFeatureTests(t, cbCfg, routerCfg, adapters) // cb instance not directly used for assertions here
    step := &protos.PaymentStep{StepId: "s1-sla", Amount: 100, Currency: "USD"}
	 traceCtx := custom_context.NewTraceContext(go_std_context.Background())

    // Budget is too low for either primary or fallback (defaultMinRequiredBudgetMs is 50ms in router.go)
    ctxLowBudget := custom_context.StepExecutionContext{RemainingBudgetMs: 40, TraceID: traceCtx.GetTraceID()}

    result, err := r.ExecuteStep(traceCtx, ctxLowBudget, step) // Pass traceCtx

    require.NoError(t, err) // Router itself does not error for this routing decision
    assert.False(t, result.Success)
    assert.Equal(t, "p1", result.ProviderName, "ProviderName should be primary as it was the first evaluated for SLA")
    assert.Equal(t, "ALL_PROVIDERS_UNAVAILABLE", result.ErrorCode, "ErrorCode should indicate no providers could be tried")
    assert.Contains(t, result.ErrorMessage, "insufficient SLA budget (40ms) for primary provider p1")
    assert.Contains(t, result.ErrorMessage, "Fallback Skipped (SLA): insufficient SLA budget (40ms) for fallback provider p2")

    mockProc.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Added one more mock.Anything for traceCtx
}

func TestRouter_Full_PrimaryCircuitOpen_FallbackSLASkipped(t *testing.T) {
    cbCfg := circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 1 * time.Minute}
    routerCfg := router.RouterConfig{PrimaryProviderName: "p1", FallbackProviderName: "p2"}
    adapters := map[string]adapter.ProviderAdapter{
        "p1": &adaptermock.MockAdapter{Name: "p1"},
        "p2": &adaptermock.MockAdapter{Name: "p2"},
    }
    r, mockProc, cb := setupRouterForFullFeatureTests(t, cbCfg, routerCfg, adapters)
    step := &protos.PaymentStep{StepId: "s1-cb-sla", Amount:100, Currency:"USD"}
	 traceCtx := custom_context.NewTraceContext(go_std_context.Background())

    // Budget is too low for fallback attempt
    ctxLowBudget := custom_context.StepExecutionContext{RemainingBudgetMs: 40, TraceID: traceCtx.GetTraceID()}

    // Open P1 circuit
    cb.RecordFailure("p1") // This makes P1's state = Open because FailureThreshold = 1
    statusP1Initial, _ := cb.GetProviderStatus("p1")
    require.Equal(t, circuitbreaker.StateOpen, statusP1Initial, "P1 circuit should be open for test setup")

    result, err := r.ExecuteStep(traceCtx, ctxLowBudget, step) // Pass traceCtx

    require.NoError(t, err)
    assert.False(t, result.Success)
    // The result.ProviderName might be p1 (if ExecuteStep sets it before SLA/CB checks) or empty.
    // The current router.go sets ProviderName in the result even for SLA/CB failures before fallback.
    assert.Equal(t, "p1", result.ProviderName, "ProviderName should be primary from initial attempt info")
    assert.Equal(t, "ALL_PROVIDERS_UNAVAILABLE", result.ErrorCode, "ErrorCode should indicate no providers available")
	 // The message from router.ExecuteStep when primary CB is open and fallback SLA is also bad:
    assert.Contains(t, result.ErrorMessage, "circuit open for primary provider p1")
    assert.Contains(t, result.ErrorMessage, "Fallback Skipped (SLA): insufficient SLA budget (40ms) for fallback provider p2")


    mockProc.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Added one more mock.Anything
}
