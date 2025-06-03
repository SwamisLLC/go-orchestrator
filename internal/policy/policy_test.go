package policy

import (
	"testing"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/context"
	// internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPaymentPolicyEnforcer(t *testing.T) {
	ppe := NewPaymentPolicyEnforcer()
	assert.NotNil(t, ppe)
}

func TestPaymentPolicyEnforcer_Evaluate_Stub(t *testing.T) {
	ppe := NewPaymentPolicyEnforcer()

	domainCtx := context.DomainContext{
		MerchantID: "test-merchant",
	}

	// Create a dummy PaymentStep if it were used by the Evaluate method.
	// For the current stub, it's not, but good to have for future.
	// dummyStep := &internalv1.PaymentStep{ /* ... fields ... */ }

	dummyStepCtx := context.StepExecutionContext{
		TraceID: "trace-123",
		SpanID: "span-456",
		StartTime: time.Now(),
		RemainingBudgetMs: 5000,
	}

	decision, err := ppe.Evaluate(domainCtx, dummyStepCtx) // Pass dummyStep if signature changes

	require.NoError(t, err)
	assert.True(t, decision.AllowRetry, "Stub policy should allow retry")
	assert.False(t, decision.SkipFallback, "Stub policy should not skip fallback")
	assert.False(t, decision.EscalateManual, "Stub policy should not escalate manually")
}
