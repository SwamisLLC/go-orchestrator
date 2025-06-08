package processor

import (
	"fmt"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// Processor wraps ProviderAdapter calls to return an internal StepResult.
// It's responsible for selecting the correct adapter and translating its result.
type Processor struct {
	adapterRegistry map[string]adapter.ProviderAdapter
}

// NewProcessor creates a new Processor with a given adapter registry.
func NewProcessor(registry map[string]adapter.ProviderAdapter) *Processor {
	if registry == nil {
		panic("adapter registry cannot be nil")
	}
	return &Processor{
		adapterRegistry: registry,
	}
}

// ProcessSingleStep selects the appropriate provider adapter and calls its Process method.
// It then maps the adapter.ProviderResult to an internalv1.StepResult.
func (p *Processor) ProcessSingleStep(
	traceCtx context.TraceContext,
	step *internalv1.PaymentStep,
	stepCtx context.StepExecutionContext,
) (*internalv1.StepResult, error) {
	if step == nil {
		return &internalv1.StepResult{
			Success:      false,
			ErrorCode:    "PROCESSOR_NIL_STEP",
			ErrorMessage: "Processor received a nil payment step",
		}, fmt.Errorf("processor: payment step cannot be nil")
	}

	adapterToUse, ok := p.adapterRegistry[step.ProviderName]
	if !ok {
	    return &internalv1.StepResult{
			StepId:       step.StepId,
			Success:      false,
			ProviderName: step.ProviderName,
			ErrorCode:    "ADAPTER_NOT_FOUND",
			ErrorMessage: fmt.Sprintf("No adapter registered for provider: %s", step.ProviderName),
			Details:      map[string]string{"info": "No adapter registered"},
		}, nil
	}

	providerRes, err := adapterToUse.Process(traceCtx, step, stepCtx)

	if err != nil {
		details := providerRes.Details
		if details == nil {
			details = make(map[string]string)
		}
		details["adapter_error_message"] = err.Error()

		return &internalv1.StepResult{
			StepId:       step.StepId,
			Success:      false,
			ProviderName: step.ProviderName,
			ErrorCode:    "ADAPTER_EXECUTION_ERROR",
			ErrorMessage: fmt.Sprintf("Adapter %s failed to process: %s", step.ProviderName, err.Error()),
			LatencyMs:    providerRes.LatencyMs,
			Details:      details,
		}, err
	}

	stepResult := &internalv1.StepResult{
		StepId:       step.StepId,
		Success:      providerRes.Success,
		ProviderName: providerRes.Provider,
		ErrorCode:    providerRes.ErrorCode,
		ErrorMessage: providerRes.ErrorMessage,
		LatencyMs:    providerRes.LatencyMs,
		Details:      providerRes.Details,
	}

    if stepResult.Details == nil && providerRes.Details != nil {
        stepResult.Details = providerRes.Details
    } else if stepResult.Details == nil {
         stepResult.Details = make(map[string]string)
    }

	return stepResult, nil
}
