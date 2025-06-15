package router_test

import (
	"errors"
	"fmt"
	"testing"
	"time"
	// go_std_context "context" // Standard Go context for NewTraceContext - Removed

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/router"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// MockProcessor is a mock implementation of the router.ProcessorInterface using testify/mock.
type MockProcessor struct {
	mock.Mock
}

func (m *MockProcessor) ProcessSingleStep(
	traceCtx context.TraceContext, // Added traceCtx
	stepCtx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
	selectedAdapter adapter.ProviderAdapter,
) (*orchestratorinternalv1.StepResult, error) {
	args := m.Called(traceCtx, stepCtx, step, selectedAdapter) // Added traceCtx
	res, _ := args.Get(0).(*orchestratorinternalv1.StepResult)
	return res, args.Error(1)
}

// Helper functions for creating step results
func successfulStepResult(stepID, providerName string) *orchestratorinternalv1.StepResult {
	return &orchestratorinternalv1.StepResult{StepId: stepID, Success: true, ProviderName: providerName}
}

func failedStepResult(stepID, providerName, errorCode string) *orchestratorinternalv1.StepResult {
	return &orchestratorinternalv1.StepResult{StepId: stepID, Success: false, ProviderName: providerName, ErrorCode: errorCode}
}


func TestNewRouter(t *testing.T) {
	mockProcessor := new(MockProcessor)
	mockAdapters := map[string]adapter.ProviderAdapter{"primary": &adaptermock.MockAdapter{}}
	cfg := router.RouterConfig{PrimaryProviderName: "primary"}
	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{})

	t.Run("Valid construction", func(t *testing.T) {
		r, err := router.NewRouter(mockProcessor, mockAdapters, cfg, cb)
		assert.NoError(t, err)
		assert.NotNil(t, r)
	})
	t.Run("Nil processor", func(t *testing.T) {
		_, err := router.NewRouter(nil, mockAdapters, cfg, cb)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "processor cannot be nil")
	})
	t.Run("Nil adapters map", func(t *testing.T) {
		_, err := router.NewRouter(mockProcessor, nil, cfg, cb)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "adapters map cannot be nil")
	})
	t.Run("Nil circuit breaker", func(t *testing.T) {
		_, err := router.NewRouter(mockProcessor, mockAdapters, cfg, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circuit breaker cannot be nil")
	})
	t.Run("Empty primary provider name", func(t *testing.T) {
		emptyCfg := router.RouterConfig{PrimaryProviderName: ""}
		_, err := router.NewRouter(mockProcessor, mockAdapters, emptyCfg, cb)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary provider name in config cannot be empty")
	})
}


func TestRouter_ExecuteStep(t *testing.T) {
	mockPrimaryAdapter := &adaptermock.MockAdapter{Name: "primary"}
	mockFallbackAdapter := &adaptermock.MockAdapter{Name: "fallback"}

	mockAdapters := map[string]adapter.ProviderAdapter{
		"primary":  mockPrimaryAdapter,
		"fallback": mockFallbackAdapter,
	}

	defaultRouterConfig := router.RouterConfig{
		PrimaryProviderName:  "primary",
		FallbackProviderName: "fallback",
	}

	dummyTraceCtx := context.NewTraceContext(nil) // Common TraceContext for tests
	defaultStepCtx := context.StepExecutionContext{
		TraceID:           dummyTraceCtx.GetTraceID(),
		SpanID:            dummyTraceCtx.GetSpanID(), // This will be the initial span
		StartTime:         time.Now(),
		RemainingBudgetMs: 1000,       // Sufficient for defaultMinRequiredBudgetMs = 50
	}
	dummyPaymentStep := &orchestratorinternalv1.PaymentStep{StepId: "step-123", ProviderName: "any", Amount: 100, Currency: "USD"}

	cbCfg := circuitbreaker.Config{FailureThreshold: 2, ResetTimeout: 1 * time.Minute}


	t.Run("Primary provider succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		expectedResult := successfulStepResult(dummyPaymentStep.StepId, "primary")
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(expectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		mockProcessor.AssertExpectations(t)
		state, _ := cb.GetProviderStatus("primary")
		assert.Equal(t, circuitbreaker.StateClosed, state, "CB for primary should be closed after success")
	})

	t.Run("Primary fails, Fallback succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		primaryFailResult := failedStepResult(dummyPaymentStep.StepId, "primary", "PRIMARY_FAIL")
		fallbackExpectedResult := successfulStepResult(dummyPaymentStep.StepId, "fallback")

		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryFailResult, nil).Once()
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackExpectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.NoError(t, err)
		assert.Equal(t, fallbackExpectedResult, result)
		mockProcessor.AssertExpectations(t)
		statePrimary, _ := cb.GetProviderStatus("primary")
		assert.Equal(t, circuitbreaker.StateClosed, statePrimary, "CB for primary should be closed (1 failure < threshold 2)")
		stateFallback, _ := cb.GetProviderStatus("fallback")
		assert.Equal(t, circuitbreaker.StateClosed, stateFallback, "CB for fallback should be closed after success")
	})

	t.Run("Primary fails (with error), Fallback succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		primaryStepResult := failedStepResult(dummyPaymentStep.StepId, "primary", "PRIMARY_ERROR")
		primaryError := errors.New("primary provider error")
		fallbackExpectedResult := successfulStepResult(dummyPaymentStep.StepId, "fallback")

		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryStepResult, primaryError).Once()
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackExpectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx
		assert.NoError(t, err) // Error from primary is handled, fallback result is returned
		assert.Equal(t, fallbackExpectedResult, result)
		mockProcessor.AssertExpectations(t)
	})


	t.Run("Primary fails, Fallback fails", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		primaryFailResult := failedStepResult(dummyPaymentStep.StepId, "primary", "PRIMARY_FAIL")
		fallbackFailResult := failedStepResult(dummyPaymentStep.StepId, "fallback", "FALLBACK_FAIL")
		fallbackError := errors.New("fallback provider error")

		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryFailResult, nil).Once()
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackFailResult, fallbackError).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Error(t, err)
		assert.Equal(t, fallbackError, err)
		assert.Equal(t, fallbackFailResult, result)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Primary provider adapter not found", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		badConfig := router.RouterConfig{PrimaryProviderName: "nonexistent", FallbackProviderName: "fallback"}
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, badConfig, cb)
		require.NoError(t, err)


		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary provider adapter 'nonexistent' not found")
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "ROUTER_CONFIGURATION_ERROR", result.GetErrorCode())
		assert.Equal(t, "nonexistent", result.GetProviderName())
		mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Added mock.Anything for traceCtx
	})

	t.Run("Fallback provider adapter not found (after primary fails)", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg)
		badFallbackConfig := router.RouterConfig{PrimaryProviderName: "primary", FallbackProviderName: "nonexistent"}
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, badFallbackConfig, cb)
		require.NoError(t, err)

		primaryFailResult := failedStepResult(dummyPaymentStep.StepId, "primary", "PRIMARY_FAIL")
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryFailResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Error(t, err) // Error is now expected from tryFallback when adapter not found
		assert.Contains(t, err.Error(), "fallback provider adapter 'nonexistent' not found")
		// The result will be the augmented primaryFailResult
		assert.Equal(t, primaryFailResult.StepId, result.StepId)
		assert.False(t, result.Success)
		assert.Equal(t, "ROUTER_CONFIGURATION_ERROR", result.GetErrorCode())
		assert.Contains(t, result.GetErrorMessage(), "Fallback Attempt Failed: fallback provider adapter 'nonexistent' not found")
		mockProcessor.AssertExpectations(t)
	})

	t.Run("TestExecuteStep_PrimaryCircuitOpen_FallbackSucceeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 1*time.Minute})
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		cb.RecordFailure("primary")
		require.False(t, cb.AllowRequest("primary"))

		fallbackExpectedResult := successfulStepResult(dummyPaymentStep.StepId, "fallback")
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackExpectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.NoError(t, err)
		assert.True(t, result.GetSuccess())
		assert.Equal(t, "fallback", result.GetProviderName())
		mockProcessor.AssertExpectations(t)
		stateFallback, _ := cb.GetProviderStatus("fallback")
		assert.Equal(t, circuitbreaker.StateClosed, stateFallback)
	})

	t.Run("TestExecuteStep_PrimaryFails_FallbackCircuitOpen", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 1*time.Minute})
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		cb.RecordFailure("fallback")
		require.False(t, cb.AllowRequest("fallback"))

		primaryFailResult := failedStepResult(dummyPaymentStep.StepId, "primary", "PRIMARY_FAIL_CB_TEST")
		mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.TraceContext"), defaultStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryFailResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Nil(t, err) // Error from primary is handled, result reflects primary failure + fallback info
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "primary", result.GetProviderName())
		assert.Equal(t, "PRIMARY_FAIL_CB_TEST", result.GetErrorCode())
		assert.Contains(t, result.GetErrorMessage(), "Fallback Circuit Open: [circuit open for fallback provider fallback]")
		mockProcessor.AssertExpectations(t)
	})

	t.Run("TestExecuteStep_BothCircuitsOpen", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 1*time.Minute})
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		cb.RecordFailure("primary")
		cb.RecordFailure("fallback")
		require.False(t, cb.AllowRequest("primary"))
		require.False(t, cb.AllowRequest("fallback"))

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, defaultStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Nil(t, err) // Router returns result, not error, for this condition
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "primary", result.GetProviderName())
		assert.Equal(t, "ALL_PROVIDERS_UNAVAILABLE", result.GetErrorCode())
		assert.Contains(t, result.GetErrorMessage(), "Primary Error: [circuit open for primary provider primary]; Fallback Circuit Open: [circuit open for fallback provider fallback]")
		mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Added mock.Anything for traceCtx
	})

	// --- SLA Budget Tests ---
	const lowBudgetMs int64 = 30 // Less than defaultMinRequiredBudgetMs (50 from router.go)

	t.Run("TestExecuteStep_BothPrimaryAndFallbackSLABudgetExceeded", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		cb := circuitbreaker.NewCircuitBreaker(cbCfg) // Use default cbCfg
		routerInstance, err := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig, cb)
		require.NoError(t, err)

		slaStepCtx := context.StepExecutionContext{RemainingBudgetMs: lowBudgetMs, TraceID: dummyTraceCtx.GetTraceID()} // Use TraceID

		result, err := routerInstance.ExecuteStep(dummyTraceCtx, slaStepCtx, dummyPaymentStep) // Pass dummyTraceCtx

		assert.Nil(t, err) // Router returns result, not error
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "ALL_PROVIDERS_UNAVAILABLE", result.GetErrorCode())
		assert.Equal(t, "primary", result.GetProviderName()) // Primary was attempted first (conceptually)
		assert.Contains(t, result.GetErrorMessage(), "insufficient SLA budget ("+fmt.Sprintf("%d",lowBudgetMs)+"ms) for primary provider primary")
		assert.Contains(t, result.GetErrorMessage(), "Fallback Skipped (SLA): insufficient SLA budget ("+fmt.Sprintf("%d",lowBudgetMs)+"ms) for fallback provider fallback")
		mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Added mock.Anything for traceCtx
	})
}
