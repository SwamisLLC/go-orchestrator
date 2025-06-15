package planbuilder_test

import (
	go_context "context" // Correct alias for standard context package
	"testing"
	"fmt" // For error message in mock

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go" // Added for DTO inspection

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/planbuilder" // This is the package under test
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
)

// MockMerchantConfigRepository is a mock implementation of MerchantConfigRepository.
// It must match the interface: Get(merchantID string) (context.MerchantConfig, error)
type MockMerchantConfigRepository struct {
	cfg context.MerchantConfig // Store as value
	err error
	id  string // Store ID to match against for this simple mock
}

func (m *MockMerchantConfigRepository) Get(merchantID string) (context.MerchantConfig, error) {
	if m.err != nil {
		return context.MerchantConfig{}, m.err
	}
	if m.id == merchantID { // Only return cfg if ID matches what was set up
		return m.cfg, nil
	}
	return context.MerchantConfig{}, fmt.Errorf("mock config not found for ID: %s (mock configured for %s)", merchantID, m.id)
}

// SetConfig is a helper for tests to set up the mock's state. Not part of the interface.
func (m *MockMerchantConfigRepository) SetConfig(cfg context.MerchantConfig, err error) {
	m.cfg = cfg
	m.id = cfg.ID // Store the ID for matching in Get
	m.err = err
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
	initialRequests := testutil.ToFloat64(planbuilder.GetPlanRequestsTotal())

	var initialMetricDTO dto.Metric
	var initialDurationCount uint64
	if err := planbuilder.GetPlanBuildDurationSeconds().Write(&initialMetricDTO); err == nil {
		if histo := initialMetricDTO.GetHistogram(); histo != nil {
			initialDurationCount = histo.GetSampleCount()
		} else {
			initialDurationCount = 0 // Default if histogram part of DTO is nil
		}
	} else {
		t.Logf("Warning: could not get initial histogram count: %v", err)
		initialDurationCount = 0 // Default if Write fails
	}

	mockRepo := &MockMerchantConfigRepository{}
	merchantID := "test-merchant-" + uuid.NewString()

	merchantCfg := context.MerchantConfig{
		ID:              merchantID,
		DefaultProvider: "mock-provider",
		DefaultCurrency: "USD",
		ProviderAPIKeys: map[string]string{"mock-provider": "pk_live_mock"},
		UserPrefs:       map[string]string{"merchant_tier": "gold"},
		DefaultTimeout:  context.TimeoutConfig{OverallBudgetMs: 1000, ProviderTimeoutMs: 600},
	}
	mockRepo.SetConfig(merchantCfg, nil) // Use the helper to set up the mock

	mockCompositeService := &MockCompositePaymentService{
		OptimizeFunc: func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
			return plan, nil
		},
	}

	pb := planbuilder.NewPlanBuilder(mockRepo, mockCompositeService) // mockRepo now implements the interface

	mockTraceCtx := context.NewTraceContext(go_context.Background())

	activeCfg, err := mockRepo.Get(merchantID)
	if err != nil {
		t.Fatalf("Failed to get merchant config for domain context setup: %v", err)
	}

	domainCtx := context.DomainContext{
		ActiveMerchantConfig: activeCfg,
		MerchantID:           merchantID,
	}

	extReq := &orchestratorexternalv1.ExternalRequest{
		MerchantId: merchantID,
		Amount:     1000,
		Currency:   "USD",
	}

	_, err = pb.Build(mockTraceCtx, domainCtx, extReq)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	finalRequests := testutil.ToFloat64(planbuilder.GetPlanRequestsTotal())
	if finalRequests != initialRequests+1 {
		t.Errorf("planRequestsTotal expected to increment by 1, got initial=%f, final=%f", initialRequests, finalRequests)
	}

	var finalMetricDTO dto.Metric
	var finalDurationCount uint64
	if err := planbuilder.GetPlanBuildDurationSeconds().Write(&finalMetricDTO); err == nil {
		if histo := finalMetricDTO.GetHistogram(); histo != nil {
			finalDurationCount = histo.GetSampleCount()
		} else {
			t.Fatalf("Histogram field in DTO is nil after build")
		}
	} else {
		t.Fatalf("Failed to write final histogram metric to DTO: %v", err)
	}

	if finalDurationCount != initialDurationCount+1 {
		t.Errorf("planBuildDurationSeconds observation count expected to increment by 1, got initial=%d, final=%d", initialDurationCount, finalDurationCount)
	}
}
