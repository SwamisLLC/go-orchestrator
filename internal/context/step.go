package context

import (
	"time"
)

// Credentials represent the authentication details for a specific payment provider.
type Credentials struct {
	APIKey string
	// Other fields like secret, token, etc.
}

// StepExecutionContext is derived by Orchestrator for each PaymentStep.
type StepExecutionContext struct {
	TraceID             string      // Taken directly from TraceContext
	SpanID              string      // Current Span ID for this step
	StartTime           time.Time   // When this step's processing began
	RemainingBudgetMs   int64       // How many ms remain before overall SLA expires
	StepRetryPolicy     RetryPolicy // Subset of DomainContext.RetryPolicy, possibly modified
	ProviderCredentials Credentials // API key or token for the specific payment provider
	AttemptNumber       int         // Which attempt this is (1 for first, 2 for second, etc.)
	// any other per-step fields needed by Router/Adapter
}

// DeriveStepContext creates a StepExecutionContext from broader contexts.
func DeriveStepContext(tc TraceContext, dc DomainContext, providerName string, overallTimeoutBudgetMs int64, startTime time.Time, attemptNumber int) StepExecutionContext {
	// Calculate remaining budget based on overall budget and time elapsed since domain context creation.
	// This is a simplified calculation. A more robust one would track elapsed time more accurately.
	elapsedMs := time.Since(startTime).Milliseconds()
	remainingBudget := overallTimeoutBudgetMs - elapsedMs
	if remainingBudget < 0 {
		remainingBudget = 0
	}

	return StepExecutionContext{
		TraceID:           tc.TraceID,
		SpanID:            tc.NewSpan(), // Generate a new SpanID for this step
		StartTime:         time.Now(),
		RemainingBudgetMs: remainingBudget,
		StepRetryPolicy:   dc.RetryPolicy, // Use the domain's retry policy by default
		ProviderCredentials: Credentials{
			APIKey: dc.GetProviderAPIKey(providerName),
		},
		AttemptNumber:     attemptNumber,
	}
}
