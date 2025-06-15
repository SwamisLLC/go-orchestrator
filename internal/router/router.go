// Package router coordinates multi-provider orchestration for individual payment steps.
// Its responsibilities include circuit-breaking, fallback sequencing based on policies,
// SLA budget enforcement, parallel fan-out handling (if applicable), and emitting
// unified telemetry for step attempts.
// Refer to spec2_1.md, section 7.3 for more details on the Router.
package router

import (
	"errors" // Added for errors.New
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

const defaultMinRequiredBudgetMs int64 = 50 // milliseconds

// RouterConfig holds the configuration for the router's provider selection logic.
type RouterConfig struct {
	PrimaryProviderName  string
	FallbackProviderName string
	// MinRequiredBudgetMs int64 // Optional: to make it configurable per router instance
}

// ProcessorInterface defines the contract for processing a single payment step.
type ProcessorInterface interface {
	ProcessSingleStep(
		traceCtx context.TraceContext,
		stepCtx context.StepExecutionContext,
		step *orchestratorinternalv1.PaymentStep,
		selectedAdapter adapter.ProviderAdapter,
	) (*orchestratorinternalv1.StepResult, error)
}

// Router is responsible for selecting a payment provider and executing a payment step.
type Router struct {
	processor ProcessorInterface
	adapters  map[string]adapter.ProviderAdapter
	config    RouterConfig
	cb        *circuitbreaker.CircuitBreaker
}

// NewRouter creates a new Router instance.
func NewRouter(
	processor ProcessorInterface,
	adapters map[string]adapter.ProviderAdapter,
	config RouterConfig,
	cb *circuitbreaker.CircuitBreaker,
) (*Router, error) {
	if processor == nil {
		return nil, fmt.Errorf("processor cannot be nil")
	}
	if adapters == nil {
		return nil, fmt.Errorf("adapters map cannot be nil")
	}
	if cb == nil {
		return nil, fmt.Errorf("circuit breaker cannot be nil")
	}
	if config.PrimaryProviderName == "" {
		return nil, fmt.Errorf("primary provider name in config cannot be empty")
	}
	return &Router{
		processor: processor,
		adapters:  adapters,
		config:    config,
		cb:        cb,
	}, nil
}

func (r *Router) ExecuteStep(
	originalTraceCtx context.TraceContext, // Renamed to avoid conflict with context from span
	stepCtx context.StepExecutionContext, // Renamed from ctx to avoid conflict
	step *orchestratorinternalv1.PaymentStep,
) (*orchestratorinternalv1.StepResult, error) {
	// Get a tracer instance
	tracer := otel.Tracer("router")

	// Start a new span
	ctx, span := tracer.Start(originalTraceCtx.Context(), "Router.ExecuteStep")
	defer span.End()

	// Update traceCtx with the new context from the span
	// The new stdCtx (named 'ctx' here) comes from tracer.Start()
	// The TraceID is propagated from the originalTraceCtx
	// The SpanID for the new TraceContext should be the ID of the new OpenTelemetry span
	traceCtx := context.NewTraceContextWithIDs(ctx, originalTraceCtx.GetTraceID(), span.SpanContext().SpanID().String())

	log.Printf("Router.ExecuteStep: [%s/%s] Processing step %s with primary provider %s. Budget: %dms",
		traceCtx.GetTraceID(), traceCtx.GetSpanID(), step.GetStepId(), r.config.PrimaryProviderName, stepCtx.RemainingBudgetMs)

	minBudget := defaultMinRequiredBudgetMs

	if stepCtx.RemainingBudgetMs < minBudget { // Corrected: stepCtx was shadowed by ctx from span
		errMsg := fmt.Sprintf("insufficient SLA budget (%dms) for primary provider %s, minimum required %dms", stepCtx.RemainingBudgetMs, r.config.PrimaryProviderName, minBudget)
		log.Printf("Router.ExecuteStep: %s. Attempting fallback.", errMsg)
		primaryResultForFallback := &orchestratorinternalv1.StepResult{
			StepId:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "SLA_BUDGET_EXCEEDED",
			ErrorMessage: errMsg,
			ProviderName: r.config.PrimaryProviderName,
		}
		// ExecuteStep calls tryFallback, traceCtx must be passed here.
		return r.tryFallback(traceCtx, stepCtx, step, primaryResultForFallback, nil) // Corrected: stepCtx
	}

	primaryAdapter, ok := r.adapters[r.config.PrimaryProviderName]
	if !ok {
		errMsg := fmt.Sprintf("primary provider adapter '%s' not found", r.config.PrimaryProviderName)
		log.Printf("Router.ExecuteStep: Error: %s", errMsg)
		return &orchestratorinternalv1.StepResult{
			StepId:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "ROUTER_CONFIGURATION_ERROR",
			ErrorMessage: errMsg,
			ProviderName: r.config.PrimaryProviderName,
		}, errors.New(errMsg) // Changed to errors.New
	}

	if !r.cb.AllowRequest(r.config.PrimaryProviderName) {
		errMsg := fmt.Sprintf("circuit open for primary provider %s", r.config.PrimaryProviderName)
		log.Printf("Router.ExecuteStep: %s. Attempting fallback.", errMsg)
		primaryResultForFallback := &orchestratorinternalv1.StepResult{
			StepId:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "CIRCUIT_OPEN",
			ErrorMessage: errMsg,
			ProviderName: r.config.PrimaryProviderName,
		}
		// ExecuteStep calls tryFallback, traceCtx must be passed here.
		return r.tryFallback(traceCtx, stepCtx, step, primaryResultForFallback, nil) // Corrected: stepCtx
	}

	primaryStepResult, primaryErr := r.processor.ProcessSingleStep(traceCtx, stepCtx, step, primaryAdapter) // Corrected: stepCtx

	if primaryStepResult == nil {
		if primaryErr != nil {
			 primaryStepResult = &orchestratorinternalv1.StepResult{StepId: step.GetStepId(), Success: false, ErrorCode: "PROCESSOR_ERROR", ErrorMessage: primaryErr.Error(), ProviderName: r.config.PrimaryProviderName}
		} else {
			 primaryStepResult = &orchestratorinternalv1.StepResult{StepId: step.GetStepId(), Success: false, ErrorCode: "UNKNOWN_PROCESSOR_FAILURE", ErrorMessage: "Processor returned nil result and nil error", ProviderName: r.config.PrimaryProviderName}
		}
	}
	primaryStepResult.ProviderName = r.config.PrimaryProviderName

	if primaryErr != nil || !primaryStepResult.Success {
		log.Printf("Router.ExecuteStep: Primary provider %s failed for step %s. Error: %v, ResultSuccess: %t. Recording failure and attempting fallback.", r.config.PrimaryProviderName, step.GetStepId(), primaryErr, primaryStepResult.Success)
		r.cb.RecordFailure(r.config.PrimaryProviderName)
		// ExecuteStep calls tryFallback, traceCtx must be passed here.
		return r.tryFallback(traceCtx, stepCtx, step, primaryStepResult, primaryErr) // Corrected: stepCtx
	}

	log.Printf("Router.ExecuteStep: [%s/%s] Primary provider %s succeeded for step %s. Recording success.",
		traceCtx.TraceID, traceCtx.SpanID, r.config.PrimaryProviderName, step.GetStepId())
	r.cb.RecordSuccess(r.config.PrimaryProviderName)
	return primaryStepResult, nil
}

func (r *Router) tryFallback(
	originalTraceCtx context.TraceContext, // Renamed to avoid conflict
	stepCtx context.StepExecutionContext, // Renamed from ctx to avoid conflict
	step *orchestratorinternalv1.PaymentStep,
	primaryResult *orchestratorinternalv1.StepResult,
	primaryError error,
) (*orchestratorinternalv1.StepResult, error) {
	// Get a tracer instance - assuming we want a new span for fallback attempt as well
	tracer := otel.Tracer("router") // Or use a more specific name like "router.fallback"

	// Start a new span for the fallback attempt
	// It's important to use originalTraceCtx here to link it to the parent ExecuteStep span correctly
	ctxFromSpan, span := tracer.Start(originalTraceCtx.Context(), "Router.tryFallback") // Renamed ctx to ctxFromSpan
	defer span.End()

	// Update traceCtx for this scope
	// The new stdCtx (named 'ctxFromSpan' here) comes from tracer.Start()
	// The TraceID is propagated from the originalTraceCtx
	// The SpanID for the new TraceContext should be the ID of the new OpenTelemetry span
	traceCtx := context.NewTraceContextWithIDs(ctxFromSpan, originalTraceCtx.GetTraceID(), span.SpanContext().SpanID().String())


	if r.config.FallbackProviderName == "" {
		log.Printf("Router.tryFallback: [%s/%s] No fallback provider configured for step %s. Returning primary result.",
			traceCtx.GetTraceID(), traceCtx.GetSpanID(), step.GetStepId()) // Use new traceCtx for logging
		return primaryResult, primaryError
	}

	log.Printf("Router.tryFallback: [%s/%s] Attempting fallback with provider %s for step %s. Budget: %dms",
		traceCtx.GetTraceID(), traceCtx.GetSpanID(), r.config.FallbackProviderName, step.GetStepId(), stepCtx.RemainingBudgetMs) // Use new traceCtx and stepCtx

	minBudget := defaultMinRequiredBudgetMs

	if stepCtx.RemainingBudgetMs < minBudget { // Use stepCtx
		errMsg := fmt.Sprintf("insufficient SLA budget (%dms) for fallback provider %s, minimum required %dms", stepCtx.RemainingBudgetMs, r.config.FallbackProviderName, minBudget) // Corrected: stepCtx
		log.Printf("Router.tryFallback: %s. Returning primary result.", errMsg)
		if primaryError == nil && primaryResult != nil {
			primaryResult.ErrorMessage = fmt.Sprintf("Primary Result (Code: %s, Msg: %s); Fallback Skipped (SLA): %s",
				primaryResult.GetErrorCode(), primaryResult.GetErrorMessage(), errMsg)
			if primaryResult.ErrorCode == "CIRCUIT_OPEN" || primaryResult.ErrorCode == "SLA_BUDGET_EXCEEDED" {
				primaryResult.ErrorCode = "ALL_PROVIDERS_UNAVAILABLE"
			}
		}
		return primaryResult, primaryError
	}

	fallbackAdapter, ok := r.adapters[r.config.FallbackProviderName]
	if !ok {
		errMsg := fmt.Sprintf("fallback provider adapter '%s' not found", r.config.FallbackProviderName)
		log.Printf("Router.tryFallback: Error: %s. Modifying primary result to reflect this.", errMsg)

		originalPrimaryErrorCode := primaryResult.GetErrorCode()
		originalPrimaryErrorMsg := primaryResult.GetErrorMessage()

		primaryResult.Success = false
		primaryResult.ErrorCode = "ROUTER_CONFIGURATION_ERROR"
		primaryResult.ErrorMessage = fmt.Sprintf("Primary Failure (Code: %s, Msg: %s); Fallback Attempt Failed: %s",
			originalPrimaryErrorCode, originalPrimaryErrorMsg, errMsg)

		if primaryError == nil {
			return primaryResult, errors.New(errMsg) // Changed to errors.New
		}
		return primaryResult, primaryError
	}

	if !r.cb.AllowRequest(r.config.FallbackProviderName) {
		errMsg := fmt.Sprintf("circuit open for fallback provider %s", r.config.FallbackProviderName)
		log.Printf("Router.tryFallback: %s. Returning primary result.", errMsg)
		primaryResult.ErrorMessage = fmt.Sprintf("Primary Error: [%s]; Fallback Circuit Open: [%s]", primaryResult.ErrorMessage, errMsg)
		if primaryResult.ErrorCode == "CIRCUIT_OPEN" || primaryResult.ErrorCode == "SLA_BUDGET_EXCEEDED" {
		    primaryResult.ErrorCode = "ALL_PROVIDERS_UNAVAILABLE"
        }
		return primaryResult, primaryError
	}

	fallbackStepResult, fallbackErr := r.processor.ProcessSingleStep(traceCtx, stepCtx, step, fallbackAdapter) // Corrected: stepCtx

	if fallbackStepResult == nil {
		if fallbackErr != nil {
			fallbackStepResult = &orchestratorinternalv1.StepResult{StepId: step.GetStepId(), Success: false, ErrorCode: "PROCESSOR_ERROR", ErrorMessage: fallbackErr.Error(), ProviderName: r.config.FallbackProviderName}
		} else {
			fallbackStepResult = &orchestratorinternalv1.StepResult{StepId: step.GetStepId(), Success: false, ErrorCode: "UNKNOWN_PROCESSOR_FAILURE", ErrorMessage: "Processor returned nil result and nil error", ProviderName: r.config.FallbackProviderName}
		}
	}
	fallbackStepResult.ProviderName = r.config.FallbackProviderName

	if fallbackErr != nil || !fallbackStepResult.Success {
		log.Printf("Router.tryFallback: [%s/%s] Fallback provider %s failed for step %s. Error: %v, ResultSuccess: %t. Recording failure.",
			traceCtx.TraceID, traceCtx.SpanID, r.config.FallbackProviderName, step.GetStepId(), fallbackErr, fallbackStepResult.Success)
		r.cb.RecordFailure(r.config.FallbackProviderName)
	} else {
		log.Printf("Router.tryFallback: [%s/%s] Fallback provider %s succeeded for step %s. Recording success.",
			traceCtx.TraceID, traceCtx.SpanID, r.config.FallbackProviderName, step.GetStepId())
		r.cb.RecordSuccess(r.config.FallbackProviderName)
	}
	return fallbackStepResult, fallbackErr
}
