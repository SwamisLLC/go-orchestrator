package router

import (
	"fmt"
	// "time" // No longer needed if MinRemainingBudgetMs is just an int64

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

const MinRemainingBudgetMs = 10

type Router struct {
	processor               *processor.Processor
	primaryFallbackProvider string
	circuitBreaker          *circuitbreaker.CircuitBreaker
}

func NewRouter(p *processor.Processor, fallbackProvider string, cb *circuitbreaker.CircuitBreaker) *Router {
	if p == nil {
		panic("processor cannot be nil")
	}
	if cb == nil {
		panic("circuit breaker cannot be nil")
	}
	return &Router{
		processor: p,
		primaryFallbackProvider: fallbackProvider,
		circuitBreaker:          cb,
	}
}

func (r *Router) ExecuteStep(
	traceCtx context.TraceContext,
	step *internalv1.PaymentStep,
	stepCtx context.StepExecutionContext,
) (*internalv1.StepResult, error) {
	if step == nil {
		return nil, fmt.Errorf("router: payment step cannot be nil")
	}

	// 1. SLA Budget Enforcement
	if stepCtx.RemainingBudgetMs < MinRemainingBudgetMs {
		return &internalv1.StepResult{
			StepId:       step.StepId,
			Success:      false,
			ProviderName: step.ProviderName,
			ErrorCode:    "TIMEOUT_BUDGET_EXCEEDED",
			ErrorMessage: fmt.Sprintf("SLA budget exhausted before attempting provider %s. Remaining: %dms", step.ProviderName, stepCtx.RemainingBudgetMs),
			Details:      map[string]string{"remaining_budget_ms": fmt.Sprintf("%d", stepCtx.RemainingBudgetMs)},
		}, nil
	}

	// 2. Attempt Primary Provider
	var primaryResult *internalv1.StepResult
	var primaryErr error

	if !r.circuitBreaker.IsHealthy(step.ProviderName) {
		primaryResult = &internalv1.StepResult{
			StepId:       step.StepId,
			Success:      false,
			ProviderName: step.ProviderName,
			ErrorCode:    "CIRCUIT_OPEN",
			ErrorMessage: fmt.Sprintf("Circuit open for primary provider: %s", step.ProviderName),
			Details:      map[string]string{"info": "Circuit breaker tripped for primary provider"},
		}
	} else {
		primaryResult, primaryErr = r.processor.ProcessSingleStep(traceCtx, step, stepCtx)
		if primaryErr != nil {
			r.circuitBreaker.RecordFailure(step.ProviderName)
			return primaryResult, fmt.Errorf("router: error processing step with primary provider %s: %w", step.ProviderName, primaryErr)
		}
		if primaryResult.Success {
			r.circuitBreaker.RecordSuccess(step.ProviderName)
		} else {
			r.circuitBreaker.RecordFailure(step.ProviderName)
		}
	}

	// 3. Fallback Logic
	if primaryResult.Success || r.primaryFallbackProvider == "" || r.primaryFallbackProvider == step.ProviderName {
		return primaryResult, nil
	}

	fallbackStep := &internalv1.PaymentStep{
		StepId:       step.StepId,
		ProviderName: r.primaryFallbackProvider,
		Amount:       step.Amount,
		Currency:     step.Currency,
		IsFanOut:     step.IsFanOut,
		ProviderPayload: step.ProviderPayload,
	}
	if step.Metadata != nil {
		fallbackStep.Metadata = make(map[string]string)
		for k, v := range step.Metadata {
			fallbackStep.Metadata[k] = v
		}
	} else {
		fallbackStep.Metadata = make(map[string]string)
	}
	fallbackStep.Metadata["fallback_of_step_id"] = step.StepId
	fallbackStep.Metadata["original_provider"] = step.ProviderName

	if stepCtx.RemainingBudgetMs < MinRemainingBudgetMs {
		if primaryResult.Details == nil { primaryResult.Details = make(map[string]string) }
		primaryResult.Details["fallback_attempt_skipped_reason"] = "SLA budget exhausted before fallback"
		return primaryResult, nil
	}

	if !r.circuitBreaker.IsHealthy(fallbackStep.ProviderName) {
		if primaryResult.Details == nil { primaryResult.Details = make(map[string]string) }
		primaryResult.Details["fallback_attempt_skipped_reason"] = fmt.Sprintf("Circuit open for fallback provider: %s", fallbackStep.ProviderName)
		return primaryResult, nil
	}

	fallbackResult, fallbackErr := r.processor.ProcessSingleStep(traceCtx, fallbackStep, stepCtx)
	if fallbackErr != nil {
		r.circuitBreaker.RecordFailure(fallbackStep.ProviderName)
		if fallbackResult == nil {
		    fallbackResult = &internalv1.StepResult{StepId: fallbackStep.StepId, Success: false, ProviderName: fallbackStep.ProviderName, Details: make(map[string]string)}
		} else if fallbackResult.Details == nil {
		    fallbackResult.Details = make(map[string]string)
		}
		fallbackResult.Details["original_provider_attempted"] = step.ProviderName
		fallbackResult.Details["is_fallback_attempt_failed_processor"] = "true"
		return fallbackResult, fmt.Errorf("router: error processing step with fallback provider %s: %w", fallbackStep.ProviderName, fallbackErr)
	}

	if fallbackResult.Success {
		r.circuitBreaker.RecordSuccess(fallbackStep.ProviderName)
	} else {
		r.circuitBreaker.RecordFailure(fallbackStep.ProviderName)
	}

    if fallbackResult.Details == nil { fallbackResult.Details = make(map[string]string) }
	fallbackResult.Details["is_fallback"] = "true"
	fallbackResult.Details["original_provider_attempted"] = step.ProviderName
	return fallbackResult, nil
}
