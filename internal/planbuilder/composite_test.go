package planbuilder

import (
	"testing"

	"github.com/google/uuid"
	"github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStubCompositePaymentService(t *testing.T) {
	svc := NewStubCompositePaymentService()
	assert.NotNil(t, svc)
}

func TestStubCompositePaymentService_Optimize(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{MerchantID: "testMerchant"}
	inputPlan := &orchestratorinternalv1.PaymentPlan{
		PlanId: uuid.NewString(),
		Steps: []*orchestratorinternalv1.PaymentStep{
			{StepId: uuid.NewString(), ProviderName: "stripe", Amount: 1000, Currency: "USD"},
		},
	}

	optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
	require.NoError(t, err)
	require.NotNil(t, optimizedPlan)
	assert.Equal(t, inputPlan, optimizedPlan, "Stub optimizer should return the input plan")
}
