// Package planbuilder is responsible for constructing a PaymentPlan based on
// merchant configuration and the incoming request context. It may involve
// invoking domain services like a CompositePaymentService for fee and risk
// optimization to determine the sequence of payment steps.
// Refer to spec2_1.md, section 7.1 for more details on PlanBuilder.
package planbuilder

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	orchestratorexternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
)

var (
	planRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "plan_requests_total",
		Help: "Total number of plan requests.",
	})
	planBuildDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "plan_build_duration_seconds",
		Help: "Duration of plan build operations in seconds.",
	})
)

// GetPlanRequestsTotal returns the planRequestsTotal metric for testing.
func GetPlanRequestsTotal() prometheus.Counter {
	return planRequestsTotal
}

// GetPlanBuildDurationSeconds returns the planBuildDurationSeconds metric for testing.
func GetPlanBuildDurationSeconds() prometheus.Histogram {
	return planBuildDurationSeconds
}

// PlanBuilder constructs a PaymentPlan from merchant config + incoming context.
type PlanBuilder struct {
	merchantRepo     context.MerchantConfigRepository // To fetch merchant specific settings
	compositeService CompositePaymentService          // For fee & risk optimization
}

// NewPlanBuilder creates a new PlanBuilder.
func NewPlanBuilder(repo context.MerchantConfigRepository, compositeSvc CompositePaymentService) *PlanBuilder {
	if repo == nil {
		// Fallback to a default in-memory repo if none provided, or handle error
		// For now, let's assume it's required.
		panic("MerchantConfigRepository cannot be nil")
	}
	if compositeSvc == nil {
	    panic("CompositePaymentService cannot be nil")
	}
	return &PlanBuilder{
		merchantRepo:     repo,
		compositeService: compositeSvc,
	}
}

// Build constructs a payment plan.
// For this basic version, it creates a single-step plan using the merchant's default provider.
// It also calls the composite service's Optimize method.
func (b *PlanBuilder) Build(domainCtx context.DomainContext, extReq *orchestratorexternalv1.ExternalRequest) (*orchestratorinternalv1.PaymentPlan, error) {
	// Increment total requests counter
	planRequestsTotal.Inc()

	// Start timer for duration histogram
	timer := prometheus.NewTimer(planBuildDurationSeconds)
	defer timer.ObserveDuration()

	// Get a tracer instance
	tracer := otel.Tracer("planbuilder")

	// Start a new span
	ctx, span := tracer.Start(domainCtx.TraceContext.Context(), "PlanBuilder.Build")
	defer span.End()

	// Update domainCtx with the new context from the span
	domainCtx.TraceContext = context.NewTraceContext(ctx, domainCtx.TraceContext.TraceID())


	if extReq == nil {
		return nil, fmt.Errorf("external request cannot be nil")
	}

	merchantCfg := domainCtx.ActiveMerchantConfig

	// Basic plan: one step with the default provider and amount from request
	rawStep := &orchestratorinternalv1.PaymentStep{
		StepId:       uuid.NewString(),
		ProviderName: merchantCfg.DefaultProvider,
		Amount:       extReq.Amount,    // Amount from the external request
		Currency:     extReq.Currency,  // Currency from the external request
		IsFanOut:     false,
		Metadata:     make(map[string]string), // Initialize metadata map
		ProviderPayload: make(map[string]string), // Initialize provider payload map
	}

	// Ensure metadata and provider_payload are not nil, even if empty
	if rawStep.Metadata == nil {
	    rawStep.Metadata = make(map[string]string)
	}
	if rawStep.ProviderPayload == nil {
	    rawStep.ProviderPayload = make(map[string]string)
	}


	initialPlan := &orchestratorinternalv1.PaymentPlan{
		PlanId: uuid.NewString(),
		Steps:  []*orchestratorinternalv1.PaymentStep{rawStep},
	}

	// Call the composite service to potentially optimize the plan
	optimizedPlan, err := b.compositeService.Optimize(domainCtx, initialPlan)
	if err != nil {
		return nil, fmt.Errorf("failed to optimize payment plan: %w", err)
	}

	// Ensure steps are not nil after optimization
	if optimizedPlan != nil && optimizedPlan.Steps != nil {
	    for _, step := range optimizedPlan.Steps {
	        if step.Metadata == nil {
	            step.Metadata = make(map[string]string)
	        }
	        if step.ProviderPayload == nil {
	            step.ProviderPayload = make(map[string]string)
	        }
	    }
	}


	return optimizedPlan, nil
}
