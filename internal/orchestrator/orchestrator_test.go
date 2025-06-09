package orchestrator

import (
	"testing"
	// "time" // Removed

	"github.com/yourorg/payment-orchestrator/internal/context"
	// "github.com/yourorg/payment-orchestrator/internal/policy" // Will use mock interface
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	// "github.com/google/uuid" // Removed
)

// MockRouter is a mock implementation of RouterInterface
type MockRouter struct {
	mock.Mock
}

func (m *MockRouter) ExecuteStep(ctx context.StepExecutionContext, step *internalv1.PaymentStep) (*internalv1.StepResult, error) {
	args := m.Called(ctx, step)
	res, _ := args.Get(0).(*internalv1.StepResult)
	return res, args.Error(1)
}

// MockPolicyEnforcer is a mock implementation of PolicyEnforcerInterface
type MockPolicyEnforcer struct {
	mock.Mock
}

func (m *MockPolicyEnforcer) Evaluate(ctx context.StepExecutionContext, currentStep *internalv1.PaymentStep, stepResult *internalv1.StepResult) (bool, error) {
	args := m.Called(ctx, currentStep, stepResult)
	return args.Bool(0), args.Error(1)
}

// MockMerchantConfigRepository
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
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMcr := new(MockMerchantConfigRepository)

	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMcr)
	assert.NotNil(t, orc)
	assert.Equal(t, mockRouter, orc.router)
	assert.Equal(t, mockPolicyEnforcer, orc.policyEnforcer)
	assert.Equal(t, mockMcr, orc.merchantConfigRepo)

	assert.Panics(t, func() { NewOrchestrator(nil, mockPolicyEnforcer, mockMcr) }, "Should panic if router is nil")
	assert.Panics(t, func() { NewOrchestrator(mockRouter, nil, mockMcr) }, "Should panic if policy enforcer is nil")
	assert.Panics(t, func() { NewOrchestrator(mockRouter, mockPolicyEnforcer, nil) }, "Should panic if merchant config repo is nil")
}

func TestOrchestrator_Execute_EmptyPlan(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMcr := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMcr)

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

func TestOrchestrator_Execute_SingleStepPlan_Success(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := &MockMerchantConfigRepository{
		cfg: context.MerchantConfig{DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey"}},
	}
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID:           "merchant1",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 5000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "step1", ProviderName: "stripe", Amount: 1000, Currency: "USD", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-single-step",
		Steps:  []*internalv1.PaymentStep{step1},
	}

	// Mock expectations
	expectedStepResult1 := &internalv1.StepResult{StepId: "step1", Success: true, ProviderName: "stripe"}
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(false, nil).Once() // allowRetry = false

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.PaymentID)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 1)
	assert.Equal(t, expectedStepResult1, result.StepResults[0])

	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_MultiStepPlan_AllSuccess(t *testing.T) { // Renamed from _Stubbed
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := &MockMerchantConfigRepository{
		cfg: context.MerchantConfig{DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey", "adyen": "testkey2"}},
	}
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID:           "merchant2",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	step2 := &internalv1.PaymentStep{StepId: "s2", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-multi-step",
		Steps:  []*internalv1.PaymentStep{step1, step2},
	}

	// Mock expectations
	expectedStepResult1 := &internalv1.StepResult{StepId: "s1", Success: true, ProviderName: "stripe"}
	expectedStepResult2 := &internalv1.StepResult{StepId: "s2", Success: true, ProviderName: "adyen"}
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(false, nil).Once()
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.StepExecutionContext"), step2).Return(expectedStepResult2, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step2, expectedStepResult2).Return(false, nil).Once()


	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 2)
	assert.Equal(t, expectedStepResult1, result.StepResults[0])
	assert.Equal(t, expectedStepResult2, result.StepResults[1])

	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_HandlesNilStepInPlan(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := &MockMerchantConfigRepository{
		cfg: context.MerchantConfig{DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey", "adyen": "testkey2"}},
	}
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext()
	domainCtx := context.DomainContext{
		MerchantID:           "merchant-nil-step",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: mockMCR.cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	step3 := &internalv1.PaymentStep{StepId: "s3", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-with-nil-step",
		Steps:  []*internalv1.PaymentStep{step1, nil, step3},
	}

	// Mock expectations for non-nil steps
	expectedStepResult1 := &internalv1.StepResult{StepId: "s1", Success: true, ProviderName: "stripe"}
	expectedStepResult3 := &internalv1.StepResult{StepId: "s3", Success: true, ProviderName: "adyen"}

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(false, nil).Once()
	// No router/policy calls for nil step
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.StepExecutionContext"), step3).Return(expectedStepResult3, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step3, expectedStepResult3).Return(false, nil).Once()


	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err) // The main Execute function doesn't return an error for this specific case of nil step in plan
	assert.Equal(t, "FAILURE", result.Status, "Overall status should be FAILURE due to nil step")
	require.Len(t, result.StepResults, 3)

	assert.Equal(t, expectedStepResult1, result.StepResults[0]) // s1 is processed

	// Check the result for the nil step
	assert.False(t, result.StepResults[1].Success, "Nil step should result in a failure StepResult")
	assert.Equal(t, "NIL_STEP", result.StepResults[1].ErrorCode)
	assert.Equal(t, "nil-step-1", result.StepResults[1].StepId) // ID generated by orchestrator

	assert.Equal(t, expectedStepResult3, result.StepResults[2]) // s3 is processed after nil

	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}
