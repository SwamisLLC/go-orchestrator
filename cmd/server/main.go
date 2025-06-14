package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	custom_context "github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/orchestrator"
	"github.com/yourorg/payment-orchestrator/internal/planbuilder"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	"github.com/yourorg/payment-orchestrator/internal/adapter" // Corrected path
	adaptermock "github.com/yourorg/payment-orchestrator/internal/adapter/mock" // Corrected path
	"github.com/yourorg/payment-orchestrator/internal/router"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	protos "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

func processPaymentHandler(c *gin.Context) {
	var req orchestratorexternalv1.ExternalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// Basic Validation
	if req.MerchantId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed: MerchantId is required"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed: Amount must be positive"})
		return
	}
	if req.Currency == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed: Currency is required"})
		return
	}

	// Initialize Dependencies
	merchantRepo := custom_context.NewInMemoryMerchantConfigRepository()
	merchantRepo.AddConfig(custom_context.MerchantConfig{
		ID:              req.MerchantId,
		DefaultProvider: "mock-primary",
		DefaultCurrency: req.Currency,
		ProviderAPIKeys: map[string]string{"mock-primary": "pk_live_mock", "mock-fallback": "fk_live_mock"},
		UserPrefs:       map[string]string{"merchant_tier": "gold", "split_parts": "1"},
		DefaultTimeout:  custom_context.TimeoutConfig{OverallBudgetMs: 1000, ProviderTimeoutMs: 600}, // Added positive budget
	})

	compositeService := planbuilder.NewStubCompositePaymentService()
	pb := planbuilder.NewPlanBuilder(merchantRepo, compositeService)

	mockPrimaryAdapter := adaptermock.NewMockAdapter("mock-primary")
	mockPrimaryAdapter.ProcessFunc = func(traceCtx custom_context.TraceContext, step *protos.PaymentStep, stepCtx custom_context.StepExecutionContext) (adapter.ProviderResult, error) {
		// Simulate a successful processing
		return adapter.ProviderResult{
			StepID:        step.GetStepId(),
			Success:       true,
			Provider:      step.GetProviderName(), // Or mockPrimaryAdapter.GetName()
			TransactionID: "txn_mock_" + step.GetProviderName(),
			Details:       map[string]string{"source": "mock_primary_adapter_in_main"},
		}, nil
	}

	mockFallbackAdapter := adaptermock.NewMockAdapter("mock-fallback")
	mockFallbackAdapter.ProcessFunc = func(traceCtx custom_context.TraceContext, step *protos.PaymentStep, stepCtx custom_context.StepExecutionContext) (adapter.ProviderResult, error) {
		// Simulate a successful processing
		return adapter.ProviderResult{
			StepID:        step.GetStepId(),
			Success:       true,
			Provider:      step.GetProviderName(), // Or mockFallbackAdapter.GetName()
			TransactionID: "txn_mock_" + step.GetProviderName(),
			Details:       map[string]string{"source": "mock_fallback_adapter_in_main"},
		}, nil
	}
	adaptersMap := map[string]adapter.ProviderAdapter{"mock-primary": mockPrimaryAdapter, "mock-fallback": mockFallbackAdapter}

	proc := processor.NewProcessor()
	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{})
	routerCfg := router.RouterConfig{PrimaryProviderName: "mock-primary", FallbackProviderName: "mock-fallback"}
	rtr, err := router.NewRouter(proc, adaptersMap, routerCfg, cb)
	if err != nil {
		log.Printf("Error creating router: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server configuration error: failed to initialize router"})
		return
	}

	rules := []policy.RuleConfig{{Name: "DefaultRetry", Expression: "attempt_number < 2 && !step_success"}}
	policyEnforcer, err := policy.NewPaymentPolicyEnforcer(rules)
	if err != nil {
		log.Printf("Error creating policy enforcer: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server configuration error: failed to initialize policy enforcer"})
		return
	}

	orch := orchestrator.NewOrchestrator(rtr, policyEnforcer, merchantRepo)

	// Build Contexts
	// Create a ContextBuilder instance
	contextBuilder := custom_context.NewContextBuilder(merchantRepo)
	// Call BuildContexts on the instance, it returns traceCtx as well
	traceCtx, domainCtx, err := contextBuilder.BuildContexts(&req)
	if err != nil {
		log.Printf("Error building contexts: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error building request context: " + err.Error()})
		return
	}

	// Build Plan
	paymentPlan, err := pb.Build(domainCtx, &req)
	if err != nil {
		log.Printf("Error building payment plan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error building payment plan: " + err.Error()})
		return
	}

	// Execute Plan
	paymentResult, err := orch.Execute(traceCtx, paymentPlan, domainCtx)
	if err != nil {
		log.Printf("Critical error during payment execution: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Critical error during payment execution: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, paymentResult)
}

func setupRouter() *gin.Engine {
	router := gin.Default()
	router.POST("/process-payment", processPaymentHandler)
	return router
}

func main() {
	log.Println("Starting server...")
	router := setupRouter()
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
