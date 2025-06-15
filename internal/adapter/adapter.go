// Package adapter defines the interface for payment provider adapters
// and contains implementations for specific providers.
// Adapters handle all provider-specific API calls, including serialization,
// retry logic, backoff strategies, idempotency, and error mapping,
// normalizing raw provider responses into a common ProviderResult format.
// Refer to spec2_1.md, section 7.4 for more details on ProviderAdapters.
package adapter

import (
	"github.com/yourorg/payment-orchestrator/internal/context"
	// Using the new protos path
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// ProviderPayload represents the specific data a provider adapter needs for a payment step.
// This is a placeholder; in a real system, this might be more structured,
// possibly using `google.protobuf.Any` or specific types per provider if known upfront.
// For the `PaymentStep` in `step.proto`, we used `map[string]string provider_payload`.
// We'll assume for now that the adapter receives this map and interprets it.
// type ProviderPayload map[string]string // This matches the step.proto definition

// ProviderResult holds the outcome of a payment processing attempt by a provider adapter.
type ProviderResult struct {
	StepID        string            // Correlates with PaymentStep.StepId
	Success       bool              // Whether the provider confirmed payment success
	Provider      string            // Name of the provider that processed this (e.g., "stripe")
	ErrorCode     string            // Provider-specific error code, if any
	ErrorMessage  string            // Provider-specific error message, if any
	TransactionID string            // Transaction ID from the provider, if successful
	LatencyMs     int64             // Latency of the call to the provider
	RawResponse   []byte            // Raw response body from the provider (for debugging/logging)
	HTTPStatus    int               // HTTP status code from the provider's response
	Details       map[string]string // Any other relevant details from the provider
}

// ProviderAdapter is the interface implemented by each payment gateway adapter.
// It handles all provider-specific API calls, including serialization,
// retry (within limits), backoff, idempotency, and error mapping.
type ProviderAdapter interface {
	// Process handles a single payment step using the specific provider logic.
	// - traceCtx: For logging and tracing.
	// - step: The payment step details, including amount, currency, and provider_payload.
	// - stepCtx: Execution context for this step, including credentials and timeout budget.
	Process(
		traceCtx context.TraceContext,
		step *internalv1.PaymentStep, // Takes the PaymentStep directly
		stepCtx context.StepExecutionContext,
	) (ProviderResult, error)

	// HealthCheck (Optional, can be added later)
	// Performs a health check of the provider's API.
	// HealthCheck(ctx context.Context) error

	// GetName returns the name of the provider (e.g., "stripe", "adyen").
	GetName() string
}
