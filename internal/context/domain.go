package context

import (
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
)

// TimeoutConfig stores merchant SLA settings.
type TimeoutConfig struct {
	OverallBudgetMs int64 // Overall budget for the entire payment processing
	ProviderTimeoutMs int64 // Timeout for individual provider calls
}

// RetryPolicy stores merchant's global retry rules.
type RetryPolicy struct {
	MaxAttempts int
	// Could include backoff strategies, etc.
}

// MerchantConfig holds static configuration for a merchant.
// This is a simplified version. In a real system, this would be more complex
// and likely loaded from a database or configuration service.
type MerchantConfig struct {
	ID                  string
	PolicyVersion       int
	UserPrefs           map[string]string
	DefaultTimeout      TimeoutConfig
	DefaultRetryPolicy  RetryPolicy
	FeatureFlags        map[string]bool
	ProviderAPIKeys     map[string]string // e.g., "stripe" -> "sk_test_...", "adyen" -> "..."
	DefaultProvider     string
	DefaultCurrency     string
	// Other fields like fee schedules, routing rules etc.
}

// DomainContext carries purely business-relevant data for payment processing.
type DomainContext struct {
	MerchantID            string
	MerchantPolicyVersion int
	UserPreferences       map[string]string
	TimeoutConfig         TimeoutConfig
	RetryPolicy           RetryPolicy
	FeatureFlags          map[string]bool
	ActiveMerchantConfig  MerchantConfig // The resolved merchant configuration
}

// BuildDomainContext populates DomainContext from an ExternalRequest and MerchantConfig.
// This is a simplified builder. A real implementation would involve more complex logic,
// potentially fetching merchant configurations from a repository.
func BuildDomainContext(extReq *orchestratorexternalv1.ExternalRequest, merchantCfg MerchantConfig) (DomainContext, error) {
	// Basic validation or enrichment can happen here
	// For now, we directly map, assuming merchantCfg is pre-fetched and relevant.

	return DomainContext{
		MerchantID:            extReq.MerchantId,
		MerchantPolicyVersion: merchantCfg.PolicyVersion,
		UserPreferences:       merchantCfg.UserPrefs, // Could be merged with extReq.Metadata
		TimeoutConfig:         merchantCfg.DefaultTimeout,
		RetryPolicy:           merchantCfg.DefaultRetryPolicy,
		FeatureFlags:          merchantCfg.FeatureFlags,
		ActiveMerchantConfig:  merchantCfg,
	}, nil
}

// Accessor methods (examples)
func (d *DomainContext) GetFeatureFlag(key string) bool {
	if d.FeatureFlags == nil {
		return false
	}
	return d.FeatureFlags[key]
}

func (d *DomainContext) GetProviderAPIKey(providerName string) string {
	if d.ActiveMerchantConfig.ProviderAPIKeys == nil {
		return ""
	}
	return d.ActiveMerchantConfig.ProviderAPIKeys[providerName]
}
