package orchestrator

import (
	"errors" // Ensure errors is imported
	"testing"
	go_std_context "context" // Aliased import for standard context

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy" // Import for policy.PolicyDecision
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRouter is a mock implementation of RouterInterface
type MockRouter struct {
	mock.Mock
}

// Updated signature to match RouterInterface
func (m *MockRouter) ExecuteStep(traceCtx context.TraceContext, stepCtx context.StepExecutionContext, step *internalv1.PaymentStep) (*internalv1.StepResult, error) {
	args := m.Called(traceCtx, stepCtx, step) // Add traceCtx to Called
	res, _ := args.Get(0).(*internalv1.StepResult)
	return res, args.Error(1)
}

// MockPolicyEnforcer is a mock implementation of PolicyEnforcerInterface
type MockPolicyEnforcer struct {
	mock.Mock
}

// Updated signature to match PolicyEnforcerInterface
func (m *MockPolicyEnforcer) Evaluate(domainCtx context.DomainContext, stepCtx context.StepExecutionContext, currentStep *internalv1.PaymentStep, stepResult *internalv1.StepResult) (policy.PolicyDecision, error) {
	args := m.Called(domainCtx, stepCtx, currentStep, stepResult) // Add domainCtx
	pd, ok := args.Get(0).(policy.PolicyDecision)
	if !ok && args.Get(0) != nil {
		panic("mock: argument 0 is not policy.PolicyDecision")
	}
	return pd, args.Error(1)
}

// MockMerchantConfigRepository
type MockMerchantConfigRepository struct {
	mock.Mock
}

func (m *MockMerchantConfigRepository) Get(merchantID string) (context.MerchantConfig, error) {
	args := m.Called(merchantID)
	cfg, _ := args.Get(0).(context.MerchantConfig)
	return cfg, args.Error(1)
}
func (m *MockMerchantConfigRepository) AddConfig(config context.MerchantConfig) error {
	args := m.Called(config)
	return args.Error(0)
}


func TestNewOrchestrator(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMcr := new(MockMerchantConfigRepository)

	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMcr)
	assert.NotNil(t, orc)

	assert.Panics(t, func() { NewOrchestrator(nil, mockPolicyEnforcer, mockMcr) }, "Should panic if router is nil")
	assert.Panics(t, func() { NewOrchestrator(mockRouter, nil, mockMcr) }, "Should panic if policy enforcer is nil")
	assert.Panics(t, func() { NewOrchestrator(mockRouter, mockPolicyEnforcer, nil) }, "Should panic if merchant config repo is nil")
}

func TestOrchestrator_Execute_EmptyPlan(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	domainCtx := context.DomainContext{}

	result, err := orc.Execute(traceCtx, nil, domainCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan cannot be empty or nil")
	assert.Equal(t, "FAILURE", result.Status)
	assert.Equal(t, "Plan is empty or nil", result.FailureReason)

	emptyPlan := &internalv1.PaymentPlan{PlanId: "plan123", Steps: []*internalv1.PaymentStep{}}
	result, err = orc.Execute(traceCtx, emptyPlan, domainCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan cannot be empty or nil")
	assert.Equal(t, "FAILURE", result.Status)
}

func TestOrchestrator_Execute_SingleStepPlan_Success(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID:"merchant1", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey"}}
	domainCtx := context.DomainContext{
		MerchantID:           "merchant1",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 5000},
		ActiveMerchantConfig: cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "step1", ProviderName: "stripe", Amount: 1000, Currency: "USD", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-single-step",
		Steps:  []*internalv1.PaymentStep{step1},
	}

	mockMCR.On("Get", "merchant1").Return(cfg, nil)
	expectedStepResult1 := &internalv1.StepResult{StepId: "step1", Success: true, ProviderName: "stripe"}
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()

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

func TestOrchestrator_Execute_MultiStepPlan_AllSuccess(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID:"merchant2", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey", "adyen": "testkey2"}}
	domainCtx := context.DomainContext{
		MerchantID:           "merchant2",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	step2 := &internalv1.PaymentStep{StepId: "s2", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-multi-step",
		Steps:  []*internalv1.PaymentStep{step1, step2},
	}

	mockMCR.On("Get", "merchant2").Return(cfg, nil)
	expectedStepResult1 := &internalv1.StepResult{StepId: "s1", Success: true, ProviderName: "stripe"}
	expectedStepResult2 := &internalv1.StepResult{StepId: "s2", Success: true, ProviderName: "adyen"}
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step2).Return(expectedStepResult2, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step2, expectedStepResult2).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()

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
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID:"merchant-nil-step", DefaultProvider: "stripe", ProviderAPIKeys: map[string]string{"stripe": "testkey", "adyen": "testkey2"}}
	domainCtx := context.DomainContext{
		MerchantID:           "merchant-nil-step",
		TimeoutConfig:        context.TimeoutConfig{OverallBudgetMs: 10000},
		ActiveMerchantConfig: cfg,
	}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 500, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	step3 := &internalv1.PaymentStep{StepId: "s3", ProviderName: "adyen", Amount: 700, Currency: "EUR", Metadata: make(map[string]string), ProviderPayload: make(map[string]string)}
	plan := &internalv1.PaymentPlan{
		PlanId: "plan-with-nil-step",
		Steps:  []*internalv1.PaymentStep{step1, nil, step3},
	}

	mockMCR.On("Get", "merchant-nil-step").Return(cfg, nil)
	expectedStepResult1 := &internalv1.StepResult{StepId: "s1", Success: true, ProviderName: "stripe"}
	expectedStepResult3 := &internalv1.StepResult{StepId: "s3", Success: true, ProviderName: "adyen"}

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step3).Return(expectedStepResult3, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step3, expectedStepResult3).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status, "Overall status should be FAILURE due to nil step")
	require.Len(t, result.StepResults, 3)
	assert.Equal(t, expectedStepResult1, result.StepResults[0])
	assert.False(t, result.StepResults[1].Success, "Nil step should result in a failure StepResult")
	assert.Equal(t, "NIL_STEP", result.StepResults[1].ErrorCode)
	assert.Equal(t, "nil-step-1", result.StepResults[1].StepId)
	assert.Equal(t, expectedStepResult3, result.StepResults[2])
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_RouterReturnsCriticalError(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantError", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantError", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-router-crit-err", Steps: []*internalv1.PaymentStep{step1}}

	mockMCR.On("Get", "merchantError").Return(cfg, nil)
	routerErr := errors.New("router critical failure")
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(nil, routerErr).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, mock.MatchedBy(func(sr *internalv1.StepResult) bool {
		return sr.GetStepId() == "s1" && !sr.GetSuccess() && sr.GetErrorCode() == "ORCHESTRATOR_ROUTER_CRITICAL_ERROR" && sr.GetErrorMessage() == routerErr.Error()
	})).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "FAIL"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err, "Orchestrator.Execute itself should not error for this case")
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1)
	assert.False(t, result.StepResults[0].GetSuccess())
	assert.Equal(t, "ORCHESTRATOR_ROUTER_CRITICAL_ERROR", result.StepResults[0].GetErrorCode())
	assert.Equal(t, routerErr.Error(), result.StepResults[0].GetErrorMessage())
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_RouterReturnsErrorWithPartialResult(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantError2", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantError2", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-router-partial-err", Steps: []*internalv1.PaymentStep{step1}}

	mockMCR.On("Get", "merchantError2").Return(cfg, nil)
	routerErr := errors.New("router partial failure")
	partialResultFromRouter := &internalv1.StepResult{StepId: "s1", Success: false, ProviderName: "p1", ErrorCode: "", ErrorMessage: ""}

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(partialResultFromRouter, routerErr).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, mock.MatchedBy(func(sr *internalv1.StepResult) bool {
		return sr.GetStepId() == "s1" && !sr.GetSuccess() && sr.GetErrorCode() == "ROUTER_ERROR_WITH_RESULT" && sr.GetErrorMessage() == routerErr.Error()
	})).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "FAIL"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1)
	rs := result.StepResults[0]
	assert.False(t, rs.GetSuccess())
	assert.Equal(t, "ROUTER_ERROR_WITH_RESULT", rs.GetErrorCode())
	assert.Equal(t, routerErr.Error(), rs.GetErrorMessage())
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_RouterReturnsNilResultAndNilError(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantNilNil", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantNilNil", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-router-nil-nil", Steps: []*internalv1.PaymentStep{step1}}

	mockMCR.On("Get", "merchantNilNil").Return(cfg, nil)
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(nil, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, mock.MatchedBy(func(sr *internalv1.StepResult) bool {
		return sr.GetStepId() == "s1" && !sr.GetSuccess() && sr.GetErrorCode() == "ORCHESTRATOR_NIL_ROUTER_RESULT"
	})).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "FAIL"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1)
	rs := result.StepResults[0]
	assert.False(t, rs.GetSuccess())
	assert.Equal(t, "ORCHESTRATOR_NIL_ROUTER_RESULT", rs.GetErrorCode())
	assert.Contains(t, rs.GetErrorMessage(), "Router returned nil result and nil error")
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_PolicyEnforcerReturnsError(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantPolicyErr", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantPolicyErr", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-policy-err", Steps: []*internalv1.PaymentStep{step1}}

	mockMCR.On("Get", "merchantPolicyErr").Return(cfg, nil)
	stepResultFromRouter := &internalv1.StepResult{StepId: "s1", Success: true, ProviderName: "p1", ErrorMessage: "Original success"}
	policyErr := errors.New("policy evaluation failed")

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(stepResultFromRouter, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, stepResultFromRouter).Return(policy.PolicyDecision{}, policyErr).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1)
	rs := result.StepResults[0]
	assert.False(t, rs.GetSuccess())
	assert.Equal(t, "POLICY_EVALUATION_ERROR", rs.GetErrorCode())
	assert.Contains(t, rs.GetErrorMessage(), "OriginalMsg: [Original success]")
	assert.Contains(t, rs.GetErrorMessage(), policyErr.Error())
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_StepFails_PolicyDeniesRetry_StopsExecution(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantNoRetry", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantNoRetry", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	step2 := &internalv1.PaymentStep{StepId: "s2", ProviderName: "p2"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-no-retry-stop", Steps: []*internalv1.PaymentStep{step1, step2}}

	mockMCR.On("Get", "merchantNoRetry").Return(cfg, nil)
	failedStepResult := &internalv1.StepResult{StepId: "s1", Success: false, ProviderName: "p1", ErrorCode: "GENERIC_DECLINE"}

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(failedStepResult, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, failedStepResult).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "FAIL", Reason: "No retry policy"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1)
	rs1 := result.StepResults[0]
	assert.False(t, rs1.GetSuccess())
	assert.Equal(t, "GENERIC_DECLINE", rs1.GetErrorCode())
	assert.Equal(t, "No retry policy", rs1.GetDetails()["policy_reason"])
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestrator_Execute_StepFails_PolicyAllowsRetry_ContinuesExecution(t *testing.T) {
	mockRouter := new(MockRouter)
	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMCR := new(MockMerchantConfigRepository)
	orc := NewOrchestrator(mockRouter, mockPolicyEnforcer, mockMCR)

	traceCtx := context.NewTraceContext(go_std_context.Background())
	cfg := context.MerchantConfig{ID: "merchantAllowRetry", DefaultProvider: "p1"}
	domainCtx := context.DomainContext{MerchantID: "merchantAllowRetry", TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 1000}, ActiveMerchantConfig: cfg}
	step1 := &internalv1.PaymentStep{StepId: "s1", ProviderName: "p1"}
	step2 := &internalv1.PaymentStep{StepId: "s2", ProviderName: "p2"}
	plan := &internalv1.PaymentPlan{PlanId: "plan-allow-retry-continue", Steps: []*internalv1.PaymentStep{step1, step2}}

	mockMCR.On("Get", "merchantAllowRetry").Return(cfg, nil)
	failedStep1Result := &internalv1.StepResult{StepId: "s1", Success: false, ProviderName: "p1", ErrorCode: "SOFT_DECLINE"}
	successStep2Result := &internalv1.StepResult{StepId: "s2", Success: true, ProviderName: "p2"}

	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step1).Return(failedStep1Result, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step1, failedStep1Result).Return(policy.PolicyDecision{AllowRetry: true, NextAction: "RETRY_LATER", Reason: "Soft decline, retry allowed"}, nil).Once()
	mockRouter.On("ExecuteStep", mock.AnythingOfType("context.TraceContext"), mock.AnythingOfType("context.StepExecutionContext"), step2).Return(successStep2Result, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.DomainContext"), mock.AnythingOfType("context.StepExecutionContext"), step2, successStep2Result).Return(policy.PolicyDecision{AllowRetry: false, NextAction: "PROCEED"}, nil).Once()

	result, err := orc.Execute(traceCtx, plan, domainCtx)
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 2)

	rs1 := result.StepResults[0]
	assert.False(t, rs1.GetSuccess())
	assert.Equal(t, "SOFT_DECLINE", rs1.GetErrorCode())
	assert.Equal(t, "Soft decline, retry allowed", rs1.GetDetails()["policy_reason"])
	assert.Equal(t, "true", rs1.GetDetails()["policy_allow_retry"])

	rs2 := result.StepResults[1]
	assert.True(t, rs2.GetSuccess())
	mockRouter.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

// Helper to ensure errors package is available if TestOrchestrator_Execute_RouterReturnsCriticalError or other new tests are run standalone
var _ = errors.New("")
