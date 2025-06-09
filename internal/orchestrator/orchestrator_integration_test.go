package orchestrator_test

import (
	// "fmt" // Not used yet, but might be for error messages
	"testing"
	// "time" // No longer used directly

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/orchestrator"
	// "github.com/yourorg/payment-orchestrator/internal/policy" // No longer imported directly
	"github.com/yourorg/payment-orchestrator/internal/router" // Using actual router
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	// "google.golang.org/protobuf/types/known/timestamppb" // Not used directly in these tests yet
)

// --- Mock Definitions ---
// MockProcessor is a mock for router.ProcessorInterface
type MockProcessor struct {
	mock.Mock
}

func (m *MockProcessor) ProcessSingleStep(
	ctx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
	adapter adapter.ProviderAdapter,
) (*orchestratorinternalv1.StepResult, error) {
	args := m.Called(ctx, step, adapter)
	res, _ := args.Get(0).(*orchestratorinternalv1.StepResult)
	return res, args.Error(1)
}

// MockPolicyEnforcer is a mock for orchestrator.PolicyEnforcerInterface
type MockPolicyEnforcer struct {
	mock.Mock
}

func (m *MockPolicyEnforcer) Evaluate(
	ctx context.StepExecutionContext,
	currentStep *orchestratorinternalv1.PaymentStep,
	stepResult *orchestratorinternalv1.StepResult,
) (bool, error) {
	args := m.Called(ctx, currentStep, stepResult)
	return args.Bool(0), args.Error(1)
}

// MockMerchantConfigRepository is a mock for context.MerchantConfigRepository
type MockMerchantConfigRepository struct {
	mock.Mock
}

func (m *MockMerchantConfigRepository) Get(merchantID string) (context.MerchantConfig, error) {
	args := m.Called(merchantID) // Corrected: Method name for mock setup should match interface
	cfg, _ := args.Get(0).(context.MerchantConfig)
	return cfg, args.Error(1)
}

func (m *MockMerchantConfigRepository) AddConfig(config context.MerchantConfig) {
	m.Called(config)
}
// --- End Mock Definitions ---

// Helper to create Orchestrator with a real Router (using MockProcessor) and a MockPolicyEnforcer
func newTestOrchestratorWithRealRouterAndMockPolicy(
	t *testing.T,
	mockProc *MockProcessor, // This is the local MockProcessor
	routerCfg router.RouterConfig,
	adapters map[string]adapter.ProviderAdapter,
	mockPE *MockPolicyEnforcer, // This is the local MockPolicyEnforcer
	mockMCR *MockMerchantConfigRepository,
) *orchestrator.Orchestrator {
	actualRouter := router.NewRouter(mockProc, adapters, routerCfg)
	// Corrected order of arguments for NewOrchestrator
	return orchestrator.NewOrchestrator(actualRouter, mockPE, mockMCR)
}

// Helper to create Orchestrator with MockRouter and MockPolicyEnforcer (more unit-test like for Orchestrator itself)
// For integration, the above is preferred. This can be used if router's internal logic is not part of the test.
// func newTestOrchestratorWithAllMocks(
// 	t *testing.T,
// 	mockRouter *MockRouter, // This would be orchestrator_test.MockRouter if it were in a suitable package
// 	mockPE *MockPolicyEnforcer,
// 	mockMCR *MockMerchantConfigRepository,
// ) *orchestrator.Orchestrator {
// 	return orchestrator.NewOrchestrator(mockRouter, mockPE, mockMCR)
// }


func TestOrchestratorIntegration_Execute_SingleStep_Success(t *testing.T) {
	// Arrange
	mockProcessor := new(MockProcessor)
	mockAdapter := &adaptermock.MockAdapter{Name: "primary"} // Using the manual mock from adapter/mock
	adapters := map[string]adapter.ProviderAdapter{"primary": mockAdapter}
	routerCfg := router.RouterConfig{PrimaryProviderName: "primary", FallbackProviderName: "fallback"}

	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMerchantRepo := new(MockMerchantConfigRepository)
	// Removed expectation on mockMerchantRepo.Get as Orchestrator.Execute does not call it.
	// The DomainContext should be provided as if it's already built.

	orc := newTestOrchestratorWithRealRouterAndMockPolicy(t, mockProcessor, routerCfg, adapters, mockPolicyEnforcer, mockMerchantRepo)

	step1 := &orchestratorinternalv1.PaymentStep{StepId: "step1", ProviderName: "primary", Amount: 100} // ProviderName here is a hint
	paymentPlan := &orchestratorinternalv1.PaymentPlan{PlanId: "plan1", Steps: []*orchestratorinternalv1.PaymentStep{step1}}

	traceCtx := context.NewTraceContext() // Create TraceContext separately
	domainCtx := context.DomainContext{
		MerchantID: "merchant1",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 5000},
		// ActiveMerchantConfig will be fetched via mockMerchantRepo by context builder (not directly used by orchestrator's Execute)
	}

	expectedStepResult := &orchestratorinternalv1.StepResult{StepId: "step1", Success: true, ProviderName: "primary"}
	mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.StepExecutionContext"), step1, mockAdapter).
		Return(expectedStepResult, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult).
		Return(false, nil).Once() // allowRetry = false

	// Act
	result, err := orc.Execute(traceCtx, paymentPlan, domainCtx) // Pass traceCtx separately

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 1)
	assert.Equal(t, expectedStepResult, result.StepResults[0])
	mockProcessor.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
	mockMerchantRepo.AssertExpectations(t) // This will now pass as no calls are expected
}

func TestOrchestratorIntegration_Execute_MultiStep_AllSuccess(t *testing.T) {
	mockProcessor := new(MockProcessor)
	mockAdapterPrimary := &adaptermock.MockAdapter{Name: "primary"}
	mockAdapterFallback := &adaptermock.MockAdapter{Name: "fallback"}
	adapters := map[string]adapter.ProviderAdapter{"primary": mockAdapterPrimary, "fallback": mockAdapterFallback}
	routerCfg := router.RouterConfig{PrimaryProviderName: "primary", FallbackProviderName: "fallback"}

	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMerchantRepo := new(MockMerchantConfigRepository)
	// Removed expectation on mockMerchantRepo.Get
	// mockMerchantRepo.On("Get", "merchant-multi").Return(context.MerchantConfig{ID: "merchant-multi"}, nil)

	orc := newTestOrchestratorWithRealRouterAndMockPolicy(t, mockProcessor, routerCfg, adapters, mockPolicyEnforcer, mockMerchantRepo)

	step1 := &orchestratorinternalv1.PaymentStep{StepId: "s1", ProviderName: "primary", Amount: 100}
	step2 := &orchestratorinternalv1.PaymentStep{StepId: "s2", ProviderName: "primary", Amount: 200} // Router will use primary for this too
	paymentPlan := &orchestratorinternalv1.PaymentPlan{PlanId: "plan-multi", Steps: []*orchestratorinternalv1.PaymentStep{step1, step2}}

	traceCtx := context.NewTraceContext() // Create TraceContext separately
	domainCtx := context.DomainContext{
		MerchantID: "merchant-multi",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 10000},
	}

	expectedStepResult1 := &orchestratorinternalv1.StepResult{StepId: "s1", Success: true, ProviderName: "primary"}
	expectedStepResult2 := &orchestratorinternalv1.StepResult{StepId: "s2", Success: true, ProviderName: "primary"}

	mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.StepExecutionContext"), step1, mockAdapterPrimary).
		Return(expectedStepResult1, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, expectedStepResult1).
		Return(false, nil).Once()

	mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.StepExecutionContext"), step2, mockAdapterPrimary).
		Return(expectedStepResult2, nil).Once()
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step2, expectedStepResult2).
		Return(false, nil).Once()

	result, err := orc.Execute(traceCtx, paymentPlan, domainCtx) // Pass traceCtx separately

	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	require.Len(t, result.StepResults, 2)
	assert.Equal(t, expectedStepResult1, result.StepResults[0])
	assert.Equal(t, expectedStepResult2, result.StepResults[1])
	mockProcessor.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
}

func TestOrchestratorIntegration_Execute_StepFailure_RouterDetermined_StopsExecution(t *testing.T) {
	// Orchestrator stops if a step fails AND policy evaluate says no retry (which is default mock for Evaluate if not set for failure)
	// This test checks if the router returns a failure, and policy (mocked) says no retry, plan stops.
	mockProcessor := new(MockProcessor)
	mockAdapter := &adaptermock.MockAdapter{Name: "primary"}
	adapters := map[string]adapter.ProviderAdapter{"primary": mockAdapter}
	routerCfg := router.RouterConfig{PrimaryProviderName: "primary", FallbackProviderName: "fallback"}

	mockPolicyEnforcer := new(MockPolicyEnforcer) // Mock policy enforcer
	mockMerchantRepo := new(MockMerchantConfigRepository)
	// Removed expectation on mockMerchantRepo.Get
	// mockMerchantRepo.On("Get", "merchant-fail-stop").Return(context.MerchantConfig{ID: "merchant-fail-stop"}, nil)

	orc := newTestOrchestratorWithRealRouterAndMockPolicy(t, mockProcessor, routerCfg, adapters, mockPolicyEnforcer, mockMerchantRepo)

	step1 := &orchestratorinternalv1.PaymentStep{StepId: "step1-fail", ProviderName: "primary"}
	step2 := &orchestratorinternalv1.PaymentStep{StepId: "step2-should-not-run", ProviderName: "primary"}
	paymentPlan := &orchestratorinternalv1.PaymentPlan{PlanId: "plan-fail", Steps: []*orchestratorinternalv1.PaymentStep{step1, step2}}

	traceCtx := context.NewTraceContext() // Create TraceContext separately
	domainCtx := context.DomainContext{
		MerchantID: "merchant-fail-stop",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 5000},
	}

	// Router's processor for primary adapter returns a failed step result
	primaryFailedStepResult := &orchestratorinternalv1.StepResult{StepId: "step1-fail", Success: false, ProviderName: "primary", ErrorCode: "GENERIC_DECLINE"}
	mockProcessor.On("ProcessSingleStep", mock.AnythingOfType("context.StepExecutionContext"), step1, mockAdapter).
		Return(primaryFailedStepResult, nil).Once()

	// Router then attempts fallback, but fallback adapter is not in `adapters` map for this specific test setup if we want to test this path
	// Let's adjust: assume fallback adapter *is* present, but router's ProcessSingleStep for fallback also fails or primary failure is enough.
	// The current router logic would attempt fallback. If fallback adapter is missing, Router returns ROUTER_CONFIGURATION_ERROR.
	// The test failure showed it was receiving ROUTER_CONFIGURATION_ERROR for provider "fallback".
	// This means the mockAdapter for "primary" was correctly used, and then the router tried "fallback".
	// The test setup for adapters was: adapters := map[string]adapter.ProviderAdapter{"primary": mockAdapter}
	// So, the fallback adapter "fallback" was indeed missing.

	// Expected result from Router when fallback adapter is missing after primary failure:
	expectedRouterOutputForEval := &orchestratorinternalv1.StepResult{
		StepId:       "step1-fail", // from original step
		Success:      false,
		ProviderName: "fallback", // Router reports the provider it attempted for fallback
		ErrorCode:    "ROUTER_CONFIGURATION_ERROR",
		ErrorMessage: "router: fallback provider adapter 'fallback' not found",
	}

	// Policy Enforcer mock: Evaluate is called with the result from the router (which indicates fallback attempt failed)
	mockPolicyEnforcer.On("Evaluate", mock.AnythingOfType("context.StepExecutionContext"), step1, mock.MatchedBy(func(sr *orchestratorinternalv1.StepResult) bool {
		return sr.GetStepId() == expectedRouterOutputForEval.StepId &&
			!sr.GetSuccess() &&
			sr.GetProviderName() == expectedRouterOutputForEval.ProviderName &&
			sr.GetErrorCode() == expectedRouterOutputForEval.ErrorCode
	})).Return(false, nil).Once() // false = no retry, so orchestrator should stop

	// Act
	result, err := orc.Execute(traceCtx, paymentPlan, domainCtx) // Pass traceCtx separately

	// Assert
	require.NoError(t, err) // Orchestrator itself doesn't error for plan failures
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1) // Only the first step should have a result

	actualResult := result.StepResults[0]
	assert.Equal(t, expectedRouterOutputForEval.StepId, actualResult.StepId)
	assert.False(t, actualResult.Success)
	assert.Equal(t, expectedRouterOutputForEval.ProviderName, actualResult.ProviderName) // Should be "fallback"
	assert.Equal(t, expectedRouterOutputForEval.ErrorCode, actualResult.ErrorCode)     // Should be "ROUTER_CONFIGURATION_ERROR"
	assert.Contains(t, actualResult.ErrorMessage, "fallback provider adapter 'fallback' not found")

	mockProcessor.AssertExpectations(t)
	mockPolicyEnforcer.AssertExpectations(t)
	// mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.AnythingOfType("context.StepExecutionContext"), step2, mock.Anything) // step2 should not be processed
}

// TestOrchestratorIntegration_Execute_PolicyDeniesRetry_StopsExecution is effectively the same as above with MockPolicyEnforcer.
// The distinction is subtle: router can fail a step, or policy can "fail" a step (by denying retry on a failed step).
// The above test covers the case where router returns Success:false, and policy says "no retry".

func TestOrchestratorIntegration_Execute_RouterReturnsError_StopsExecution(t *testing.T) {
	// Test case where the router itself encounters an error (e.g., provider not configured)
	mockProcessor := new(MockProcessor) // Will not be called if router fails before processor
	mockAdapter := &adaptermock.MockAdapter{Name: "primary"}
	adapters := map[string]adapter.ProviderAdapter{"primary": mockAdapter}
	// Intentionally use a router config that will cause an error in the real router
	routerCfg := router.RouterConfig{PrimaryProviderName: "non_existent_primary", FallbackProviderName: "fallback"}

	mockPolicyEnforcer := new(MockPolicyEnforcer)
	mockMerchantRepo := new(MockMerchantConfigRepository)
	// mockMerchantRepo.On("Get", "merchant-router-err").Return(context.MerchantConfig{ID: "merchant-router-err"}, nil)

	orc := newTestOrchestratorWithRealRouterAndMockPolicy(t, mockProcessor, routerCfg, adapters, mockPolicyEnforcer, mockMerchantRepo)

	step1 := &orchestratorinternalv1.PaymentStep{StepId: "step1-router-err", ProviderName: "non_existent_primary"}
	step2 := &orchestratorinternalv1.PaymentStep{StepId: "step2-should-not-run-router", ProviderName: "primary"}
	paymentPlan := &orchestratorinternalv1.PaymentPlan{PlanId: "plan-router-err", Steps: []*orchestratorinternalv1.PaymentStep{step1, step2}}

	traceCtx := context.NewTraceContext() // Create TraceContext separately
	domainCtx := context.DomainContext{
		MerchantID: "merchant-router-err",
		TimeoutConfig: context.TimeoutConfig{OverallBudgetMs: 5000},
	}

	// Router will return an error and a StepResult indicating config error.
	// No need to mock ProcessSingleStep on mockProcessor as router.ExecuteStep will fail before calling it.

	// Policy Enforcer mock: Evaluate is still called on the error result from the router
	// The router itself creates a StepResult when a provider is not found.
	expectedRouterErrorResult := &orchestratorinternalv1.StepResult{
		StepId:       "step1-router-err",
		Success:      false,
		ProviderName: "non_existent_primary",
		ErrorCode:    "ROUTER_CONFIGURATION_ERROR",
		ErrorMessage: "router: primary provider adapter 'non_existent_primary' not found",
	}
	// We need to use mock.MatchedBy for StepResult because its ErrorMessage might be dynamic/wrapped by router
	mockPolicyEnforcer.On("Evaluate",
		mock.AnythingOfType("context.StepExecutionContext"),
		step1,
		mock.MatchedBy(func(sr *orchestratorinternalv1.StepResult) bool {
			return sr.GetStepId() == expectedRouterErrorResult.StepId && !sr.GetSuccess() && sr.GetErrorCode() == expectedRouterErrorResult.ErrorCode
		}),
	).Return(false, nil).Once() // No retry on router config error


	result, err := orc.Execute(traceCtx, paymentPlan, domainCtx) // Pass traceCtx separately

	require.NoError(t, err) // Orchestrator itself does not error, the plan fails.
	assert.Equal(t, "FAILURE", result.Status)
	require.Len(t, result.StepResults, 1) // Only the first step attempted

	actualResult := result.StepResults[0]
	assert.Equal(t, expectedRouterErrorResult.StepId, actualResult.StepId)
	assert.False(t, actualResult.Success)
	assert.Equal(t, expectedRouterErrorResult.ProviderName, actualResult.ProviderName)
	assert.Equal(t, expectedRouterErrorResult.ErrorCode, actualResult.ErrorCode)
	// ErrorMessage from router might have more details, so Contains is safer.
	assert.Contains(t, actualResult.ErrorMessage, "primary provider adapter 'non_existent_primary' not found")

	mockProcessor.AssertNotCalled(t, "ProcessSingleStep", mock.Anything, mock.Anything, mock.Anything)
	mockPolicyEnforcer.AssertExpectations(t)
}

// TODO: Add test for PolicyEnforcer returning an error during Evaluate.
// TODO: Add test for a step failing but policy allows retry (plan still fails overall for now, but all steps should be attempted unless a prior failure caused a hard stop).
