package orchestrator

import (
	"fmt"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	"github.com/yourorg/payment-orchestrator/internal/router" // Import router
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
	policyEnforcer    *policy.PaymentPolicyEnforcer
	router            *router.Router
	merchantConfigRepo context.MerchantConfigRepository
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	pe *policy.PaymentPolicyEnforcer,
	mcr context.MerchantConfigRepository,
	rtr *router.Router, // Added router parameter
) *Orchestrator {
	if pe == nil {
		panic("PolicyEnforcer cannot be nil")
	}
	if mcr == nil {
		panic("MerchantConfigRepository cannot be nil")
	}
	if rtr == nil { // Check for nil router
		panic("Router cannot be nil")
	}
	return &Orchestrator{
		policyEnforcer:    pe,
		merchantConfigRepo: mcr,
		router:            rtr, // Store router
	}
}

// Execute processes a payment plan.
func (o *Orchestrator) Execute(
	traceCtx context.TraceContext,
	plan *internalv1.PaymentPlan,
	domainCtx context.DomainContext,
) (PaymentResult, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return PaymentResult{PaymentID: uuid.NewString(), Status: "FAILURE", FailureReason: "Plan is empty or nil"}, fmt.Errorf("plan cannot be empty or nil")
	}

	var allStepResults []*internalv1.StepResult
	overallSuccess := true
	anyStepAttempted := false

	startTime := time.Now()

	for i, step := range plan.Steps {
		if step == nil {
			overallSuccess = false
			stepResult := &internalv1.StepResult{
				StepId:       fmt.Sprintf("nil-step-%d", i),
				Success:      false,
				ErrorCode:    "ORCH_NIL_STEP_IN_PLAN",
				ErrorMessage: "Orchestrator encountered a nil step in the plan",
				Details:      make(map[string]string),
			}
			allStepResults = append(allStepResults, stepResult)
			continue
		}

		// Pass step.Amount and step.Currency to DeriveStepContext
		stepCtx := context.DeriveStepContext(traceCtx, domainCtx, step.ProviderName, step.Amount, step.Currency, domainCtx.TimeoutConfig.OverallBudgetMs, startTime)

		_, policyErr := o.policyEnforcer.Evaluate(domainCtx, stepCtx) // policyDecision is not used by current simplified logic
		if policyErr != nil {
			overallSuccess = false
			stepResult := &internalv1.StepResult{
				StepId:       step.StepId,
				Success:      false,
				ProviderName: step.ProviderName,
				ErrorCode:    "POLICY_EVALUATION_ERROR",
				ErrorMessage: fmt.Sprintf("Policy evaluation failed: %v", policyErr),
				LatencyMs:    time.Since(stepCtx.StartTime).Milliseconds(),
				Details:      make(map[string]string),
			}
			allStepResults = append(allStepResults, stepResult)
			continue
		}

		var currentStepResult *internalv1.StepResult
		var routerErr error

        // This is a simplified interpretation of PolicyDecision for now.
        // A more complex policy might have an explicit "SkipStep" field.
        // Current stub PolicyDecision: AllowRetry=true, SkipFallback=false, EscalateManual=false
        // We'll assume for now if !policyDecision.AllowRetry it implies do not attempt or it's a final decision.
        // The provided code for Orchestrator `Execute` method has a more complex placeholder for this.
        // For this iteration, we'll use a simplified path: if policy evaluation didn't error, proceed.
        // The `policyDecision.SkipFallback` is more relevant to the Router, not for skipping the step entirely here.
        // The current PolicyEnforcer stub always returns AllowRetry=true.
        // So, this logic will always proceed to router.ExecuteStep unless policyErr != nil.

		anyStepAttempted = true
		currentStepResult, routerErr = o.router.ExecuteStep(traceCtx, step, stepCtx)
		// Removed logging for RouterResult here for brevity, can be re-added if specific debug needed

		if routerErr != nil {
			overallSuccess = false
			if currentStepResult == nil {
				currentStepResult = &internalv1.StepResult{StepId: step.StepId, ProviderName: step.ProviderName, Details: make(map[string]string)}
			}
			currentStepResult.Success = false
			currentStepResult.ErrorCode = "ROUTER_EXECUTION_ERROR" // This indicates router's own error
			currentStepResult.ErrorMessage = fmt.Sprintf("Router execution failed: %v", routerErr)
			if currentStepResult.Details == nil { currentStepResult.Details = make(map[string]string) }
			currentStepResult.Details["router_error_details"] = routerErr.Error()
		}

		if currentStepResult == nil {
		    overallSuccess = false
		    currentStepResult = &internalv1.StepResult{
		        StepId: step.StepId, ProviderName: step.ProviderName, Success: false,
		        ErrorCode: "ORCH_INTERNAL_ERROR", ErrorMessage: "Missing step result from router",
		        Details: make(map[string]string),
		    }
		}

		if !currentStepResult.Success {
			overallSuccess = false
		}
		allStepResults = append(allStepResults, currentStepResult)
	}

	finalStatus := "FAILURE"
	if !anyStepAttempted && len(allStepResults) > 0 {
	    finalStatus = "FAILURE"
	} else if !anyStepAttempted && len(allStepResults) == 0 {
	    finalStatus = "FAILURE" // Should have been caught by initial plan empty check
	} else if overallSuccess {
		finalStatus = "SUCCESS"
	} else {
	    partialSuccess := false
	    for _, sr := range allStepResults {
	        if sr.Success {
	            partialSuccess = true
	            break
	        }
	    }
	    if partialSuccess {
	        finalStatus = "PARTIAL_SUCCESS"
	    } else {
	        finalStatus = "FAILURE"
	    }
	}

	return PaymentResult{
		PaymentID:   uuid.NewString(),
		Status:      finalStatus,
		StepResults: allStepResults,
	}, nil
}
