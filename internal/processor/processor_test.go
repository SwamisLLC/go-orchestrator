package processor

import (
	"errors"
	"testing"
	"time"
	go_std_context "context" // Re-add for context.Background()


	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/payment-orchestrator/internal/adapter"
	customcontext "github.com/yourorg/payment-orchestrator/internal/context"
	protos "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	// "go.uber.org/zap/zaptest" // No longer using zaptest directly for TraceContext
)

// MockProviderAdapter is a mock implementation of the adapter.ProviderAdapter interface for testing.
type MockProviderAdapter struct {
	ProcessFunc func(
		traceCtx customcontext.TraceContext,
		step *protos.PaymentStep,
		stepCtx customcontext.StepExecutionContext,
	) (adapter.ProviderResult, error)
	GetNameFunc func() string
}

func (m *MockProviderAdapter) Process(
	traceCtx customcontext.TraceContext,
	step *protos.PaymentStep,
	stepCtx customcontext.StepExecutionContext,
) (adapter.ProviderResult, error) {
	if m.ProcessFunc != nil {
		return m.ProcessFunc(traceCtx, step, stepCtx)
	}
	return adapter.ProviderResult{}, errors.New("ProcessFunc not implemented in mock")
}

func (m *MockProviderAdapter) GetName() string {
	if m.GetNameFunc != nil {
		return m.GetNameFunc()
	}
	return "mock-provider"
}

func TestProcessor_ProcessSingleStep_Success(t *testing.T) {
	// logger := zaptest.NewLogger(t) // Standard log will be used by processor
	traceCtx := customcontext.NewTraceContext(go_std_context.Background()) // Use standard TraceContext constructor
	proc := NewProcessor()

	mockAdapter := &MockProviderAdapter{
		GetNameFunc: func() string {
			return "test-success-provider"
		},
		ProcessFunc: func(tc customcontext.TraceContext, s *protos.PaymentStep, sc customcontext.StepExecutionContext) (adapter.ProviderResult, error) {
			return adapter.ProviderResult{
				StepID:        s.GetStepId(),
				Success:       true,
				Provider:      "test-success-provider",
				TransactionID: "txn_123success",
				Details:       map[string]string{"original_detail": "value1"},
				LatencyMs:     50, // mock latency from provider
			}, nil
		},
	}

	step := &protos.PaymentStep{
		StepId:       "step_test_success",
		ProviderName: "test-success-provider",
		Amount:       1000,
		Currency:     "USD",
	}
	stepCtx := customcontext.StepExecutionContext{ /* APIKey: "dummy_key" */ }

	result, err := proc.ProcessSingleStep(traceCtx, stepCtx, step, mockAdapter) // Corrected order

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, step.GetStepId(), result.GetStepId())
	assert.True(t, result.GetSuccess())
	assert.Equal(t, "test-success-provider", result.GetProviderName())
	assert.Empty(t, result.GetErrorCode())
	assert.Empty(t, result.GetErrorMessage())
	assert.Equal(t, "txn_123success", result.GetDetails()["transaction_id"])
	assert.Equal(t, "value1", result.GetDetails()["original_detail"])
	assert.True(t, result.GetLatencyMs() >= 0, "Latency should be recorded")
	// As we set LatencyMs in ProviderResult, that should be used.
	assert.Equal(t, int64(50), result.GetLatencyMs())
}

func TestProcessor_ProcessSingleStep_ProviderFailure(t *testing.T) {
	// logger := zaptest.NewLogger(t)
	traceCtx := customcontext.NewTraceContext(go_std_context.Background()) // Pass nil
	proc := NewProcessor()

	mockAdapter := &MockProviderAdapter{
		GetNameFunc: func() string {
			return "test-fail-provider"
		},
		ProcessFunc: func(tc customcontext.TraceContext, s *protos.PaymentStep, sc customcontext.StepExecutionContext) (adapter.ProviderResult, error) {
			return adapter.ProviderResult{
				StepID:        s.GetStepId(),
				Success:       false,
				Provider:      "test-fail-provider",
				ErrorCode:     "INSUFFICIENT_FUNDS",
				ErrorMessage:  "The payment failed due to insufficient funds.",
				Details:       map[string]string{"reason": "poorness"},
				LatencyMs:     75,
			}, nil // No error from adapter itself, but provider declined
		},
	}

	step := &protos.PaymentStep{
		StepId:       "step_test_provider_fail",
		ProviderName: "test-fail-provider",
		Amount:       20000,
		Currency:     "USD",
	}
	stepCtx := customcontext.StepExecutionContext{}

	result, err := proc.ProcessSingleStep(traceCtx, stepCtx, step, mockAdapter) // Corrected order

	require.NoError(t, err) // The call to adapter was successful
	require.NotNil(t, result)

	assert.Equal(t, step.GetStepId(), result.GetStepId())
	assert.False(t, result.GetSuccess())
	assert.Equal(t, "test-fail-provider", result.GetProviderName())
	assert.Equal(t, "INSUFFICIENT_FUNDS", result.GetErrorCode())
	assert.Equal(t, "The payment failed due to insufficient funds.", result.GetErrorMessage())
	assert.Equal(t, "poorness", result.GetDetails()["reason"])
	assert.Equal(t, int64(75), result.GetLatencyMs())
}

func TestProcessor_ProcessSingleStep_AdapterError(t *testing.T) {
	// logger := zaptest.NewLogger(t)
	traceCtx := customcontext.NewTraceContext(go_std_context.Background()) // Pass nil
	proc := NewProcessor()

	expectedError := errors.New("adapter network communication failed")
	mockAdapter := &MockProviderAdapter{
		GetNameFunc: func() string {
			return "test-adapter-error-provider"
		},
		ProcessFunc: func(tc customcontext.TraceContext, s *protos.PaymentStep, sc customcontext.StepExecutionContext) (adapter.ProviderResult, error) {
			// Simulate some work before erroring to ensure measurable latency
			time.Sleep(5 * time.Millisecond)
			// Adapter itself returns an error
			return adapter.ProviderResult{}, expectedError
		},
	}

	step := &protos.PaymentStep{
		StepId:       "step_test_adapter_error",
		ProviderName: "test-adapter-error-provider",
	}
	stepCtx := customcontext.StepExecutionContext{}

	result, err := proc.ProcessSingleStep(traceCtx, stepCtx, step, mockAdapter) // Corrected order

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedError, err)

	assert.Equal(t, step.GetStepId(), result.GetStepId())
	assert.False(t, result.GetSuccess())
	assert.Equal(t, "test-adapter-error-provider", result.GetProviderName())
	assert.Equal(t, "ADAPTER_ERROR", result.GetErrorCode())
	assert.Equal(t, "adapter network communication failed", result.GetErrorMessage())
	assert.Nil(t, result.GetDetails()) // No details from provider if adapter failed
	assert.True(t, result.GetLatencyMs() > 0, "Latency should be recorded and positive")
}

func TestProcessor_ProcessSingleStep_LatencyCalculation(t *testing.T) {
	// logger := zaptest.NewLogger(t)
	traceCtx := customcontext.NewTraceContext(go_std_context.Background()) // Pass nil
	proc := NewProcessor()

	mockAdapter := &MockProviderAdapter{
		GetNameFunc: func() string { return "latency-provider" },
		ProcessFunc: func(tc customcontext.TraceContext, s *protos.PaymentStep, sc customcontext.StepExecutionContext) (adapter.ProviderResult, error) {
			time.Sleep(20 * time.Millisecond) // Simulate work by adapter
			return adapter.ProviderResult{
				StepID:   s.GetStepId(),
				Success:  true,
				Provider: "latency-provider",
				// No LatencyMs set here, so processor should calculate it
			}, nil
		},
	}
	step := &protos.PaymentStep{StepId: "step_latency_test"}
	stepCtx := customcontext.StepExecutionContext{}

	result, err := proc.ProcessSingleStep(traceCtx, stepCtx, step, mockAdapter) // Corrected order
	require.NoError(t, err)
	assert.True(t, result.GetLatencyMs() >= 20, "Calculated latency should be at least 20ms")
}
