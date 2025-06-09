package router

import (
	"fmt"
	"log" // Using standard log for now

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	// "github.com/yourorg/payment-orchestrator/internal/processor" // Not directly used by router struct, but by ProcessorInterface
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// RouterConfig holds the configuration for the router's provider selection logic.
type RouterConfig struct {
	PrimaryProviderName  string
	FallbackProviderName string
}

// ProcessorInterface defines the contract for processing a single payment step.
// This allows for mocking the processor in router tests.
type ProcessorInterface interface {
	ProcessSingleStep(
		ctx context.StepExecutionContext,
		step *orchestratorinternalv1.PaymentStep,
		adapter adapter.ProviderAdapter,
	) (*orchestratorinternalv1.StepResult, error)
}

// Router is responsible for selecting a payment provider and executing a payment step.
// It currently implements a simple primary/fallback logic.
type Router struct {
	processor ProcessorInterface
	adapters  map[string]adapter.ProviderAdapter
	config    RouterConfig
	// logger    *log.Logger // Example for a more structured logger
}

// NewRouter creates a new Router instance.
func NewRouter(
	processor ProcessorInterface,
	adapters map[string]adapter.ProviderAdapter,
	config RouterConfig,
) *Router {
	return &Router{
		processor: processor,
		adapters:  adapters,
		config:    config,
	}
}

// ExecuteStep attempts to process a payment step using a primary provider,
// and falls back to a secondary provider if the primary attempt fails.
func (r *Router) ExecuteStep(
	ctx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
) (*orchestratorinternalv1.StepResult, error) {
	log.Printf("Router: Starting ExecuteStep for StepID: %s, Original Provider in Step: %s", step.GetStepId(), step.GetProviderName())

	// Attempt with Primary Provider
	primaryAdapter, ok := r.adapters[r.config.PrimaryProviderName]
	if !ok {
		err := fmt.Errorf("router: primary provider adapter '%s' not found", r.config.PrimaryProviderName)
		log.Printf("Router: Error - %v", err)
		// Return a StepResult indicating failure due to configuration error
		return &orchestratorinternalv1.StepResult{
			StepId:       step.GetStepId(),
			Success:      false,
			ProviderName: r.config.PrimaryProviderName, // The one we attempted to use
			ErrorCode:    "ROUTER_CONFIGURATION_ERROR",
			ErrorMessage: err.Error(),
		}, err
	}

	log.Printf("Router: Attempting StepID %s with primary provider: %s", step.GetStepId(), r.config.PrimaryProviderName)
	// Update step's provider name to reflect the one being used by the router for this attempt
	// This is important if the original step.ProviderName was a generic hint or empty.
	// However, the processor will also set ProviderName in the result based on the adapter it used.
	// For clarity, we can log which provider the router *intends* to use.
	// The actual step.ProviderName field might be more of an input preference.
	// Let's assume the processor correctly sets ProviderName in the StepResult.

	result, err := r.processor.ProcessSingleStep(ctx, step, primaryAdapter)

	if err != nil || (result != nil && !result.GetSuccess()) {
		log.Printf("Router: Primary provider %s failed for StepID %s. Error: %v, ResultSuccess: %t. Attempting fallback.",
			r.config.PrimaryProviderName, step.GetStepId(), err, result != nil && result.GetSuccess())

		fallbackAdapter, ok := r.adapters[r.config.FallbackProviderName]
		if !ok {
			fallbackErr := fmt.Errorf("router: fallback provider adapter '%s' not found", r.config.FallbackProviderName)
			log.Printf("Router: Error - %v", fallbackErr)
			// Return a StepResult indicating failure due to configuration error
			// We return the original error/result from primary if that's more informative,
			// or this new configuration error. The spec implies returning the fallback's outcome.
			// If primary failed and fallback config is missing, this is a critical router/config issue.
			return &orchestratorinternalv1.StepResult{
				StepId:       step.GetStepId(),
				Success:      false,
				ProviderName: r.config.FallbackProviderName, // The one we attempted to use
				ErrorCode:    "ROUTER_CONFIGURATION_ERROR",
				ErrorMessage: fallbackErr.Error(),
			}, fallbackErr // Return the fallback config error
		}

		log.Printf("Router: Attempting StepID %s with fallback provider: %s", step.GetStepId(), r.config.FallbackProviderName)
		return r.processor.ProcessSingleStep(ctx, step, fallbackAdapter)
	}

	log.Printf("Router: Primary provider %s succeeded for StepID %s.", r.config.PrimaryProviderName, step.GetStepId())
	return result, err
}
