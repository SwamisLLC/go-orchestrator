package processor

import (
	"log"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	customcontext "github.com/yourorg/payment-orchestrator/internal/context"
	protos "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// Processor is responsible for executing a single payment step with a given provider adapter.
type Processor struct {
	// Processor currently doesn't need to hold any state.
	// It acts as a stateless executor of a step via a provided adapter.
}

// NewProcessor creates a new Processor.
func NewProcessor() *Processor {
	return &Processor{}
}

// ProcessSingleStep executes a payment step using the chosen provider adapter.
// It translates the adapter's specific result into the common StepResult protobuf.
func (p *Processor) ProcessSingleStep(
	traceCtx customcontext.TraceContext,        // For logging & tracing
	stepCtx customcontext.StepExecutionContext, // Context for this specific step (e.g., API keys)
	step *protos.PaymentStep,                   // The payment step to execute
	selectedAdapter adapter.ProviderAdapter,   // The adapter chosen by the Router
) (*protos.StepResult, error) {
	log.Printf("Processor: [%s/%s] Starting ProcessSingleStep, StepID: %s, Provider: %s, StepSpan: %s",
		traceCtx.TraceID, traceCtx.SpanID, step.GetStepId(), selectedAdapter.GetName(), stepCtx.SpanID)

	startTime := time.Now()

	providerResult, err := selectedAdapter.Process(traceCtx, step, stepCtx)

	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		// Error returned by the adapter's Process method itself (e.g., network issue, config error)
		// This is different from a provider declining a payment but the call itself succeeding.
		log.Printf("Processor: [%s/%s] Adapter processing failed for StepID %s, Provider: %s, Error: %v",
			traceCtx.TraceID, traceCtx.SpanID, step.GetStepId(), selectedAdapter.GetName(), err)
		return &protos.StepResult{
			StepId:       step.GetStepId(),
			Success:      false,
			ProviderName: selectedAdapter.GetName(),
			ErrorCode:    "ADAPTER_ERROR", // Generic code for errors within adapter execution
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
			Details:      nil, // No provider details if adapter itself failed
		}, err // Return the original error as well
	}

	// If adapter.Process returned no error, we map ProviderResult to StepResult
	stepResultProto := &protos.StepResult{
		StepId:       providerResult.StepID,
		Success:      providerResult.Success,
		ProviderName: providerResult.Provider, // Should match selectedAdapter.GetName()
		ErrorCode:    providerResult.ErrorCode,
		ErrorMessage: providerResult.ErrorMessage,
		LatencyMs:    latencyMs, // Overwrite with calculated latency if adapter didn't provide one, or use its value.
		Details:      providerResult.Details,
	}

	// Ensure latency is recorded, either from providerResult or calculated
	if providerResult.LatencyMs > 0 {
		stepResultProto.LatencyMs = providerResult.LatencyMs
	}


	if providerResult.Success {
		log.Printf("Processor: [%s/%s] Step processed successfully, StepID: %s, Provider: %s, TransactionID: %s",
			traceCtx.TraceID, traceCtx.SpanID, step.GetStepId(), selectedAdapter.GetName(), providerResult.TransactionID)
		if stepResultProto.Details == nil && providerResult.TransactionID != "" {
			stepResultProto.Details = make(map[string]string)
		}
		if providerResult.TransactionID != "" {
			stepResultProto.Details["transaction_id"] = providerResult.TransactionID
		}
	} else {
		log.Printf("Processor: [%s/%s] Step processing failed by provider, StepID: %s, Provider: %s, ErrorCode: %s, ErrorMessage: %s",
			traceCtx.TraceID, traceCtx.SpanID, step.GetStepId(), selectedAdapter.GetName(), providerResult.ErrorCode, providerResult.ErrorMessage)
	}

	return stepResultProto, nil
}
