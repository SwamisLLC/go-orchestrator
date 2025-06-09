package processor_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/mock" // No longer needed for manual mock

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	// timestamppb is no longer needed
)

func TestProcessor_ProcessSingleStep(t *testing.T) {
	t.Run("Successful payment processing", func(t *testing.T) {
		// Arrange
		mockAdapter := new(adaptermock.MockAdapter) // Use correct mock
		proc := processor.NewProcessor()

		baseTraceID := "trace-123"
		stepSpanID := "span-step-123"
		stepCtx := context.StepExecutionContext{
			TraceID:           baseTraceID,
			SpanID:            stepSpanID,
			StartTime:         time.Now(),
			RemainingBudgetMs: 10000,
			// ProviderCredentials would be set here in a real scenario
		}
		expectedTraceCtxForAdapter := context.TraceContext{TraceID: baseTraceID, SpanID: stepSpanID}

		paymentStep := &orchestratorinternalv1.PaymentStep{
			StepId:       "step-abc",
			ProviderName: "test-provider",
			Amount:       1000,
			Currency:     "USD",
			// PaymentMethod: orchestratorinternalv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD, // This field does not exist on PaymentStep
		}
		adapterResponse := adapter.ProviderResult{
			StepID:        "step-abc",
			Success:       true,
			TransactionID: "prov123",
			Provider:      "test-provider",
		}

		var processFuncCalled bool
		mockAdapter.ProcessFunc = func(tc context.TraceContext, step *orchestratorinternalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
			processFuncCalled = true
			assert.Equal(t, expectedTraceCtxForAdapter, tc)
			assert.Equal(t, paymentStep, step)
			assert.Equal(t, stepCtx, sc)
			return adapterResponse, nil
		}

		// Act
		result, err := proc.ProcessSingleStep(stepCtx, paymentStep, mockAdapter)

		// Assert
		assert.NoError(t, err)
		assert.True(t, processFuncCalled, "Adapter's ProcessFunc was not called")
		assert.NotNil(t, result)
		assert.Equal(t, "step-abc", result.GetStepId())
		assert.True(t, result.GetSuccess())
		assert.Equal(t, "test-provider", result.GetProviderName())
		assert.Equal(t, "prov123", result.GetDetails()["provider_transaction_id"])
		assert.Empty(t, result.GetErrorCode())
		assert.Empty(t, result.GetErrorMessage())
	})

	t.Run("Failed payment processing (adapter returns an error)", func(t *testing.T) {
		// Arrange
		mockAdapter := new(adaptermock.MockAdapter) // Use correct mock
		proc := processor.NewProcessor()

		baseTraceID := "trace-456"
		stepSpanID := "span-step-456"
		stepCtx := context.StepExecutionContext{
			TraceID:           baseTraceID,
			SpanID:            stepSpanID,
			StartTime:         time.Now(),
			RemainingBudgetMs: 9000,
		}
		expectedTraceCtxForAdapter := context.TraceContext{TraceID: baseTraceID, SpanID: stepSpanID}

		paymentStep := &orchestratorinternalv1.PaymentStep{
			StepId:       "step-def",
			ProviderName: "test-provider-fail",
			Amount:       2000,
			Currency:     "EUR",
			// PaymentMethod: orchestratorinternalv1.PaymentMethod_PAYMENT_METHOD_BANK_TRANSFER, // This field does not exist on PaymentStep
		}
		expectedAdapterError := errors.New("provider processing error")
		adapterResponse := adapter.ProviderResult{
			StepID:       "step-def",
			Success:      false,
			Provider:     "test-provider-fail",
			ErrorCode:    "PROVIDER_DOWN",
			ErrorMessage: "The provider experienced an internal issue.",
		}

		var processFuncCalled bool
		mockAdapter.ProcessFunc = func(tc context.TraceContext, step *orchestratorinternalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
			processFuncCalled = true
			assert.Equal(t, expectedTraceCtxForAdapter, tc)
			// It's good practice to assert relevant parts of step and sc if they are critical for the mock's behavior
			return adapterResponse, expectedAdapterError
		}

		// Act
		result, err := proc.ProcessSingleStep(stepCtx, paymentStep, mockAdapter)

		// Assert
		assert.Error(t, err)
		assert.True(t, processFuncCalled, "Adapter's ProcessFunc was not called")
		assert.Equal(t, expectedAdapterError, err)
		assert.NotNil(t, result)
		assert.Equal(t, "step-def", result.GetStepId())
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "test-provider-fail", result.GetProviderName())
		assert.Equal(t, "PROVIDER_DOWN", result.GetErrorCode())    // ErrorCode from adapterResult
		assert.Equal(t, "The provider experienced an internal issue.", result.GetErrorMessage()) // ErrorMessage from adapterResult
	})

	t.Run("Failed payment processing (adapter returns success:false, no error)", func(t *testing.T) {
		// Arrange
		mockAdapter := new(adaptermock.MockAdapter)
		proc := processor.NewProcessor()

		baseTraceID := "trace-789"
		stepSpanID := "span-step-789"
		stepCtx := context.StepExecutionContext{
			TraceID:           baseTraceID,
			SpanID:            stepSpanID,
			StartTime:         time.Now(),
			RemainingBudgetMs: 8000,
		}
		expectedTraceCtxForAdapter := context.TraceContext{TraceID: baseTraceID, SpanID: stepSpanID}

		paymentStep := &orchestratorinternalv1.PaymentStep{
			StepId:       "step-ghi",
			ProviderName: "test-provider-reject",
			Amount:       3000,
			Currency:     "GBP",
			// PaymentMethod: orchestratorinternalv1.PaymentMethod_PAYMENT_METHOD_IDEAL, // This field does not exist on PaymentStep
		}
		adapterResponse := adapter.ProviderResult{
			StepID:       "step-ghi",
			Success:      false, // Explicitly false
			Provider:     "test-provider-reject",
			ErrorCode:    "CARD_DECLINED",
			ErrorMessage: "Card was declined by issuer.",
		}

		var processFuncCalled bool
		mockAdapter.ProcessFunc = func(tc context.TraceContext, step *orchestratorinternalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error) {
			processFuncCalled = true
			assert.Equal(t, expectedTraceCtxForAdapter, tc) // Added assertion for consistency
			return adapterResponse, nil // No error returned by adapter
		}

		// Act
		result, err := proc.ProcessSingleStep(stepCtx, paymentStep, mockAdapter)

		// Assert
		assert.NoError(t, err) // No error from ProcessSingleStep itself
		assert.True(t, processFuncCalled, "Adapter's ProcessFunc was not called")
		assert.NotNil(t, result)
		assert.Equal(t, "step-ghi", result.GetStepId())
		assert.False(t, result.GetSuccess())
		assert.Equal(t, "test-provider-reject", result.GetProviderName())
		assert.Equal(t, "CARD_DECLINED", result.GetErrorCode())
		assert.Equal(t, "Card was declined by issuer.", result.GetErrorMessage())
	})
}
