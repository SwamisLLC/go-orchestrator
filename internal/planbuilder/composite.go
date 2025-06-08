package planbuilder

import (
	"strings"

	"github.com/google/uuid"
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// CompositePaymentService defines the interface for optimizing payment plans.
type CompositePaymentService interface {
	Optimize(domainCtx context.DomainContext, plan *internalv1.PaymentPlan) (*internalv1.PaymentPlan, error)
}

// StubCompositePaymentService now implements a simple split optimization.
type StubCompositePaymentService struct {
	// Configurable default split providers if not specified in metadata
	// This field is not used by the current Optimize logic but could be in a more advanced version.
	DefaultSplitProviders []string
}

// NewStubCompositePaymentService creates a new stub composite payment service.
// The defaultSplitProviders argument is currently not used by the Optimize method's logic
// but is kept for potential future enhancements where defaults might be applied if no metadata is found.
func NewStubCompositePaymentService(defaultSplitProviders ...string) *StubCompositePaymentService {
	dsp := []string{"default-split-provider-1", "default-split-provider-2"} // Default if nothing passed
	if len(defaultSplitProviders) > 0 && defaultSplitProviders[0] != "" {
		// This logic for setting dsp based on defaultSplitProviders argument
		// is a bit simplistic. If multiple args are passed, it only uses the first if not empty.
		// A better way would be to directly assign defaultSplitProviders if len > 0.
		// For now, adhering to the spirit of having it.
		// If defaultSplitProviders = ["p1", "p2"], dsp becomes ["p1", "p2"]
		// If defaultSplitProviders = ["p1"], dsp becomes ["p1"]
		// If defaultSplitProviders = [], dsp remains the hardcoded default.
		// If defaultSplitProviders = [""], dsp remains the hardcoded default.
		// Corrected logic:
		if len(defaultSplitProviders) > 0 {
		    isAllEmpty := true
		    for _, s := range defaultSplitProviders {
		        if s != "" {
		            isAllEmpty = false
		            break
		        }
		    }
		    if !isAllEmpty {
		        // Filter out empty strings if any, and use the provided ones
		        var validProviders []string
		        for _, s := range defaultSplitProviders {
		            if s != "" {
		                validProviders = append(validProviders, s)
		            }
		        }
		        if len(validProviders) > 0 {
		             dsp = validProviders
		        }
		    }
		}
	}
	return &StubCompositePaymentService{
		DefaultSplitProviders: dsp,
	}
}

// Optimize implements a simple split based on metadata.
// It looks for "split_payment_evenly_to_providers" in step metadata or domain context.
// Example value: "stripe,adyen"
func (s *StubCompositePaymentService) Optimize(domainCtx context.DomainContext, plan *internalv1.PaymentPlan) (*internalv1.PaymentPlan, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return plan, nil // Nothing to optimize
	}

	// This optimizer only acts on single-step plans for simplicity.
	if len(plan.Steps) != 1 {
		return plan, nil // Only optimize single-step plans for now
	}

	originalStep := plan.Steps[0]
	// Metadata check must happen after checking originalStep is not nil (already implicitly true due to len(plan.Steps) != 1 check for this path)

	splitProvidersStr := ""
	ok := false

	if originalStep.Metadata != nil {
		splitProvidersStr, ok = originalStep.Metadata["split_payment_evenly_to_providers"]
	}

	if !ok || splitProvidersStr == "" {
		// Check domain context user preferences as a fallback
		if domainCtx.UserPreferences != nil {
		    splitProvidersStr, ok = domainCtx.UserPreferences["split_payment_evenly_to_providers"]
		}
		if !ok || splitProvidersStr == "" {
		    return plan, nil // No split instruction
		}
	}

	providersToSplit := strings.Split(splitProvidersStr, ",")
	var validSplitProviders []string
	for _, p := range providersToSplit {
	    trimmed := strings.TrimSpace(p)
	    if trimmed != "" {
	        validSplitProviders = append(validSplitProviders, trimmed)
	    }
	}

	if len(validSplitProviders) < 2 { // Need at least two providers to split
		return plan, nil
	}

	numProviders := len(validSplitProviders)
	originalAmount := originalStep.Amount
	splitAmount := originalAmount / int64(numProviders)
	remainder := originalAmount % int64(numProviders)

	newSteps := make([]*internalv1.PaymentStep, 0, numProviders)
	for i, providerName := range validSplitProviders {
		currentAmount := splitAmount
		if i == 0 && remainder > 0 { // Add remainder to the first split step
			currentAmount += remainder
		}

		newStep := &internalv1.PaymentStep{
			StepId:       uuid.NewString(),
			ProviderName: providerName, // Already trimmed
			Amount:       currentAmount,
			Currency:     originalStep.Currency,
			IsFanOut:     false,
			Metadata:     make(map[string]string),
			ProviderPayload: make(map[string]string),
		}

		if originalStep.Metadata != nil {
		    for k, v := range originalStep.Metadata {
			if k != "split_payment_evenly_to_providers" {
				newStep.Metadata[k] = v
			}
		    }
		}
		newStep.Metadata["original_step_id_for_split"] = originalStep.StepId

		if originalStep.ProviderPayload != nil {
		    for k,v := range originalStep.ProviderPayload {
		        newStep.ProviderPayload[k] = v
		    }
		}

		newSteps = append(newSteps, newStep)
	}

	// This check should ideally not be needed if logic above guarantees split or returns original plan
	// However, keeping it as a safeguard or if future logic paths could result in empty newSteps.
	if len(newSteps) > 0 {
	    return &internalv1.PaymentPlan{
		PlanId: plan.PlanId,
		Steps:  newSteps,
	    }, nil
	}

	// Should technically be unreachable if splitProviders had < 2 elements,
	// as that returns the original plan. If processing somehow yields no new steps
	// (e.g., if numProviders was 0, though caught by len(validSplitProviders) < 2),
	// then returning original plan is safest.
	return plan, nil
}
