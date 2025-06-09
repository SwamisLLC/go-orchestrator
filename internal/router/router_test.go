package router_test

import (
	"errors"
	// "fmt" // No longer needed
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock" // Using existing manual mock for adapters
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/router"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// MockProcessor is a mock implementation of the router.ProcessorInterface using testify/mock.
type MockProcessor struct {
	mock.Mock
}

func (m *MockProcessor) ProcessSingleStep(
	ctx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
	adapter adapter.ProviderAdapter,
) (*orchestratorinternalv1.StepResult, error) {
	args := m.Called(ctx, step, adapter)
	res, _ := args.Get(0).(*orchestratorinternalv1.StepResult)
	return res, args.Error(1)
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

	dummyStepCtx := context.StepExecutionContext{
		TraceID:           "test-trace",
		SpanID:            "test-span",
		StartTime:         time.Now(),
		RemainingBudgetMs: 10000,
	}
	dummyPaymentStep := &orchestratorinternalv1.PaymentStep{
		StepId:       "step-123",
		ProviderName: "any", // Router should override this based on its config
		Amount:       100,
		Currency:     "USD",
	}

	t.Run("Primary provider succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig)

		expectedResult := &orchestratorinternalv1.StepResult{Success: true, ProviderName: "primary", StepId: "step-123"}
		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(expectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Primary fails, Fallback succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig)

		primaryResult := &orchestratorinternalv1.StepResult{Success: false, ProviderName: "primary", StepId: "step-123"}
		fallbackExpectedResult := &orchestratorinternalv1.StepResult{Success: true, ProviderName: "fallback", StepId: "step-123"}

		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryResult, nil).Once()
		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackExpectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.NoError(t, err)
		assert.Equal(t, fallbackExpectedResult, result)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Primary fails (with error), Fallback succeeds", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig)

		primaryResult := &orchestratorinternalv1.StepResult{Success: false, ProviderName: "primary", ErrorCode: "PRIMARY_ERR", StepId: "step-123"}
		primaryError := errors.New("primary provider error")
		fallbackExpectedResult := &orchestratorinternalv1.StepResult{Success: true, ProviderName: "fallback", StepId: "step-123"}

		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryResult, primaryError).Once()
		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackExpectedResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.NoError(t, err) // Fallback succeeded, so overall error should be nil
		assert.Equal(t, fallbackExpectedResult, result)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Primary fails, Fallback fails", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, defaultRouterConfig)

		primaryResult := &orchestratorinternalv1.StepResult{Success: false, ProviderName: "primary", StepId: "step-123"}
		fallbackResult := &orchestratorinternalv1.StepResult{Success: false, ProviderName: "fallback", ErrorCode: "FALLBACK_ERR", StepId: "step-123"}
		fallbackError := errors.New("fallback provider error")

		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryResult, nil).Once()
		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockFallbackAdapter).
			Return(fallbackResult, fallbackError).Once()

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.Error(t, err)
		assert.Equal(t, fallbackError, err)
		assert.Equal(t, fallbackResult, result)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Primary provider adapter not found", func(t *testing.T) {
		mockProcessor := new(MockProcessor) // Processor won't be called
		badConfig := router.RouterConfig{PrimaryProviderName: "nonexistent", FallbackProviderName: "fallback"}
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, badConfig)

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary provider adapter 'nonexistent' not found")
		assert.NotNil(t, result)
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "ROUTER_CONFIGURATION_ERROR", result.GetErrorCode())
		assert.Equal(t, "nonexistent", result.GetProviderName()) // Shows which provider was attempted
		mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Fallback provider adapter not found (after primary fails)", func(t *testing.T) {
		mockProcessor := new(MockProcessor)
		badFallbackConfig := router.RouterConfig{PrimaryProviderName: "primary", FallbackProviderName: "nonexistent"}
		routerInstance := router.NewRouter(mockProcessor, mockAdapters, badFallbackConfig)

		primaryResult := &orchestratorinternalv1.StepResult{Success: false, ProviderName: "primary", StepId: "step-123"}
		mockProcessor.On("ProcessSingleStep", dummyStepCtx, dummyPaymentStep, mockPrimaryAdapter).
			Return(primaryResult, nil).Once()

		result, err := routerInstance.ExecuteStep(dummyStepCtx, dummyPaymentStep)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fallback provider adapter 'nonexistent' not found")
		assert.NotNil(t, result)
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "ROUTER_CONFIGURATION_ERROR", result.GetErrorCode())
		assert.Equal(t, "nonexistent", result.GetProviderName()) // Shows which provider was attempted for fallback
		mockProcessor.AssertExpectations(t) // Primary was called
	})
}
