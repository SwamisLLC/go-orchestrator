package planbuilder_test

import (
	"context as go_context" // Alias to avoid conflict with internal/context
	"testing"
	// "time" // Not directly needed now with simplified MerchantConfig

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/planbuilder" // This is the package under test
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
)

// MockMerchantConfigRepository is a mock implementation of MerchantConfigRepository.
type MockMerchantConfigRepository struct {
	cfg *context.MerchantConfig
	err error
}

func (m *MockMerchantConfigRepository) GetConfig(merchantID string) (*context.MerchantConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Ensure cfg is not nil before checking ID.
	if m.cfg != nil && m.cfg.ID == merchantID {
		return m.cfg, nil
	}
	// If no specific config matches, return the stored cfg (which might be nil or a default)
	// or an error if that's the desired behavior for "not found".
	// For this test, if GetConfig is called with an ID that wasn't AddConfig'd,
	// it might return a nil cfg if m.cfg wasn't set for *that* ID.
	// The test specifically sets it up so it should be found.
	return m.cfg, nil
}

func (m *MockMerchantConfigRepository) AddConfig(cfg context.MerchantConfig) error {
	m.cfg = &cfg
	return nil
}

// MockCompositePaymentService is a mock implementation of CompositePaymentService.
type MockCompositePaymentService struct {
	OptimizeFunc func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error)
}

func (m *MockCompositePaymentService) Optimize(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
	if m.OptimizeFunc != nil {
		return m.OptimizeFunc(domainCtx, plan)
	}
	return plan, nil // Default behavior: return the plan as is
}

func TestPlanBuilder_Metrics_SuccessfulBuild(t *testing.T) {
	// Note on global metrics: These tests rely on globally registered Prometheus metrics via promauto.
	// This means state can persist between test runs or if tests are run in parallel.
	// The approach here is to measure the increment (current - initial) to mitigate this.
	// A more robust solution for hermetic tests would involve a local Prometheus registry.

	// Get initial metric values
	initialRequests := testutil.ToFloat64(planbuilder.GetPlanRequestsTotal())
	initialDurationCount := testutil.CollectAndGetCount(planbuilder.GetPlanBuildDurationSeconds())

	// Setup PlanBuilder and its dependencies
	mockRepo := &MockMerchantConfigRepository{}
	merchantID := "test-merchant-" + uuid.NewString() // Unique merchant ID for the test

	// Prepare MerchantConfig and add it to the mock repository
	merchantCfg := context.MerchantConfig{
		ID:              merchantID,
		DefaultProvider: "mock-provider",
		DefaultCurrency: "USD",
		ProviderAPIKeys: map[string]string{"mock-provider": "pk_live_mock"},
		UserPrefs:       map[string]string{"merchant_tier": "gold"},
		DefaultTimeout:  context.TimeoutConfig{OverallBudgetMs: 1000, ProviderTimeoutMs: 600},
	}
	if err := mockRepo.AddConfig(merchantCfg); err != nil {
		t.Fatalf("Failed to add merchant config to mock repo: %v", err)
	}


	mockCompositeService := &MockCompositePaymentService{
		OptimizeFunc: func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
			return plan, nil // Simple pass-through
		},
	}

	pb := planbuilder.NewPlanBuilder(mockRepo, mockCompositeService)

	// Prepare contexts and request
	// Use go_context.Background() for the actual context.Context
	traceCtx := context.NewTraceContext(go_context.Background(), "test-trace-"+uuid.NewString())

	// Ensure ActiveMerchantConfig in DomainContext is correctly fetched for the merchantID
	activeCfg, err := mockRepo.GetConfig(merchantID)
	if err != nil {
		t.Fatalf("Failed to get merchant config for domain context: %v", err)
	}
	if activeCfg == nil {
		t.Fatalf("ActiveMerchantConfig is nil in DomainContext setup, merchantID: %s", merchantID)
	}


	domainCtx := context.DomainContext{
		TraceContext:         traceCtx,
		ActiveMerchantConfig: activeCfg,
		// Other fields can be zero/default if not directly used by Build's metric logic path
	}

	extReq := &orchestratorexternalv1.ExternalRequest{
		MerchantId: merchantID, // Ensure this matches the config
		Amount:     1000,
		Currency:   "USD",
	}

	// Call the Build method
	_, err = pb.Build(domainCtx, extReq)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Assert metrics increment
	// 1. planRequestsTotal
	finalRequests := testutil.ToFloat64(planbuilder.GetPlanRequestsTotal())
	if finalRequests != initialRequests+1 {
		t.Errorf("planRequestsTotal expected to increment by 1, got initial=%f, final=%f", initialRequests, finalRequests)
	}

	// 2. planBuildDurationSeconds (count of observations)
	finalDurationCount := testutil.CollectAndGetCount(planbuilder.GetPlanBuildDurationSeconds())
	if finalDurationCount != initialDurationCount+1 {
		t.Errorf("planBuildDurationSeconds observation count expected to increment by 1, got initial=%d, final=%d", initialDurationCount, finalDurationCount)
	}
}
