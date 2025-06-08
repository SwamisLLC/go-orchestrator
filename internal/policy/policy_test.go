package policy

import (
	"testing"
	"time" // Keep for stepCtx.StartTime if used by old tests.
	"github.com/yourorg/payment-orchestrator/internal/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPaymentPolicyEnforcer_EmptyAndNilRules(t *testing.T) {
	ppe, err := NewPaymentPolicyEnforcer(nil)
	require.NoError(t, err)
	assert.NotNil(t, ppe)
	assert.Empty(t, ppe.rules)

	ppe, err = NewPaymentPolicyEnforcer([]PolicyRule{})
	require.NoError(t, err)
	assert.NotNil(t, ppe)
	assert.Empty(t, ppe.rules)
}

func TestNewPaymentPolicyEnforcer_CompilationError(t *testing.T) {
    rules := []PolicyRule{
        {ID: "rule1", Expression: "amount > 100", Decision: PolicyDecision{AllowRetry: false}},
        {ID: "rule2", Expression: "region ==", Decision: PolicyDecision{SkipFallback: true}}, // Invalid expression
    }
    _, err := NewPaymentPolicyEnforcer(rules)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "failed to compile rule ID 'rule2'")
    assert.Contains(t, err.Error(), "Unexpected end of expression") // More stable check
}

func TestNewPaymentPolicyEnforcer_EmptyExpressionInRule(t *testing.T) {
    rules := []PolicyRule{{ID: "empty_expr_rule", Expression: "", Decision: PolicyDecision{AllowRetry: false}}}
    _, err := NewPaymentPolicyEnforcer(rules)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "policy rule ID 'empty_expr_rule' has an empty expression")
}


func TestPaymentPolicyEnforcer_Evaluate_DSL(t *testing.T) {
	rules := []PolicyRule{
		{
			ID: "high_fraud_escalate", Expression: "fraudScore > 0.8", Priority: 1,
			Decision: PolicyDecision{AllowRetry: false, SkipFallback: true, EscalateManual: true},
		},
		{
			ID: "large_amount_eu_no_retry", Expression: "amount >= 100000 && region == 'EU'", Priority: 2, // amount in cents
			Decision: PolicyDecision{AllowRetry: false, SkipFallback: false, EscalateManual: false},
		},
		{
			ID: "silver_tier_allow_retry", Expression: "merchantTier == 'silver'", Priority: 3,
			Decision: PolicyDecision{AllowRetry: true, SkipFallback: false, EscalateManual: false},
		},
	}
	ppe, err := NewPaymentPolicyEnforcer(rules) // Note: rules are not sorted by priority yet
	require.NoError(t, err)

	domainCtx := context.DomainContext{MerchantID: "m1"}

	t.Run("NoRuleMatches_DefaultDecision", func(t *testing.T) {
		stepCtx := context.StepExecutionContext{
			AmountCents: 5000, Currency: "USD", Region: "US", MerchantTier: "bronze", FraudScore: 0.1,
		}
		decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
		require.NoError(t, errEval)
		assert.True(t, decision.AllowRetry)
		assert.False(t, decision.SkipFallback)
		assert.False(t, decision.EscalateManual)
	})

	t.Run("HighFraudRuleMatches", func(t *testing.T) {
		stepCtx := context.StepExecutionContext{
			AmountCents: 1000, Currency: "USD", Region: "US", MerchantTier: "gold", FraudScore: 0.9,
		}
		decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
		require.NoError(t, errEval)
		assert.False(t, decision.AllowRetry)
		assert.True(t, decision.SkipFallback)
		assert.True(t, decision.EscalateManual)
	})

	t.Run("LargeAmountEURuleMatches", func(t *testing.T) {
		// This rule is second. If high fraud also matched, high fraud would win.
		stepCtx := context.StepExecutionContext{
			AmountCents: 100000, Currency: "EUR", Region: "EU", MerchantTier: "gold", FraudScore: 0.2, // Low fraud
		}
		decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
		require.NoError(t, errEval)
		assert.False(t, decision.AllowRetry)
		assert.False(t, decision.SkipFallback)
		assert.False(t, decision.EscalateManual)
	})

	t.Run("SilverTierRuleMatches", func(t *testing.T) {
		// This rule is third. It only wins if higher priority rules (lower Prio number, or earlier in list) don't match.
		stepCtx := context.StepExecutionContext{
			AmountCents: 500, Currency: "USD", Region: "US", MerchantTier: "silver", FraudScore: 0.1, // Low fraud
		}
		decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
		require.NoError(t, errEval)
		assert.True(t, decision.AllowRetry)
		assert.False(t, decision.SkipFallback)
		assert.False(t, decision.EscalateManual)
	})

	t.Run("FirstMatchingRuleWins_HighFraudBeforeSilver", func(t *testing.T) {
		// Rules are evaluated in order. HighFraud is first.
		stepCtx := context.StepExecutionContext{
			AmountCents: 500, Currency: "USD", Region: "US", MerchantTier: "silver", FraudScore: 0.9, // High fraud
		}
		decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
		require.NoError(t, errEval)
		assert.False(t, decision.AllowRetry)   // From high_fraud_escalate
		assert.True(t, decision.SkipFallback)  // From high_fraud_escalate
		assert.True(t, decision.EscalateManual) // From high_fraud_escalate
	})

	t.Run("EvaluationError_UndefinedFunction", func(t *testing.T) {
	    // This error should be caught by NewPaymentPolicyEnforcer during compilation.
	    badRules := []PolicyRule{
	        {ID: "bad_func", Expression: "nonExistentFunction(amount) == true", Decision: PolicyDecision{}},
	    }
	    _, errNew := NewPaymentPolicyEnforcer(badRules)
	    require.Error(t, errNew, "NewPaymentPolicyEnforcer should error on undefined function")
		// Govaluate error message for undefined function is "Undefined function nonExistentFunction"
		// which is wrapped by our error message.
	    assert.Contains(t, errNew.Error(), "failed to compile rule ID 'bad_func'")
	    assert.Contains(t, errNew.Error(), "Undefined function nonExistentFunction") // Corrected string
	})

	t.Run("EvaluationError_ParameterNotFound", func(t *testing.T) {
		// Rule uses a parameter not provided in the map
		errorRule := []PolicyRule{
			{ID: "missing_param_rule", Expression: "undefinedParam > 10", Decision: PolicyDecision{}},
		}
		errorPPE, errNew := NewPaymentPolicyEnforcer(errorRule)
		require.NoError(t, errNew)

		stepCtx := context.StepExecutionContext{AmountCents: 100} // undefinedParam is not set by default
		_, evalErr := errorPPE.Evaluate(domainCtx, stepCtx)
		require.Error(t, evalErr, "Expected an error due to undefined parameter")
		// Govaluate specific error for undefined parameters
		assert.Contains(t, evalErr.Error(), "No parameter 'undefinedParam' found.")
	})
}

// Test for old stub behavior (default decision when no rules)
func TestPaymentPolicyEnforcer_Evaluate_DefaultDecisionWithNoRules(t *testing.T) {
	ppe, err := NewPaymentPolicyEnforcer(nil)
	require.NoError(t, err)
	domainCtx := context.DomainContext{MerchantID: "test-merchant"}
	// StepCtx with some example values, though they won't match any rules
	stepCtx := context.StepExecutionContext{
		AmountCents: 1000, Currency: "USD", Region: "US", MerchantTier: "bronze", FraudScore: 0.1,
		StartTime: time.Now(), // For completeness, though not used by rules here
	}

	decision, errEval := ppe.Evaluate(domainCtx, stepCtx)
	require.NoError(t, errEval)
	assert.True(t, decision.AllowRetry, "Default policy should allow retry")
	assert.False(t, decision.SkipFallback, "Default policy should not skip fallback")
	assert.False(t, decision.EscalateManual, "Default policy should not escalate manually")
}
