package processor

import (
	"log" // Using standard log for now

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	// timestamppb is not needed as StepResult does not have a timestamp field
)

// Processor handles the execution of a single payment step by invoking the appropriate provider adapter.
type Processor struct {
	// logger can be added here if a more sophisticated logger is introduced.
	// For now, we'll use the standard `log` package.
}

// NewProcessor creates a new instance of the Processor.
func NewProcessor() *Processor {
	return &Processor{}
}

// ProcessSingleStep instructs a given provider adapter to process a payment step.
// It takes the step-specific execution context, the payment step details, and the adapter to use.
// It returns the result of the step execution and any error encountered.
func (p *Processor) ProcessSingleStep(
	ctx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
	providerAdapter adapter.ProviderAdapter,
) (*orchestratorinternalv1.StepResult, error) {
	log.Printf("Processor: Starting ProcessSingleStep for StepID: %s, Provider: %s", step.GetStepId(), step.GetProviderName())

	// Construct TraceContext for the adapter
	traceCtxForAdapter := context.TraceContext{
		TraceID: ctx.TraceID,
		SpanID:  ctx.SpanID,
	}

	// Delegate the actual processing to the adapter
	adapterResult, err := providerAdapter.Process(traceCtxForAdapter, step, ctx)

	// Convert adapter.ProviderResult to orchestratorinternalv1.StepResult
	stepResultDetails := make(map[string]string)
	if adapterResult.TransactionID != "" {
		stepResultDetails["provider_transaction_id"] = adapterResult.TransactionID
	}
	// Potentially copy other details from adapterResult.Details to stepResultDetails if needed

	stepResult := &orchestratorinternalv1.StepResult{
		StepId:        adapterResult.StepID, // Should match step.GetStepId()
		ProviderName:  step.GetProviderName(), // The provider that was intended to be used for this step
		Success:       adapterResult.Success,
		ErrorCode:     adapterResult.ErrorCode,
		ErrorMessage:  adapterResult.ErrorMessage,
		LatencyMs:     adapterResult.LatencyMs, // Assuming ProviderResult has LatencyMs
		Details:       stepResultDetails,
	}

	if err != nil {
		log.Printf("Processor: Error processing StepID %s with Provider %s: %v", step.GetStepId(), step.GetProviderName(), err)
		stepResult.Success = false
		// Ensure ErrorCode and ErrorMessage from the error are prioritized if adapterResult didn't set them well
		if stepResult.ErrorCode == "" {
			stepResult.ErrorCode = "PROCESSOR_EXECUTION_ERROR" // Generic processor error
		}
		if stepResult.ErrorMessage == "" {
			stepResult.ErrorMessage = err.Error()
		}
		return stepResult, err // Return the original error
	}

	// If no error, adapterResult.Success is the source of truth for success status
	// ErrorCode and ErrorMessage from adapterResult are already mapped.
	// If !adapterResult.Success but err is nil, ensure ErrorCode/Msg are populated.
	if !stepResult.Success {
		if stepResult.ErrorCode == "" {
			stepResult.ErrorCode = "PROVIDER_OPERATION_FAILED"
		}
		if stepResult.ErrorMessage == "" {
			stepResult.ErrorMessage = "Provider indicated failure without a specific error message."
		}
	}

	log.Printf("Processor: Finished ProcessSingleStep for StepID %s, Provider: %s, Success: %t", step.GetStepId(), step.GetProviderName(), stepResult.GetSuccess())
	return stepResult, nil
}
