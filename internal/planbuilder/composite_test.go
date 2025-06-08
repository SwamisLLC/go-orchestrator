package planbuilder

import (
	"testing"

	// "github.com/google/uuid" // No longer needed directly in tests for composite
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStubCompositePaymentService_Defaults(t *testing.T) {
	svcDefault := NewStubCompositePaymentService()
	assert.NotNil(t, svcDefault)
	assert.Equal(t, []string{"default-split-provider-1", "default-split-provider-2"}, svcDefault.DefaultSplitProviders, "Default providers if no args")

	svcEmptyStr := NewStubCompositePaymentService("")
	assert.Equal(t, []string{"default-split-provider-1", "default-split-provider-2"}, svcEmptyStr.DefaultSplitProviders, "Default providers if empty string arg")

	svcSingle := NewStubCompositePaymentService("custom1")
	assert.Equal(t, []string{"custom1"}, svcSingle.DefaultSplitProviders, "Single custom provider")

	svcMultiple := NewStubCompositePaymentService("custom1", "custom2")
	assert.Equal(t, []string{"custom1", "custom2"}, svcMultiple.DefaultSplitProviders, "Multiple custom providers")

	svcWithEmpty := NewStubCompositePaymentService("custom1", "", "custom2")
	assert.Equal(t, []string{"custom1", "custom2"}, svcWithEmpty.DefaultSplitProviders, "Custom providers with empty string filtered out")
}


func TestStubCompositePaymentService_Optimize_NoSplit(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{MerchantID: "testMerchant"}

	t.Run("NilPlan", func(t *testing.T) {
		optimizedPlan, err := svc.Optimize(domainCtx, nil)
		require.NoError(t, err)
		assert.Nil(t, optimizedPlan, "Nil plan should return nil plan")
	})

	t.Run("EmptyPlanSteps", func(t *testing.T) {
		inputPlan := &internalv1.PaymentPlan{PlanId: "emptySteps", Steps: []*internalv1.PaymentStep{}}
		optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
		require.NoError(t, err)
		assert.Equal(t, inputPlan, optimizedPlan, "Plan with no steps should not change")
	})

	t.Run("NoMetadataForSplit", func(t *testing.T) {
		inputPlan := &internalv1.PaymentPlan{
			PlanId: "plan1",
			Steps: []*internalv1.PaymentStep{
				{StepId: "step1", ProviderName: "stripe", Amount: 1000, Currency: "USD", Metadata: make(map[string]string)},
			},
		}
		optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
		require.NoError(t, err)
		assert.Equal(t, inputPlan, optimizedPlan, "Plan should not change if no split metadata")
	})

	t.Run("MetadataPresentButNotSplitKey", func(t *testing.T) {
		inputPlan := &internalv1.PaymentPlan{
			PlanId: "plan_other_meta",
			Steps:  []*internalv1.PaymentStep{{StepId: "s1", ProviderName: "stripe", Amount: 1000, Metadata: map[string]string{"other_key": "value"}}},
		}
		optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
		require.NoError(t, err)
		assert.Equal(t, inputPlan, optimizedPlan, "Plan should not change if split key not present")
	})


	t.Run("AlreadyMultipleSteps", func(t *testing.T) {
		inputPlan := &internalv1.PaymentPlan{
			PlanId: "plan2",
			Steps: []*internalv1.PaymentStep{
				{StepId: "step1", ProviderName: "stripe", Amount: 500, Currency: "USD", Metadata: map[string]string{"split_payment_evenly_to_providers": "p1,p2"}},
				{StepId: "step2", ProviderName: "adyen", Amount: 500, Currency: "USD"},
			},
		}
		optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
		require.NoError(t, err)
		assert.Equal(t, inputPlan, optimizedPlan, "Plan should not change if already multiple steps")
	})

	t.Run("SplitProvidersInMetadataButLessThanTwoValid", func(t *testing.T) {
		inputPlan := &internalv1.PaymentPlan{
			PlanId: "plan_single_split_provider",
			Steps:  []*internalv1.PaymentStep{{StepId: "s1", ProviderName: "stripe", Amount: 1000, Metadata: map[string]string{"split_payment_evenly_to_providers": "paypal"}}},
		}
		optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
		require.NoError(t, err)
		assert.Equal(t, inputPlan, optimizedPlan, "Plan should not change if only one provider in split list")

		inputPlan2 := &internalv1.PaymentPlan{
			PlanId: "plan_single_split_provider_with_comma",
			Steps:  []*internalv1.PaymentStep{{StepId: "s1", ProviderName: "stripe", Amount: 1000, Metadata: map[string]string{"split_payment_evenly_to_providers": "paypal,"}}},
		}
		optimizedPlan2, err := svc.Optimize(domainCtx, inputPlan2)
		require.NoError(t, err)
		assert.Equal(t, inputPlan2, optimizedPlan2, "Plan should not change if only one valid provider after trim and split")

		inputPlan3 := &internalv1.PaymentPlan{
			PlanId: "plan_empty_split_string",
			Steps:  []*internalv1.PaymentStep{{StepId: "s1", ProviderName: "stripe", Amount: 1000, Metadata: map[string]string{"split_payment_evenly_to_providers": ",,"}}},
		}
		optimizedPlan3, err := svc.Optimize(domainCtx, inputPlan3)
		require.NoError(t, err)
		assert.Equal(t, inputPlan3, optimizedPlan3, "Plan should not change if split string results in no valid providers")
	})
}

func TestStubCompositePaymentService_Optimize_WithSplit_Even(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{MerchantID: "testMerchant"}
	originalStepID := "original_step_1"
	inputPlan := &internalv1.PaymentPlan{
		PlanId: "plan_even_split",
		Steps: []*internalv1.PaymentStep{
			{
				StepId:   originalStepID, ProviderName: "stripe", Amount: 1000, Currency: "USD",
				Metadata: map[string]string{"split_payment_evenly_to_providers": "paypal, square ", "other_meta": "value"}, // Note space after square
				ProviderPayload: map[string]string{"token": "tok_original"},
			},
		},
	}

	optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
	require.NoError(t, err)
	require.NotNil(t, optimizedPlan)
	assert.Equal(t, inputPlan.PlanId, optimizedPlan.PlanId)
	require.Len(t, optimizedPlan.Steps, 2, "Plan should be split into 2 steps")

	step1 := optimizedPlan.Steps[0]
	assert.NotEqual(t, originalStepID, step1.StepId)
	assert.Equal(t, "paypal", step1.ProviderName)
	assert.Equal(t, int64(500), step1.Amount)
	assert.Equal(t, "USD", step1.Currency)
	assert.Equal(t, "value", step1.Metadata["other_meta"])
	assert.Equal(t, originalStepID, step1.Metadata["original_step_id_for_split"])
	assert.Equal(t, "tok_original", step1.ProviderPayload["token"])
	_, splitInstructionFound := step1.Metadata["split_payment_evenly_to_providers"]
	assert.False(t, splitInstructionFound, "Split instruction should be removed from new steps")


	step2 := optimizedPlan.Steps[1]
	assert.NotEqual(t, originalStepID, step2.StepId)
	assert.Equal(t, "square", step2.ProviderName) // Expect provider name to be trimmed
	assert.Equal(t, int64(500), step2.Amount)
	assert.Equal(t, "USD", step2.Currency)
	assert.Equal(t, "value", step2.Metadata["other_meta"])
	assert.Equal(t, originalStepID, step2.Metadata["original_step_id_for_split"])
	assert.Equal(t, "tok_original", step2.ProviderPayload["token"])
}

func TestStubCompositePaymentService_Optimize_WithSplit_Uneven_Remainder(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{MerchantID: "testMerchant"}
	inputPlan := &internalv1.PaymentPlan{
		PlanId: "plan_uneven_split",
		Steps: []*internalv1.PaymentStep{
			{StepId: "original_s2", ProviderName: "stripe", Amount: 1001, Currency: "EUR", Metadata: map[string]string{"split_payment_evenly_to_providers": "p1, p2,p3 "}}, // Mixed spacing
		},
	}

	optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
	require.NoError(t, err)
	require.Len(t, optimizedPlan.Steps, 3, "Plan should be split into 3 steps")

    // 1001 / 3 = 333 remainder 2
	assert.Equal(t, "p1", optimizedPlan.Steps[0].ProviderName)
	assert.Equal(t, int64(333+2), optimizedPlan.Steps[0].Amount, "First step should get remainder")
	assert.Equal(t, "EUR", optimizedPlan.Steps[0].Currency)

	assert.Equal(t, "p2", optimizedPlan.Steps[1].ProviderName)
	assert.Equal(t, int64(333), optimizedPlan.Steps[1].Amount)

	assert.Equal(t, "p3", optimizedPlan.Steps[2].ProviderName)
	assert.Equal(t, int64(333), optimizedPlan.Steps[2].Amount)

    totalSplitAmount := optimizedPlan.Steps[0].Amount + optimizedPlan.Steps[1].Amount + optimizedPlan.Steps[2].Amount
    assert.Equal(t, inputPlan.Steps[0].Amount, totalSplitAmount, "Total of split amounts must equal original amount")
}

func TestStubCompositePaymentService_Optimize_WithSplit_FromDomainContextUserPrefs(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{
	    MerchantID: "merchant_with_prefs",
	    UserPreferences: map[string]string{"split_payment_evenly_to_providers": "pref_p1, pref_p2"}, // Space in prefs
	}
	originalStepID := "original_step_prefs"
	inputPlan := &internalv1.PaymentPlan{
		PlanId: "plan_split_via_prefs",
		Steps: []*internalv1.PaymentStep{
			// Step metadata does NOT contain the split instruction
			{StepId: originalStepID, ProviderName: "stripe", Amount: 200, Currency: "CAD", Metadata: map[string]string{"other_meta": "step_specific"}},
		},
	}

	optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
	require.NoError(t, err)
	require.NotNil(t, optimizedPlan)
	assert.Equal(t, inputPlan.PlanId, optimizedPlan.PlanId)
	require.Len(t, optimizedPlan.Steps, 2, "Plan should be split into 2 steps based on user prefs")

	assert.Equal(t, "pref_p1", optimizedPlan.Steps[0].ProviderName)
	assert.Equal(t, int64(100), optimizedPlan.Steps[0].Amount)
	assert.Equal(t, originalStepID, optimizedPlan.Steps[0].Metadata["original_step_id_for_split"])
	assert.Equal(t, "step_specific", optimizedPlan.Steps[0].Metadata["other_meta"]) // Ensure other metadata is preserved

	assert.Equal(t, "pref_p2", optimizedPlan.Steps[1].ProviderName)
	assert.Equal(t, int64(100), optimizedPlan.Steps[1].Amount)
}

func TestStubCompositePaymentService_Optimize_StepMetadataOverridesUserPrefs(t *testing.T) {
	svc := NewStubCompositePaymentService()
	domainCtx := context.DomainContext{
	    MerchantID: "merchant_override",
	    UserPreferences: map[string]string{"split_payment_evenly_to_providers": "pref_p1,pref_p2"}, // Prefs exist
	}
	originalStepID := "original_step_override"
	inputPlan := &internalv1.PaymentPlan{
		PlanId: "plan_split_override",
		Steps: []*internalv1.PaymentStep{
			// Step metadata DOES contain the split instruction, should take precedence
			{StepId: originalStepID, ProviderName: "stripe", Amount: 300, Currency: "JPY", Metadata: map[string]string{"split_payment_evenly_to_providers": "meta_p1,meta_p2,meta_p3"}},
		},
	}

	optimizedPlan, err := svc.Optimize(domainCtx, inputPlan)
	require.NoError(t, err)
	require.Len(t, optimizedPlan.Steps, 3, "Plan should be split into 3 steps based on step metadata")

	assert.Equal(t, "meta_p1", optimizedPlan.Steps[0].ProviderName)
	assert.Equal(t, int64(100), optimizedPlan.Steps[0].Amount)
	assert.Equal(t, "meta_p2", optimizedPlan.Steps[1].ProviderName)
	assert.Equal(t, int64(100), optimizedPlan.Steps[1].Amount)
	assert.Equal(t, "meta_p3", optimizedPlan.Steps[2].ProviderName)
	assert.Equal(t, int64(100), optimizedPlan.Steps[2].Amount)
}
