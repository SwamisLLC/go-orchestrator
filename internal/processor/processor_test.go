package processor

import (
	"fmt"
	"testing"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/adapter/mock" // Using the mock adapter
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessor(t *testing.T) {
	registry := make(map[string]adapter.ProviderAdapter)
	registry["mock"] = mock.NewMockAdapter("mock_provider")

	p := NewProcessor(registry)
	require.NotNil(t, p)
	assert.Equal(t, registry, p.adapterRegistry)

	assert.Panics(t, func() { NewProcessor(nil) }, "Should panic if registry is nil")
}

func TestProcessor_ProcessSingleStep_Success(t *testing.T) {
	mockAdapter := mock.NewMockAdapter("stripe")
	mockAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{
			StepID:        s.StepId,
			Success:       true,
			Provider:      mockAdapter.GetName(),
			TransactionID: "tx_success_123",
			LatencyMs:     120,
			Details:       map[string]string{"stripe_specific": "value"},
		}, nil
	}

	registry := map[string]adapter.ProviderAdapter{"stripe": mockAdapter}
	processor := NewProcessor(registry)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := processor.ProcessSingleStep(traceCtx, step, stepCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "s1", result.StepId)
	assert.True(t, result.Success)
	assert.Equal(t, "stripe", result.ProviderName)
	assert.Empty(t, result.ErrorCode)
	assert.Equal(t, int64(120), result.LatencyMs)
	assert.Equal(t, "value", result.Details["stripe_specific"])
}

func TestProcessor_ProcessSingleStep_AdapterReturnsError(t *testing.T) {
	mockAdapter := mock.NewMockAdapter("adyen")
	expectedProviderError := "INSUFFICIENT_FUNDS"
	mockAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{
			StepID:       s.StepId,
			Success:      false,
			Provider:     mockAdapter.GetName(),
			ErrorCode:    expectedProviderError,
			ErrorMessage: "The transaction was declined due to insufficient funds.",
			LatencyMs:    90,
		}, nil // No critical adapter error, just a provider decline
	}
	registry := map[string]adapter.ProviderAdapter{"adyen": mockAdapter}
	processor := NewProcessor(registry)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s2", ProviderName: "adyen", Amount: 200}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := processor.ProcessSingleStep(traceCtx, step, stepCtx)
	require.NoError(t, err) // Processor itself doesn't error for provider declines
	require.NotNil(t, result)

	assert.False(t, result.Success)
	assert.Equal(t, "adyen", result.ProviderName)
	assert.Equal(t, expectedProviderError, result.ErrorCode)
	assert.Equal(t, "The transaction was declined due to insufficient funds.", result.ErrorMessage)
}

func TestProcessor_ProcessSingleStep_AdapterExecutionError(t *testing.T) {
	mockAdapter := mock.NewMockAdapter("braintree")
	adapterExecutionErr := fmt.Errorf("network timeout connecting to braintree")
	mockAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		// This result might be partially filled or zero-value depending on when the error occurred
		return adapter.ProviderResult{Provider: mockAdapter.GetName(), StepID: s.StepId, LatencyMs: 500}, adapterExecutionErr
	}
	registry := map[string]adapter.ProviderAdapter{"braintree": mockAdapter}
	processor := NewProcessor(registry)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s3", ProviderName: "braintree", Amount: 300}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := processor.ProcessSingleStep(traceCtx, step, stepCtx)
	require.Error(t, err) // Expecting an error from the processor due to adapter execution failure
	assert.Equal(t, adapterExecutionErr, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "braintree", result.ProviderName) // Should still capture provider name
	assert.Equal(t, "ADAPTER_EXECUTION_ERROR", result.ErrorCode)
	assert.Equal(t, fmt.Sprintf("Adapter %s failed to process: %s", step.ProviderName, adapterExecutionErr.Error()), result.ErrorMessage)
	assert.Equal(t, int64(500), result.LatencyMs) // Check if latency from partial result is passed through
    assert.Equal(t, adapterExecutionErr.Error(), result.Details["adapter_error_message"])
}


func TestProcessor_ProcessSingleStep_AdapterNotFound(t *testing.T) {
	registry := make(map[string]adapter.ProviderAdapter) // Empty registry
	processor := NewProcessor(registry)

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "s4", ProviderName: "unknown_provider", Amount: 400}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := processor.ProcessSingleStep(traceCtx, step, stepCtx)
	// As per spec.md, Processor returns (StepResult, nil) for AdapterNotFound.
	require.NoError(t, err, "Error should be nil for AdapterNotFound as per spec")
	require.NotNil(t, result)

	assert.False(t, result.Success)
	assert.Equal(t, "unknown_provider", result.ProviderName)
	assert.Equal(t, "ADAPTER_NOT_FOUND", result.ErrorCode)
	assert.Contains(t, result.ErrorMessage, "No adapter registered for provider: unknown_provider")
}

func TestProcessor_ProcessSingleStep_NilStep(t *testing.T) {
	registry := make(map[string]adapter.ProviderAdapter)
	processor := NewProcessor(registry)
	traceCtx := context.NewTraceContext()
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := processor.ProcessSingleStep(traceCtx, nil, stepCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processor: payment step cannot be nil")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "PROCESSOR_NIL_STEP", result.ErrorCode)
}
