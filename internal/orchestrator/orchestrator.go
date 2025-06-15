// Package orchestrator is responsible for managing the execution of a payment plan.
// It iterates through the steps of a PaymentPlan, applies high-level policies,
// derives per-step execution contexts, and coordinates with the Router
// for actual step execution.
// Refer to spec2_1.md, section 7.2 for more details on the Orchestrator.
package orchestrator

import (
	"fmt"
	"log" // Added for logging
	"time"

	"go.opentelemetry.io/otel"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy" // Added import for policy.PolicyDecision
	// "github.com/yourorg/payment-orchestrator/internal/router" // Interface will be used
	"github.com/google/uuid"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// RouterInterface defines the contract for executing a payment step via a router.
type RouterInterface interface {
	ExecuteStep(traceCtx context.TraceContext, ctx context.StepExecutionContext, step *internalv1.PaymentStep) (*internalv1.StepResult, error)
}

// PolicyEnforcerInterface defines the contract for evaluating policies on a step result.
// This needs to match the richer signature of policy.PaymentPolicyEnforcer.Evaluate
type PolicyEnforcerInterface interface {
	Evaluate(
		domainCtx context.DomainContext,
		stepCtx context.StepExecutionContext,
		currentStep *internalv1.PaymentStep,
		stepResult *internalv1.StepResult,
	) (policy.PolicyDecision, error) // Updated to return policy.PolicyDecision
}

// PaymentResult represents the overall result of executing a payment plan.
type PaymentResult struct {
	PaymentID     string                       `json:"paymentId"`
	Status        string                       `json:"status"` // e.g., "SUCCESS", "PARTIAL_SUCCESS", "FAILURE"
	StepResults   []*internalv1.StepResult     `json:"stepResults"`
	FailureReason string                       `json:"failureReason,omitempty"`
}

// Orchestrator manages execution order, high-level policies, and per-step context derivation.
type Orchestrator struct {
	router             RouterInterface
	policyEnforcer     PolicyEnforcerInterface
	merchantConfigRepo context.MerchantConfigRepository // For additional merchant-level lookups
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	r RouterInterface,
	pe PolicyEnforcerInterface,
	mcr context.MerchantConfigRepository,
) *Orchestrator {
	if r == nil {
		panic("Router cannot be nil")
	}
	if pe == nil {
		panic("PolicyEnforcer cannot be nil")
	}
	if mcr == nil {
		panic("MerchantConfigRepository cannot be nil")
	}
	return &Orchestrator{
		router:             r,
		policyEnforcer:     pe,
		merchantConfigRepo: mcr,
	}
}

// Execute processes a payment plan.
func (o *Orchestrator) Execute(
	traceCtx context.TraceContext,
	plan *internalv1.PaymentPlan,
	domainCtx context.DomainContext,
) (PaymentResult, error) {
	// Get a tracer instance
	tracer := otel.Tracer("orchestrator")

	// Start a new span
	ctx, span := tracer.Start(traceCtx.Context(), "Orchestrator.Execute")
	defer span.End()

	// Update traceCtx with the new context from the span
	// The new stdCtx (named 'ctx' here) comes from tracer.Start()
	// The TraceID is propagated from the original traceCtx (which is 'traceCtx' before this line)
	// The SpanID for the new TraceContext should be the ID of the new OpenTelemetry span
	// Make sure to capture the original TraceID before 'traceCtx' is reassigned if it's not from an outer scope.
	// In this function signature, traceCtx is an input parameter, so its GetTraceID() is the original.
	originalTraceID := traceCtx.GetTraceID() // Capture before reassignment if traceCtx was from a wider scope and mutable
	traceCtx = context.NewTraceContextWithIDs(ctx, originalTraceID, span.SpanContext().SpanID().String())


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

		// Assuming this is the first attempt for any step, as Orchestrator doesn't have retry logic yet.
		attemptNumber := 1
		stepCtx := context.DeriveStepContext(traceCtx, domainCtx, step.ProviderName, domainCtx.TimeoutConfig.OverallBudgetMs, startTime, attemptNumber)

		currentStepResult, currentStepErr := o.router.ExecuteStep(traceCtx, stepCtx, step) // Pass traceCtx

		if currentStepErr != nil {
			log.Printf("Orchestrator: Error from router for StepID %s: %v", step.GetStepId(), currentStepErr)
			if currentStepResult == nil { // Router error was critical, no result provided
				currentStepResult = &internalv1.StepResult{
					StepId:       step.GetStepId(),
					Success:      false,
					ProviderName: step.GetProviderName(), // Or router's attempted provider
					ErrorCode:    "ORCHESTRATOR_ROUTER_CRITICAL_ERROR",
					ErrorMessage: currentStepErr.Error(),
				}
			} else { // Router provided a result, but also an error
				currentStepResult.Success = false // Ensure success is false
				if currentStepResult.ErrorCode == "" {
					currentStepResult.ErrorCode = "ROUTER_ERROR_WITH_RESULT"
				}
				// Preserve router's error message if specific, otherwise use the error object's message
				if currentStepResult.ErrorMessage == "" && currentStepErr != nil {
					currentStepResult.ErrorMessage = currentStepErr.Error()
				}
			}
		} else if currentStepResult == nil { // No error, but also no result (bad router behavior)
			log.Printf("Orchestrator: Nil result and nil error from router for StepID %s (unexpected)", step.GetStepId())
			currentStepResult = &internalv1.StepResult{
				StepId:       step.GetStepId(),
				Success:      false,
				ProviderName: step.GetProviderName(),
				ErrorCode:    "ORCHESTRATOR_NIL_ROUTER_RESULT",
				ErrorMessage: "Router returned nil result and nil error, which is unexpected.",
			}
			// Optionally, create an error to signal this unexpected state, though currentStepErr is nil here
			// currentStepErr = fmt.Errorf("router returned nil result and nil error for step %s", step.GetStepId())
		}

		allStepResults = append(allStepResults, currentStepResult)

		// Policy Evaluation on currentStepResult
		log.Printf("Orchestrator: Evaluating policy for StepID %s, Success: %t", step.GetStepId(), currentStepResult.GetSuccess())
		// Corrected call to Evaluate, removing domainCtx as per build error - REVERTING THIS and using the interface
		// The interface will now match the concrete type, which includes domainCtx.
		policyDecision, policyErr := o.policyEnforcer.Evaluate(domainCtx, stepCtx, step, currentStepResult)
		allowRetry := false // Default to no retry unless policyDecision changes it
		if policyDecision.AllowRetry { // Check if policyDecision is not nil if it can be
			allowRetry = true
		}


		if policyErr != nil {
			log.Printf("Orchestrator: Error evaluating policy for StepID %s: %v", step.GetStepId(), policyErr)
			// Append policy error to existing error message if any, or set it.
			currentStepResult.ErrorMessage = fmt.Sprintf("OriginalMsg: [%s]; PolicyEvalErr: [%s]", currentStepResult.GetErrorMessage(), policyErr.Error())
			currentStepResult.Success = false // Policy error implies the step cannot be considered successful in its current state
			if currentStepResult.ErrorCode == "" {
				currentStepResult.ErrorCode = "POLICY_EVALUATION_ERROR"
			}
			// Add policy decision details to step result metadata if needed
			if policyDecision.Reason != "" {
				if currentStepResult.Details == nil { currentStepResult.Details = make(map[string]string) }
				currentStepResult.Details["policy_reason"] = policyDecision.Reason
				currentStepResult.Details["policy_action"] = policyDecision.NextAction
			}
		} else { // policyErr is nil, use policyDecision
			allowRetry = policyDecision.AllowRetry
			if currentStepResult.Details == nil { currentStepResult.Details = make(map[string]string) }
			currentStepResult.Details["policy_reason"] = policyDecision.Reason
			currentStepResult.Details["policy_action"] = policyDecision.NextAction
			currentStepResult.Details["policy_allow_retry"] = fmt.Sprintf("%t", allowRetry)
			currentStepResult.Details["policy_skip_fallback"] = fmt.Sprintf("%t", policyDecision.SkipFallback) // useful for router
		}


		if !currentStepResult.GetSuccess() {
			overallStatus = "FAILURE" // Mark plan as failed if any step effectively fails
			// Use policyDecision.SkipFallback if needed by router logic (not directly here)
			if !allowRetry && policyErr == nil {
				log.Printf("Orchestrator: StepID %s failed and policy does not allow retry. Stopping plan execution.", step.GetStepId())
				break // Stop processing further steps
			} else if policyErr != nil { // Already logged, plan marked as failure
				log.Printf("Orchestrator: StepID %s processing failed due to policy evaluation error. Plan marked as failed.", step.GetStepId())
			} else { // Failed, but retry is allowed
				log.Printf("Orchestrator: StepID %s failed but policy allows retry. (Note: Orchestrator-level retry not yet implemented). Plan marked as failed.", step.GetStepId())
			}
		}
	}
	return PaymentResult{
		PaymentID:   uuid.NewString(), // Generate a unique ID for this payment execution
		Status:      overallStatus,
		StepResults: allStepResults,
	}, nil
}
