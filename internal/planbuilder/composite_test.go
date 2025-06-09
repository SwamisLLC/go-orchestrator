package planbuilder_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/planbuilder"
	// commonv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/commonv1" // No longer needed
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

func TestStubCompositePaymentService_Optimize(t *testing.T) {
	service := planbuilder.NewStubCompositePaymentService()

	createDomainCtx := func(enableSplit bool, splitParts string) context.DomainContext {
		featureFlags := make(map[string]bool)
		userPrefs := make(map[string]string)
		if enableSplit {
			featureFlags["enable_payment_splitting"] = true
		} else {
			featureFlags["enable_payment_splitting"] = false
		}
		if splitParts != "" {
			userPrefs["split_parts"] = splitParts
		}
		return context.DomainContext{
			FeatureFlags:    featureFlags,
			UserPreferences: userPrefs,
		}
	}

	// Updated Helper: pmt (PaymentMethodType) is removed as it's not on PaymentStep
	createSingleStepPlan := func(planID, stepID, provider, currency string, amount int64, meta map[string]string, payload map[string]string) *orchestratorinternalv1.PaymentPlan {
		return &orchestratorinternalv1.PaymentPlan{
			PlanId:    planID, // Was PaymentId
			Steps: []*orchestratorinternalv1.PaymentStep{
				{
					StepId:            stepID,
					ProviderName:      provider,
					Amount:            amount,   // Now int64
					Currency:          currency, // Now string
					Metadata:          meta,
					ProviderPayload:   payload,
					// PaymentMethodType removed
				},
			},
		}
	}

	t.Run("no split if feature flag not present", func(t *testing.T) {
		domainCtx := context.DomainContext{UserPreferences: map[string]string{"split_parts": "2"}}
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
		assert.Equal(t, 1, len(optimizedPlan.Steps))
	})

	t.Run("no split if feature flag is false", func(t *testing.T) {
		domainCtx := createDomainCtx(false, "2")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if split_parts preference not present", func(t *testing.T) {
		domainCtx := context.DomainContext{FeatureFlags: map[string]bool{"enable_payment_splitting": true}}
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if split_parts is invalid (abc)", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "abc")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if split_parts is 1", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "1")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("successful split into 2 equal parts", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "2")
		originalMeta := map[string]string{"orig_key": "orig_val"}
		originalPayload := map[string]string{"payload_key": "payload_val"}
		// Last args to createSingleStepPlan were pmt, amount, meta, payload. Pmt removed.
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, originalMeta, originalPayload)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		require.NotNil(t, optimizedPlan)
		require.Len(t, optimizedPlan.Steps, 2, "Plan should be split into 2 steps")

		totalAmount := int64(0)
		for i, step := range optimizedPlan.Steps {
			assert.NotEqual(t, "s1", step.GetStepId(), "Split steps should have new IDs")
			assert.Equal(t, "stripe", step.GetProviderName())
			assert.Equal(t, "USD", step.GetCurrency())
			// assert.Equal(t, "card_generic", step.GetPaymentMethodType()) // Removed
			totalAmount += step.GetAmount()

			assert.Equal(t, "s1", step.GetMetadata()["original_step_id"])
			assert.Equal(t, fmt.Sprintf("%d", i+1), step.GetMetadata()["split_part_index"])
			assert.Equal(t, "2", step.GetMetadata()["total_split_parts"])
			assert.Equal(t, "orig_val", step.GetMetadata()["orig_key"], "Original metadata should be preserved")

			assert.Equal(t, "payload_val", step.GetProviderPayload()["payload_key"], "Original provider payload should be preserved")
		}
		assert.Equal(t, int64(1000), totalAmount, "Total amount of split steps should equal original amount")
		assert.Equal(t, int64(500), optimizedPlan.Steps[0].GetAmount())
		assert.Equal(t, int64(500), optimizedPlan.Steps[1].GetAmount())
		assert.Equal(t, originalPlan.GetPlanId(), optimizedPlan.GetPlanId(), "PlanId should be preserved")
	})

	t.Run("successful split into 3 parts with remainder", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "3")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 1000, nil, nil)

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		require.Len(t, optimizedPlan.Steps, 3)

		totalAmount := int64(0)
		for i, step := range optimizedPlan.Steps {
			totalAmount += step.GetAmount()
			assert.Equal(t, "s1", step.GetMetadata()["original_step_id"])
			assert.Equal(t, fmt.Sprintf("%d", i+1), step.GetMetadata()["split_part_index"])
			assert.Equal(t, "3", step.GetMetadata()["total_split_parts"])
		}
		assert.Equal(t, int64(1000), totalAmount)
		assert.Equal(t, int64(334), optimizedPlan.Steps[0].GetAmount())
		assert.Equal(t, int64(333), optimizedPlan.Steps[1].GetAmount())
		assert.Equal(t, int64(333), optimizedPlan.Steps[2].GetAmount())
	})

	t.Run("no split if original plan is nil", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "2")
		optimizedPlan, err := service.Optimize(domainCtx, nil)
		require.NoError(t, err)
		assert.Nil(t, optimizedPlan, "Nil plan in should result in nil plan out")
	})

	t.Run("no split if original plan has no steps", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "2")
		originalPlan := &orchestratorinternalv1.PaymentPlan{PlanId: "p1", Steps: []*orchestratorinternalv1.PaymentStep{}}

		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if original plan has multiple steps", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "2")
		originalPlan := &orchestratorinternalv1.PaymentPlan{
			PlanId: "p1",
			Steps: []*orchestratorinternalv1.PaymentStep{
				{StepId: "s1", Amount: 500, Currency: "USD"},
				{StepId: "s2", Amount: 500, Currency: "USD"},
			},
		}
		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if original step amount is nil (not possible with int64, checking zero)", func(t *testing.T) {
		// With int64, Amount cannot be nil. This test becomes same as "original amount is zero".
		domainCtx := createDomainCtx(true, "2")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 0, nil, nil)
		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if original amount is zero", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "2")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 0, nil, nil)
		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan)
	})

	t.Run("no split if amount too small to be split meaningfully", func(t *testing.T) {
		domainCtx := createDomainCtx(true, "3")
		originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 2, nil, nil)
		optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
		require.NoError(t, err)
		assert.Same(t, originalPlan, optimizedPlan, "Plan should not split if amount is too small for parts")
	})

    t.Run("split with minimal amount (3 units, 3 parts)", func(t *testing.T) {
        domainCtx := createDomainCtx(true, "3")
        originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", 3, nil, nil)
        optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
        require.NoError(t, err)
        require.Len(t, optimizedPlan.Steps, 3)
        assert.Equal(t, int64(1), optimizedPlan.Steps[0].GetAmount())
        assert.Equal(t, int64(1), optimizedPlan.Steps[1].GetAmount())
        assert.Equal(t, int64(1), optimizedPlan.Steps[2].GetAmount())
    })

    // PaymentMethodType is not on Step, so this test is removed/not applicable.
    // t.Run("split preserves original PaymentMethodType", func(t *testing.T) {
    //     domainCtx := createDomainCtx(true, "2")
    //     originalPlan := createSingleStepPlan("p1", "s1", "stripe", "USD", "ideal", 1000, nil, nil)

    //     optimizedPlan, err := service.Optimize(domainCtx, originalPlan)
    //     require.NoError(t, err)
    //     require.Len(t, optimizedPlan.Steps, 2)
    //     assert.Equal(t, "ideal", optimizedPlan.Steps[0].GetPaymentMethodType())
    //     assert.Equal(t, "ideal", optimizedPlan.Steps[1].GetPaymentMethodType())
    // })
}
