package mock

import (
	"fmt"
	"testing"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMockAdapter(t *testing.T) {
	mock := NewMockAdapter("test_mock")
	require.NotNil(t, mock)
	assert.Equal(t, "test_mock", mock.GetName())
}

func TestMockAdapter_Process_DefaultBehavior(t *testing.T) {
	mock := NewMockAdapter("default_mock")
	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{
		StepId:   "step-123",
		Amount:   1000,
		Currency: "USD",
		ProviderPayload: make(map[string]string), // Ensure not nil
	}
	stepCtx := context.StepExecutionContext{
		TraceID: traceCtx.TraceID,
		SpanID:  traceCtx.NewSpan(),
		StartTime: time.Now(),
	}

	result, err := mock.Process(traceCtx, step, stepCtx)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "step-123", result.StepID)
	assert.Equal(t, "default_mock", result.Provider)
	assert.NotEmpty(t, result.TransactionID)
	assert.True(t, result.LatencyMs >= 0)
	assert.Equal(t, "true", result.Details["mock_processed"])
}

func TestMockAdapter_Process_WithCustomFunc_Success(t *testing.T) {
	mock := NewMockAdapter("custom_mock_success")
	customTransactionID := "custom-tx-id-success"
	mock.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{
			StepID:        s.StepId,
			Success:       true,
			Provider:      mock.GetName(),
			TransactionID: customTransactionID,
			LatencyMs:     50,
			Details:       map[string]string{"custom_logic": "applied_success"},
		}, nil
	}

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "step-custom-success", ProviderPayload: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := mock.Process(traceCtx, step, stepCtx)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "step-custom-success", result.StepID)
	assert.Equal(t, "custom_mock_success", result.Provider)
	assert.Equal(t, customTransactionID, result.TransactionID)
	assert.Equal(t, int64(50), result.LatencyMs)
	assert.Equal(t, "applied_success", result.Details["custom_logic"])
}

func TestMockAdapter_Process_WithCustomFunc_Error(t *testing.T) {
	mock := NewMockAdapter("custom_mock_error")
	expectedError := fmt.Errorf("custom processing error")
	mock.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
		return adapter.ProviderResult{
			StepID:       s.StepId,
			Success:      false,
			Provider:     mock.GetName(),
			ErrorCode:    "E101",
			ErrorMessage: "Something went wrong",
			LatencyMs:    75,
		}, expectedError
	}

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "step-custom-error", ProviderPayload: make(map[string]string)}
	stepCtx := context.StepExecutionContext{StartTime: time.Now()}

	result, err := mock.Process(traceCtx, step, stepCtx)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)

	assert.False(t, result.Success)
	assert.Equal(t, "step-custom-error", result.StepID)
	assert.Equal(t, "custom_mock_error", result.Provider)
	assert.Equal(t, "E101", result.ErrorCode)
	assert.Equal(t, "Something went wrong", result.ErrorMessage)
	assert.Equal(t, int64(75), result.LatencyMs)
}

func TestMockAdapter_GetName(t *testing.T) {
	mock := NewMockAdapter("my_mock_adapter")
	assert.Equal(t, "my_mock_adapter", mock.GetName())
}
