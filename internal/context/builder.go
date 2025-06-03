package context

import (
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	"fmt"
)

// MerchantConfigRepository defines an interface for fetching merchant configurations.
// This allows for different implementations (e.g., in-memory, database).
type MerchantConfigRepository interface {
	Get(merchantID string) (MerchantConfig, error)
}

// InMemoryMerchantConfigRepository is a simple in-memory implementation for testing.
type InMemoryMerchantConfigRepository struct {
	configs map[string]MerchantConfig
}

// NewInMemoryMerchantConfigRepository creates a new in-memory repository.
func NewInMemoryMerchantConfigRepository() *InMemoryMerchantConfigRepository {
	return &InMemoryMerchantConfigRepository{
		configs: make(map[string]MerchantConfig),
	}
}

// AddConfig adds a merchant configuration to the repository.
func (r *InMemoryMerchantConfigRepository) AddConfig(config MerchantConfig) {
	r.configs[config.ID] = config
}

// Get fetches a merchant configuration by ID.
func (r *InMemoryMerchantConfigRepository) Get(merchantID string) (MerchantConfig, error) {
	config, ok := r.configs[merchantID]
	if !ok {
		return MerchantConfig{}, fmt.Errorf("merchant config not found for ID: %s", merchantID)
	}
	return config, nil
}


// ContextBuilder is responsible for creating TraceContext and DomainContext.
type ContextBuilder struct {
	merchantRepo MerchantConfigRepository
}

// NewContextBuilder creates a new ContextBuilder.
func NewContextBuilder(repo MerchantConfigRepository) *ContextBuilder {
	return &ContextBuilder{
		merchantRepo: repo,
	}
}

// BuildContexts creates TraceContext and DomainContext from an external request.
func (cb *ContextBuilder) BuildContexts(extReq *orchestratorexternalv1.ExternalRequest) (TraceContext, DomainContext, error) {
	if extReq == nil {
		return TraceContext{}, DomainContext{}, fmt.Errorf("external request cannot be nil")
	}

	traceCtx := NewTraceContext() // Initialize TraceContext

	merchantCfg, err := cb.merchantRepo.Get(extReq.MerchantId)
	if err != nil {
		return traceCtx, DomainContext{}, fmt.Errorf("failed to get merchant config: %w", err)
	}

	domainCtx, err := BuildDomainContext(extReq, merchantCfg)
	if err != nil {
		return traceCtx, DomainContext{}, fmt.Errorf("failed to build domain context: %w", err)
	}

	return traceCtx, domainCtx, nil
}
