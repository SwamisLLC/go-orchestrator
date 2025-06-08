package context

import (
	"time"
)

type Credentials struct {
	APIKey string
}

type StepExecutionContext struct {
	TraceID             string
	SpanID              string
	StartTime           time.Time
	RemainingBudgetMs   int64
	StepRetryPolicy     RetryPolicy
	ProviderCredentials Credentials

	// Fields for policy evaluation (as per todo.md for Chunk 14)
	AmountCents  int64       // Amount for the current step
	Currency     string      // Currency for the current step
	Region       string      // Example: "EU", "US" (Could come from DomainContext.MerchantConfig or enriched data)
	MerchantTier string      // Example: "gold", "silver" (Could come from DomainContext.MerchantConfig)
	FraudScore   float64     // Example: 0.0 to 1.0 (Could be from an external fraud service call prior to policy)
}

// DeriveStepContext populates StepExecutionContext.
// New fields (Region, MerchantTier, FraudScore) are populated from DomainContext.UserPreferences as examples.
// FraudScore would typically be set by an enrichment step before policy evaluation; here it defaults to 0 if not in UserPrefs.
func DeriveStepContext(tc TraceContext, dc DomainContext, providerName string, stepAmountCents int64, stepCurrency string, overallTimeoutBudgetMs int64, startTime time.Time) StepExecutionContext {
	elapsedMs := time.Since(startTime).Milliseconds() // Note: startTime is the overall process start, not step start.
	remainingBudget := overallTimeoutBudgetMs - elapsedMs
	if remainingBudget < 0 {
		remainingBudget = 0
	}

	// Example population for Region and MerchantTier from UserPreferences in MerchantConfig
	// A real system might have more structured fields in MerchantConfig or other sources.
	region := ""
	if dc.ActiveMerchantConfig.UserPrefs != nil {
		region = dc.ActiveMerchantConfig.UserPrefs["region"]
	}
	merchantTier := ""
	if dc.ActiveMerchantConfig.UserPrefs != nil {
		merchantTier = dc.ActiveMerchantConfig.UserPrefs["tier"]
	}
	// FraudScore is not typically in UserPrefs; it would come from an enrichment service.
	// For this example, we'll leave it as its zero value (0.0) unless explicitly set later.

	return StepExecutionContext{
		TraceID:           tc.TraceID,
		SpanID:            tc.NewSpan(), // Generate a new SpanID for this specific step attempt
		StartTime:         time.Now(),   // Record the actual start time of this step's processing
		RemainingBudgetMs: remainingBudget,
		StepRetryPolicy:   dc.RetryPolicy,
		ProviderCredentials: Credentials{
			APIKey: dc.GetProviderAPIKey(providerName),
		},
		AmountCents:  stepAmountCents, // Populate from step
		Currency:     stepCurrency,    // Populate from step
		Region:       region,          // Example from DomainContext.ActiveMerchantConfig.UserPrefs
		MerchantTier: merchantTier,      // Example from DomainContext.ActiveMerchantConfig.UserPrefs
		// FraudScore will be 0.0 unless set by an external update to StepExecutionContext instance
	}
}
