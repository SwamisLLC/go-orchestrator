package main

import (
	"net/http"
	"os"
	"fmt" // For logging errors during setup
	"strings" // Added strings import
	// "bytes" // No longer needed after removing body peeking for logging

	"github.com/gin-gonic/gin"
	adapterpkg "github.com/yourorg/payment-orchestrator/internal/adapter" // Renamed to avoid collision
	"github.com/yourorg/payment-orchestrator/internal/adapter/mock"
	"github.com/yourorg/payment-orchestrator/internal/adapter/stripe"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/orchestrator"
	"github.com/yourorg/payment-orchestrator/internal/planbuilder"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	"github.com/yourorg/payment-orchestrator/internal/processor"
	"github.com/yourorg/payment-orchestrator/internal/router"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
	externalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1" // Added for mock adapter signature
	"google.golang.org/protobuf/encoding/protojson" // Added for proper JSON handling of protos
	"io/ioutil" // Added for reading request body
)

var (
	// Global instances of our services. In a real app, these would be managed
	// more carefully (e.g., dependency injection framework).
	appOrchestrator *orchestrator.Orchestrator
	appContextBuilder *context.ContextBuilder
	appPlanBuilder    *planbuilder.PlanBuilder
)

func setupApplication() error {
	// 1. Merchant Config Repository (In-memory for now)
	merchantRepo := context.NewInMemoryMerchantConfigRepository()
	merchantRepo.AddConfig(context.MerchantConfig{
		ID:              "merchant123",
		PolicyVersion:   1,
		UserPrefs:       map[string]string{"region": "US", "tier": "gold"},
		DefaultTimeout:  context.TimeoutConfig{OverallBudgetMs: 30000, ProviderTimeoutMs: 5000},
		DefaultRetryPolicy: context.RetryPolicy{MaxAttempts: 2},
		FeatureFlags:    map[string]bool{"newRouting": true},
		ProviderAPIKeys: map[string]string{
		    "stripe": "sk_test_yourstripeapikey", // Use a real test key if testing Stripe live
		    "mock_success": "mock_key_success",
		    "mock_fail": "mock_key_fail",
		},
		DefaultProvider: "stripe",
		DefaultCurrency: "USD",
	})
	merchantRepo.AddConfig(context.MerchantConfig{
		ID: "merchant456",
		DefaultProvider: "mock_success",
		DefaultCurrency: "EUR",
		UserPrefs: map[string]string{"region": "EU", "tier": "silver"},
		DefaultTimeout:  context.TimeoutConfig{OverallBudgetMs: 30000, ProviderTimeoutMs: 5000}, // Added
		DefaultRetryPolicy: context.RetryPolicy{MaxAttempts: 1}, // Added
		ProviderAPIKeys: map[string]string{"mock_success": "key"},
	})


	// 2. Context Builder
	appContextBuilder = context.NewContextBuilder(merchantRepo)

	// 3. Composite Payment Service (Simple Split Optimizer)
	compositeSvc := planbuilder.NewStubCompositePaymentService("stripe", "mock_success")


	// 4. Plan Builder
	appPlanBuilder = planbuilder.NewPlanBuilder(merchantRepo, compositeSvc)

	// 5. Policy Enforcer (DSL-based, with some sample rules or no rules)
	rules := []policy.PolicyRule{
		{ID: "block_high_fraud", Expression: "fraudScore > 0.9", Decision: policy.PolicyDecision{AllowRetry: false, SkipFallback: true, EscalateManual: true}},
		{ID: "default_allow", Expression: "true", Decision: policy.PolicyDecision{AllowRetry: true, SkipFallback: false, EscalateManual: false}},
	}
	policyEnforcer, err := policy.NewPaymentPolicyEnforcer(rules)
	if err != nil {
		return fmt.Errorf("failed to create policy enforcer: %w", err)
	}

	// 6. Adapters
	mockSuccessAdapter := mock.NewMockAdapter("mock_success")
	mockFailAdapter := mock.NewMockAdapter("mock_fail")
	mockFailAdapter.ProcessFunc = func(tc context.TraceContext, s *internalv1.PaymentStep, sc context.StepExecutionContext) (adapterpkg.ProviderResult, error) {
		return adapterpkg.ProviderResult{StepID: s.StepId, Success: false, Provider: "mock_fail", ErrorCode: "MOCK_DECLINE"}, nil
	}

	stripeAdapterInstance := stripe.NewStripeAdapter(nil)


	adapterRegistry := map[string]adapterpkg.ProviderAdapter{
		"mock_success": mockSuccessAdapter,
		"mock_fail":    mockFailAdapter,
		"stripe":       stripeAdapterInstance,
	}

	// 7. Processor
	appProcessor := processor.NewProcessor(adapterRegistry)

	// 8. Circuit Breaker
	appCircuitBreaker := circuitbreaker.NewCircuitBreaker()

	// 9. Router
	appRouter := router.NewRouter(appProcessor, "mock_success", appCircuitBreaker)

	// 10. Orchestrator
	appOrchestrator = orchestrator.NewOrchestrator(policyEnforcer, merchantRepo, appRouter)

	fmt.Println("Application setup complete.")
	return nil
}

func processPaymentHandler(c *gin.Context) {
	var req externalv1.ExternalRequest

	// Read raw body for protojson unmarshalling
	jsonData, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body: " + err.Error()})
		return
	}

	// Unmarshal using protojson
	if err := protojson.Unmarshal(jsonData, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload (protojson): " + err.Error()})
		return
	}

	// Validation Note: Full JSON schema validation deferred.

	traceCtx, domainCtx, errCtx := appContextBuilder.BuildContexts(&req)
	// Set baggage if context build is fine and it's the target merchant for debug logging
	if errCtx == nil && req.MerchantId == "merchant456" {
		if traceCtx.Baggage == nil {
			traceCtx.Baggage = make(map[string]string)
		}
		traceCtx.Baggage["debug_merchant_id"] = "merchant456"
	}
	if errCtx != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build request context: " + errCtx.Error()})
		return
	}

	paymentPlan, errPlan := appPlanBuilder.Build(domainCtx, &req)
	if errPlan != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build payment plan: " + errPlan.Error()})
		return
	}

	paymentResult, errExecute := appOrchestrator.Execute(traceCtx, paymentPlan, domainCtx)
	if errExecute != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment orchestration execution error: " + errExecute.Error()})
		return
	}

	httpStatus := http.StatusOK
	if paymentResult.Status == "FAILURE" {
	    httpStatus = http.StatusUnprocessableEntity
	} else if paymentResult.Status == "PARTIAL_SUCCESS" {
	    httpStatus = http.StatusMultiStatus
	}

	c.JSON(httpStatus, paymentResult)
}

func main() {
    gin.SetMode(gin.ReleaseMode) // Default to release mode
    // Check for an env variable to run in debug mode if needed
    if strings.ToLower(os.Getenv("GIN_MODE")) == "debug" {
        gin.SetMode(gin.DebugMode)
    }


    if err := setupApplication(); err != nil {
        fmt.Fprintf(os.Stderr, "Error during application setup: %v\n", err)
        os.Exit(1)
    }

	r := gin.Default()
	r.POST("/process-payment", processPaymentHandler)

	port := os.Getenv("PORT")
	if port == "" {
	    port = "8080" // Default port
	}
	fmt.Printf("Starting server on port %s...\n", port)
	if err := r.Run(":" + port); err != nil {
	    fmt.Fprintf(os.Stderr, "Failed to run server: %v\n", err)
	    os.Exit(1)
	}
}
