package planbuilder

import (
	"fmt"
	"log"
	"strconv"

	"github.com/google/uuid"
	"github.com/yourorg/payment-orchestrator/internal/context"
	// commonv1 is not used if Amount is int64 directly on PaymentStep
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// CompositePaymentService defines the interface for optimizing payment plans.
type CompositePaymentService interface {
	Optimize(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error)
}

// StubCompositePaymentService is a stub implementation that will now include simple splitting logic.
type StubCompositePaymentService struct {
}

// NewStubCompositePaymentService creates a new stub composite payment service.
func NewStubCompositePaymentService() *StubCompositePaymentService {
	return &StubCompositePaymentService{}
}

// Optimize method now implements simple amount splitting logic.
func (s *StubCompositePaymentService) Optimize(domainCtx context.DomainContext, plan *orchestratorinternalv1.PaymentPlan) (*orchestratorinternalv1.PaymentPlan, error) {
	if plan == nil || len(plan.Steps) == 0 {
		log.Println("CompositePaymentService.Optimize: Input plan is nil or empty, returning as is.")
		return plan, nil
	}

	enableSplit, splitEnabledExists := domainCtx.FeatureFlags["enable_payment_splitting"]
	splitPartsStr, partsStrExists := domainCtx.UserPreferences["split_parts"]

	if !splitEnabledExists || !enableSplit || !partsStrExists {
		log.Printf("CompositePaymentService.Optimize: Payment splitting not enabled or split_parts not set. FeatureFlags['enable_payment_splitting']=%t (exists: %t), UserPreferences['split_parts']='%s' (exists: %t). Returning plan as is.", enableSplit, splitEnabledExists, splitPartsStr, partsStrExists)
		return plan, nil
	}

	numParts, err := strconv.Atoi(splitPartsStr)
	if err != nil || numParts <= 1 {
		log.Printf("CompositePaymentService.Optimize: Invalid split_parts value '%s' (error: %v) or numParts <= 1. Defaulting to no split.", splitPartsStr, err)
		return plan, nil
	}

	log.Printf("CompositePaymentService.Optimize: Splitting payment into %d parts.", numParts)

	if len(plan.Steps) != 1 {
		log.Printf("CompositePaymentService.Optimize: Expected 1 step in plan for simple splitting, found %d. Returning plan as is.", len(plan.Steps))
		return plan, nil
	}
	originalStep := plan.Steps[0]

	if originalStep.GetAmount() <= 0 { // Amount is int64, directly on step
		log.Printf("CompositePaymentService.Optimize: Original step amount is zero or negative (%d). Cannot split. Returning plan as is.", originalStep.GetAmount())
		return plan, nil
	}

	originalAmountUnits := originalStep.GetAmount()
	originalCurrencyCode := originalStep.GetCurrency()

	baseSplitAmount := originalAmountUnits / int64(numParts)
	remainder := originalAmountUnits % int64(numParts)

	if baseSplitAmount == 0 && originalAmountUnits > 0 {
	    log.Printf("CompositePaymentService.Optimize: Original amount %d %s is too small to be split into %d parts with minimum 1 unit each. Returning plan as is.", originalAmountUnits, originalCurrencyCode, numParts)
	    return plan, nil
	}

	newSteps := make([]*orchestratorinternalv1.PaymentStep, 0, numParts)
	for i := 0; i < numParts; i++ {
		splitAmountUnits := baseSplitAmount
		if i == 0 && remainder > 0 {
			splitAmountUnits += remainder
		}

		newStep := &orchestratorinternalv1.PaymentStep{
			StepId:       uuid.NewString(),
			ProviderName: originalStep.GetProviderName(),
			Amount:       splitAmountUnits,       // Assign int64 directly
			Currency:     originalCurrencyCode, // Assign string directly
			Metadata: map[string]string{
				"original_step_id": originalStep.GetStepId(),
				"split_part_index": fmt.Sprintf("%d", i+1),
				"total_split_parts": fmt.Sprintf("%d", numParts),
			},
            IsFanOut: false,
            ProviderPayload: make(map[string]string),
		}

        if originalStep.GetMetadata() != nil {
            for k, v := range originalStep.GetMetadata() {
                if _, exists := newStep.Metadata[k]; !exists {
                    newStep.Metadata[k] = v
                }
            }
        }
        if originalStep.GetProviderPayload() != nil {
             for k,v := range originalStep.GetProviderPayload() {
                 newStep.ProviderPayload[k] = v
             }
        }
		newSteps = append(newSteps, newStep)
	}

	optimizedPlan := &orchestratorinternalv1.PaymentPlan{
		PlanId: plan.GetPlanId(), // Use PlanId
		Steps:  newSteps,
	}

	log.Printf("CompositePaymentService.Optimize: Payment split into %d steps.", len(optimizedPlan.Steps))
	return optimizedPlan, nil
}
