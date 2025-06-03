package planbuilder

import (
	"fmt"
	"testing"

	"github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCompositePaymentService for testing
type MockCompositePaymentService struct {
	OptimizeFunc func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error)
}

func (m *MockCompositePaymentService) Optimize(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
	if m.OptimizeFunc != nil {
		return m.OptimizeFunc(domainCtx, plan)
	}
	// Default stub behavior: return the plan as is
	return plan, nil
}

func NewTestMerchantConfigRepository() *context.InMemoryMerchantConfigRepository {
    repo := context.NewInMemoryMerchantConfigRepository()
    repo.AddConfig(context.MerchantConfig{
        ID:              "merchant123",
        DefaultProvider: "stripe",
        DefaultCurrency: "USD", // This field is in MerchantConfig
        ProviderAPIKeys: map[string]string{"stripe": "sk_test_dummy"},
    })
    repo.AddConfig(context.MerchantConfig{
        ID:              "merchant456",
        DefaultProvider: "adyen",
        DefaultCurrency: "EUR",
        ProviderAPIKeys: map[string]string{"adyen": "ak_test_dummy"},
    })
    return repo
}


func TestNewPlanBuilder(t *testing.T) {
	repo := NewTestMerchantConfigRepository()
	mockCompositeSvc := &MockCompositePaymentService{}

	pb := NewPlanBuilder(repo, mockCompositeSvc)
	assert.NotNil(t, pb)
	assert.Equal(t, repo, pb.merchantRepo)
	assert.Equal(t, mockCompositeSvc, pb.compositeService)

	assert.Panics(t, func() { NewPlanBuilder(nil, mockCompositeSvc) }, "Should panic if repo is nil")
	assert.Panics(t, func() { NewPlanBuilder(repo, nil) }, "Should panic if composite service is nil")
}

func TestPlanBuilder_Build_Basic(t *testing.T) {
	repo := NewTestMerchantConfigRepository()
	mockCompositeSvc := &MockCompositePaymentService{
		OptimizeFunc: func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
			// Simple pass-through for this test
			return plan, nil
		},
	}
	pb := NewPlanBuilder(repo, mockCompositeSvc)

	merchantCfg, _ := repo.Get("merchant123")
	domainCtx := context.DomainContext{
		MerchantID:           "merchant123",
		ActiveMerchantConfig: merchantCfg,
	}
	extReq := &orchestratorexternalv1.ExternalRequest{
		MerchantId: "merchant123",
		Amount:     5000,
		Currency:   "USD",
	}

	plan, err := pb.Build(domainCtx, extReq)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotEmpty(t, plan.PlanId)
	require.Len(t, plan.Steps, 1)

	step := plan.Steps[0]
	assert.NotEmpty(t, step.StepId)
	assert.Equal(t, "stripe", step.ProviderName)
	assert.Equal(t, int64(5000), step.Amount)
	assert.Equal(t, "USD", step.Currency)
	assert.False(t, step.IsFanOut)
	assert.NotNil(t, step.Metadata)
	assert.NotNil(t, step.ProviderPayload)
}

func TestPlanBuilder_Build_NilExternalRequest(t *testing.T) {
	repo := NewTestMerchantConfigRepository()
	mockCompositeSvc := &MockCompositePaymentService{}
	pb := NewPlanBuilder(repo, mockCompositeSvc)

	merchantCfg, _ := repo.Get("merchant123")
	domainCtx := context.DomainContext{
		MerchantID:           "merchant123",
		ActiveMerchantConfig: merchantCfg,
	}

	_, err := pb.Build(domainCtx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "external request cannot be nil")
}

func TestPlanBuilder_Build_CompositeServiceError(t *testing.T) {
	repo := NewTestMerchantConfigRepository()
	mockCompositeSvc := &MockCompositePaymentService{
		OptimizeFunc: func(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
			return nil, fmt.Errorf("composite error")
		},
	}
	pb := NewPlanBuilder(repo, mockCompositeSvc)

	merchantCfg, _ := repo.Get("merchant123")
	domainCtx := context.DomainContext{
		MerchantID:           "merchant123",
		ActiveMerchantConfig: merchantCfg,
	}
	extReq := &orchestratorexternalv1.ExternalRequest{
		MerchantId: "merchant123",
		Amount:     1000,
		Currency:   "EUR",
	}

	_, err := pb.Build(domainCtx, extReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to optimize payment plan")
	assert.Contains(t, err.Error(), "composite error")
}
