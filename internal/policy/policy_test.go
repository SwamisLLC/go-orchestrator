package policy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/yourorg/payment-orchestrator/internal/policy"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

func TestNewPaymentPolicyEnforcer(t *testing.T) {
	t.Run("no rules", func(t *testing.T) {
		ppe, err := policy.NewPaymentPolicyEnforcer(nil)
		require.NoError(t, err)
		assert.NotNil(t, ppe)
	})

	t.Run("valid rule", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "TestRule", Expression: "amount_units > 100"},
		}
		ppe, err := policy.NewPaymentPolicyEnforcer(rules)
		require.NoError(t, err)
		assert.NotNil(t, ppe)
	})

	t.Run("invalid rule expression", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "InvalidRule", Expression: "amount_units > #100"}, // Invalid character #
		}
		_, err := policy.NewPaymentPolicyEnforcer(rules)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to compile rule 'InvalidRule'")
	})

	t.Run("rule with empty name", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "", Expression: "amount_units > 100"},
		}
		ppe, err := policy.NewPaymentPolicyEnforcer(rules)
		require.NoError(t, err) // Should skip and not error for the whole process
		assert.NotNil(t, ppe)
		// TODO: Could add a way to inspect loaded rules count if necessary via a helper or method on PPE
	})

	t.Run("rule with empty expression", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "TestRuleEmptyExpr", Expression: ""},
		}
		ppe, err := policy.NewPaymentPolicyEnforcer(rules)
		require.NoError(t, err)
		assert.NotNil(t, ppe)
	})
}

func TestPaymentPolicyEnforcer_Evaluate(t *testing.T) {
	domainCtxBase := context.DomainContext{
		MerchantID: "merchant-123",
		UserPreferences: map[string]string{
			"fraud_score":    "0.3",
			"default_region": "us",
		},
		ActiveMerchantConfig: context.MerchantConfig{
			UserPrefs: map[string]string{"merchant_tier": "gold"},
		},
	}

	stepBase := &orchestratorinternalv1.PaymentStep{
		StepId:       "step-abc",
		ProviderName: "stripe",
		Amount:       1500, // Assuming direct int64 amount
		Currency:     "USD",
		Metadata:     map[string]string{"region": "ca"},
	}

	stepResultBase := &orchestratorinternalv1.StepResult{
		StepId:    "step-abc",
		Success:   true,
		ErrorCode: "",
	}

	stepCtxBase := context.StepExecutionContext{
		TraceID:       "trace-xyz",
		AttemptNumber: 1,
		StartTime:     time.Now(),
	}

	t.Run("no rules defined, default behavior", func(t *testing.T) {
		ppe, _ := policy.NewPaymentPolicyEnforcer(nil)
		decision, err := ppe.Evaluate(domainCtxBase, stepCtxBase, stepBase, stepResultBase)
		require.NoError(t, err)
		assert.False(t, decision.AllowRetry, "Should not allow retry for successful step by default")
		assert.Equal(t, "PROCEED", decision.NextAction)
		assert.Contains(t, decision.Reason, "Default policy outcome") // Success makes retry false

		failedStepResult := &orchestratorinternalv1.StepResult{Success: false, ErrorCode: "failed"}
		decision, err = ppe.Evaluate(domainCtxBase, stepCtxBase, stepBase, failedStepResult)
		require.NoError(t, err)
		assert.True(t, decision.AllowRetry, "Should allow retry for failed step on first attempt by default")
		assert.Contains(t, decision.Reason, "Default: Allow retry on first failure")
	})

	t.Run("RetryRule allows retry", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "RetryRule", Expression: "step_error_code == 'network_error' && attempt_number < 3"},
		}
		ppe, _ := policy.NewPaymentPolicyEnforcer(rules)

		failedStepResult := &orchestratorinternalv1.StepResult{Success: false, ErrorCode: "network_error"}
		stepCtxAttempt2 := context.StepExecutionContext{AttemptNumber: 2}

		decision, err := ppe.Evaluate(domainCtxBase, stepCtxAttempt2, stepBase, failedStepResult)
		require.NoError(t, err)
		assert.True(t, decision.AllowRetry)
		assert.Equal(t, "RetryRule allowed retry.", decision.Reason)
	})

	t.Run("RetryRule denies retry", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "RetryRule", Expression: "step_error_code == 'network_error' && attempt_number < 2"}, // Only 1 attempt allowed by rule
		}
		ppe, _ := policy.NewPaymentPolicyEnforcer(rules)

		failedStepResult := &orchestratorinternalv1.StepResult{Success: false, ErrorCode: "network_error"}
		stepCtxAttempt2 := context.StepExecutionContext{AttemptNumber: 2} // This is the 2nd attempt

		decision, err := ppe.Evaluate(domainCtxBase, stepCtxAttempt2, stepBase, failedStepResult)
		require.NoError(t, err)
		assert.False(t, decision.AllowRetry)
		assert.Contains(t, decision.Metadata["RetryRuleOutcome"], "AllowRetry=false")
	})

	t.Run("FraudRule blocks transaction", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "FraudRule", Expression: `(fraud_score > 0.8 || step_error_code == "card_fraudulent") ? "BLOCK" : "PROCEED"`},
		}
		ppe, _ := policy.NewPaymentPolicyEnforcer(rules)

		highFraudDomainCtx := context.DomainContext{UserPreferences: map[string]string{"fraud_score": "0.9"}}
		stepResult := &orchestratorinternalv1.StepResult{Success: false, ErrorCode: "some_error"}


		decision, err := ppe.Evaluate(highFraudDomainCtx, stepCtxBase, stepBase, stepResult)
		require.NoError(t, err)
		assert.Equal(t, "BLOCK", decision.NextAction)
		assert.False(t, decision.AllowRetry, "Should not allow retry if blocked")
		assert.True(t, decision.SkipFallback, "Should skip fallback if blocked")
		assert.Contains(t, decision.Reason, "FraudRule action: BLOCK")
	})

	t.Run("EscalationRule triggers manual review", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "EscalationRule", Expression: `amount_units > 100000 && region == "international"`}, // Escalate large international payments
		}
		ppe, _ := policy.NewPaymentPolicyEnforcer(rules)

		stepForEscalation := &orchestratorinternalv1.PaymentStep{
			Amount: 150000, Currency: "USD", Metadata: map[string]string{"region": "international"},
		}
		stepResult := &orchestratorinternalv1.StepResult{Success: true} // Even if successful

		decision, err := ppe.Evaluate(domainCtxBase, stepCtxBase, stepForEscalation, stepResult)
		require.NoError(t, err)
		assert.True(t, decision.EscalateManual)
		assert.Contains(t, decision.Reason, "EscalationRule triggered manual review.")
	})

	t.Run("Successful step overrides RetryRule to false", func(t *testing.T) {
		rules := []policy.RuleConfig{
			{Name: "RetryRule", Expression: "true"}, // Rule always says retry
		}
		ppe, _ := policy.NewPaymentPolicyEnforcer(rules)

		successfulStepResult := &orchestratorinternalv1.StepResult{Success: true}
		decision, err := ppe.Evaluate(domainCtxBase, stepCtxBase, stepBase, successfulStepResult)
		require.NoError(t, err)
		assert.False(t, decision.AllowRetry, "Retry should be false for successful steps regardless of rules")
		assert.Contains(t, decision.Reason, "(Retry overridden for successful step)")
	})
}
