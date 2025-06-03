package orchestrator

import (
	"testing"
	// "time" // Removed

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// "github.com/google/uuid" // Removed
)

// MockMerchantConfigRepository (MockPolicyEnforcer removed as we'll use the real one for now)
type MockMerchantConfigRepository struct {
    cfg context.MerchantConfig
    err error
}
func (m *MockMerchantConfigRepository) Get(merchantID string) (context.MerchantConfig, error) {
    if m.err != nil {
        return context.MerchantConfig{}, m.err
    }
    cfgWithID := m.cfg
    cfgWithID.ID = merchantID // Ensure the returned config has the requested ID
    return cfgWithID, nil
}
func (m *MockMerchantConfigRepository) AddConfig(config context.MerchantConfig) { /* not used in this test */ }


func TestNewOrchestrator(t *testing.T) {
	pe := policy.NewPaymentPolicyEnforcer() // Use actual policy enforcer
	mcr := &MockMerchantConfigRepository{}
	orc := NewOrchestrator(pe, mcr)
	assert.NotNil(t, orc)
	assert.Equal(t, pe, orc.policyEnforcer) // This will compare pointers
	assert.Equal(t, mcr, orc.merchantConfigRepo)

	assert.Panics(t, func() { NewOrchestrator(nil, mcr) }, "Should panic if policy enforcer is nil")
	assert.Panics(t, func() { NewOrchestrator(pe, nil) }, "Should panic if merchant config repo is nil")
}

func TestOrchestrator_Execute_EmptyPlan(t *testing.T) {
	orc := NewOrchestrator(policy.NewPaymentPolicyEnforcer(), &MockMerchantConfigRepository{})
	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{}

	// Test with nil plan
	result, err := orc.Execute(traceCtx, nil, domainCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan cannot be empty or nil")
	assert.Equal(t, "FAILURE", result.Status)
	assert.Equal(t, "Plan is empty or nil", result.FailureReason)


	// Test with empty steps
	emptyPlan := &internalv1.PaymentPlan{PlanId: "plan123", Steps: []*internalv1.PaymentStep{}}
	result, err = orc.Execute(traceCtx, emptyPlan, domainCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan cannot be empty or nil")
	assert.Equal(t, "FAILURE", result.Status)
}

func TestOrchestrator_Execute_SingleStepPlan_Stubbed(t *testing.T) {
	realPE := policy.NewPaymentPolicyEnforcer() // Use actual policy enforcer
	mockMCR := &MockMerchantConfigRepository{
	    cfg: context.MerchantConfig{DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe":"testkey"}},
	}
	orc := NewOrchestrator(realPE, mockMCR)

	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID: "merchant1",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 5000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-single-step",
		Steps: []*internalv1.PaymentStep{
			{StepId: "step1", ProviderName: "stripe", Amount: 1000, Currency: "USD", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)},
		},
	}

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, "SUCCESS", result.Status, "Overall status should be SUCCESS for stubbed execution")
	require.Len(t, result.StepResults, 1)

	stepResult1 := result.StepResults[0]
	assert.Equal(t, "step1", stepResult1.StepId)
	assert.True(t, stepResult1.Success, "Stubbed step should be successful")
	assert.Equal(t, "stripe", stepResult1.ProviderName)
	assert.Empty(t, stepResult1.ErrorCode)
	assert.NotNil(t, stepResult1.Details)
	assert.Equal(t, "success", stepResult1.Details["stubbed_result"])
}

func TestOrchestrator_Execute_MultiStepPlan_Stubbed(t *testing.T) {
	realPE := policy.NewPaymentPolicyEnforcer() // Use actual policy enforcer
	mockMCR := &MockMerchantConfigRepository{
	    cfg: context.MerchantConfig{DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe":"testkey", "adyen":"testkey2"}},
	}
	orc := NewOrchestrator(realPE, mockMCR)
	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID: "merchant2",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-multi-step",
		Steps: []*internalv1.PaymentStep{
			{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)},
			{StepId: "s2", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)},
		},
	}

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 2)

	assert.Equal(t, "s1", result.StepResults[0].StepId)
	assert.True(t, result.StepResults[0].Success)
	assert.Equal(t, "stripe", result.StepResults[0].ProviderName)
	assert.NotNil(t, result.StepResults[0].Details)

	assert.Equal(t, "s2", result.StepResults[1].StepId)
	assert.True(t, result.StepResults[1].Success)
	assert.Equal(t, "adyen", result.StepResults[1].ProviderName)
	assert.NotNil(t, result.StepResults[1].Details)
}

func TestOrchestrator_Execute_HandlesNilStepInPlan(t *testing.T) {
	realPE := policy.NewPaymentPolicyEnforcer() // Use actual policy enforcer
	mockMCR := &MockMerchantConfigRepository{
	    cfg: context.MerchantConfig{DefaultProvider: "stripe"},
	}
	orc := NewOrchestrator(realPE, mockMCR)
	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID: "merchant-nil-step",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-with-nil-step",
		Steps: []*internalv1.PaymentStep{
			{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)},
			nil, // A nil step
			{StepId: "s3", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)},
		},
	}

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err) // The main Execute function doesn't return an error for this case
	assert.Equal(t, "FAILURE", result.Status, "Overall status should be FAILURE due to nil step")
	require.Len(t, result.StepResults, 3)

	assert.Equal(t, "s1", result.StepResults[0].StepId)
	assert.True(t, result.StepResults[0].Success)

	assert.False(t, result.StepResults[1].Success, "Nil step should result in a failure StepResult")
	assert.Equal(t, "NIL_STEP", result.StepResults[1].ErrorCode)
	assert.Equal(t, "nil-step-1", result.StepResults[1].StepId)


	assert.Equal(t, "s3", result.StepResults[2].StepId)
	assert.True(t, result.StepResults[2].Success) // Subsequent steps are still stubbed as success
}
