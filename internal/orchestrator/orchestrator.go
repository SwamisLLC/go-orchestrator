package orchestrator

import (
	"fmt"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	// "github.com/yourorg/payment-orchestrator/internal/router" // Will be needed later
	"github.com/google/uuid"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// PaymentResult represents the overall result of executing a payment plan.
type PaymentResult struct {
	PaymentID     string                       `json:"paymentId"`
	Status        string                       `json:"status"` // e.g., "SUCCESS", "PARTIAL_SUCCESS", "FAILURE"
	StepResults   []*internalv1.StepResult     `json:"stepResults"`
	FailureReason string                       `json:"failureReason,omitempty"`
}

// Orchestrator manages execution order, high-level policies, and per-step context derivation.
type Orchestrator struct {
	// adapterRegistry   map[string]adapter.ProviderAdapter // Will be used by Router
	policyEnforcer    *policy.PaymentPolicyEnforcer
	// router            router.Router // Will be integrated later
	merchantConfigRepo context.MerchantConfigRepository // For additional merchant-level lookups
}

// NewOrchestrator creates a new Orchestrator.
// Dependencies like Router will be added in later chunks.
func NewOrchestrator(
	pe *policy.PaymentPolicyEnforcer,
	mcr context.MerchantConfigRepository,
	// rtr router.Router, // Add when Router is ready
) *Orchestrator {
	if pe == nil {
		panic("PolicyEnforcer cannot be nil")
	}
	if mcr == nil {
	    panic("MerchantConfigRepository cannot be nil")
	}
	return &Orchestrator{
		policyEnforcer:    pe,
		merchantConfigRepo: mcr,
		// router: rtr,
	}
}

// Execute processes a payment plan.
// For this placeholder version, it iterates through steps and returns stubbed success StepResults.
func (o *Orchestrator) Execute(
	traceCtx context.TraceContext,
	plan *internalv1.PaymentPlan,
	domainCtx context.DomainContext,
) (PaymentResult, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return PaymentResult{Status: "FAILURE", FailureReason: "Plan is empty or nil"}, fmt.Errorf("plan cannot be empty or nil")
	}

	var allStepResults []*internalv1.StepResult
	overallStatus := "SUCCESS" // Assume success initially
	startTime := time.Now()    // For SLA budget tracking (simplified for now)

	for i, step := range plan.Steps {
		if step == nil {
		    // Handle nil step if necessary, perhaps log and skip
		    overallStatus = "FAILURE"
		    stepResult := &internalv1.StepResult{
		        StepId: fmt.Sprintf("nil-step-%d", i),
		        Success: false,
		        ErrorCode: "NIL_STEP",
		        ErrorMessage: "Encountered a nil step in the plan",
		    }
		    allStepResults = append(allStepResults, stepResult)
		    continue
		}

		stepCtx := context.DeriveStepContext(traceCtx, domainCtx, step.ProviderName, domainCtx.TimeoutConfig.OverallBudgetMs, startTime)

		// 1. Policy Check (Placeholder - actual policy evaluation will happen here)
		// policyDecision, err := o.policyEnforcer.Evaluate(domainCtx, step, stepCtx)
		// if err != nil {
		//    // Handle error from policy evaluation
		// }
		// if policyDecision.SkipStep { /* ... */ }

		// 2. Execute Step via Router (Placeholder - actual call to router.ExecuteStep will be here)
		// For now, create a stub success StepResult.
		latency := time.Since(stepCtx.StartTime).Milliseconds()
		stepResult := &internalv1.StepResult{
			StepId:       step.StepId,
			Success:      true, // Stub success
			ProviderName: step.ProviderName,
			ErrorCode:    "",
			LatencyMs:    latency, // Placeholder latency
			Details:      map[string]string{"stubbed_result": "success"},
		}

		// Ensure details map is initialized
		if stepResult.Details == nil {
		    stepResult.Details = make(map[string]string)
		}


		allStepResults = append(allStepResults, stepResult)

		// If any step (even stubbed) were to fail, update overallStatus
		// if !stepResult.Success {
		// overallStatus = "FAILURE" // Or PARTIAL_SUCCESS depending on policy
		// }
	}

	// In a real scenario, if any step failed, overallStatus would be "FAILURE" or "PARTIAL_SUCCESS"
	// For this stub, all steps are successful.
	return PaymentResult{
		PaymentID:   uuid.NewString(), // Generate a unique ID for this payment execution
		Status:      overallStatus,
		StepResults: allStepResults,
	}, nil
}
