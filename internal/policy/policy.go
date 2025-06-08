package policy

import (
	"fmt"
	"github.com/Knetic/govaluate" // Import govaluate
	"github.com/yourorg/payment-orchestrator/internal/context"
)

// PolicyRule defines a structure for a rule.
type PolicyRule struct {
	ID         string // Unique ID for the rule
	Expression string // govaluate expression string
	Decision   PolicyDecision // The decision to return if this rule evaluates to true
	Priority   int    // Lower numbers run first (optional, for now sequential)
}

// PolicyDecision represents the outcome of a policy evaluation.
type PolicyDecision struct {
	AllowRetry     bool // Whether a failed step can be retried
	SkipFallback   bool // Whether to skip fallback providers
	EscalateManual bool // Whether this payment needs manual review
	// Add other decision fields as necessary, e.g., SkipStep bool
}


type PaymentPolicyEnforcer struct {
	rules            []compiledRule
	// metricsEmitter  MetricsEmitter // For future use
}

type compiledRule struct {
	id         string
	expression *govaluate.EvaluableExpression
	decision   PolicyDecision
	priority   int // For future sorting, not used in current sequential evaluation
}

// NewPaymentPolicyEnforcer compiles rules.
// Rules with invalid expressions will cause an error.
// Rules are not sorted by priority in this version; they are evaluated in the order provided.
func NewPaymentPolicyEnforcer(rules []PolicyRule) (*PaymentPolicyEnforcer, error) {
	if rules == nil {
	    rules = []PolicyRule{} // Default to no rules, results in default decisions
	}

	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Expression == "" {
		    return nil, fmt.Errorf("policy rule ID '%s' has an empty expression", rule.ID)
		}
		expr, err := govaluate.NewEvaluableExpression(rule.Expression)
		if err != nil {
			return nil, fmt.Errorf("failed to compile rule ID '%s' expression '%s': %w", rule.ID, rule.Expression, err)
		}
		compiled = append(compiled, compiledRule{
			id:         rule.ID,
			expression: expr,
			decision:   rule.Decision,
			priority:   rule.Priority, // Store priority for future use
		})
	}
	// TODO: Sort compiled rules by priority if needed in the future.
	return &PaymentPolicyEnforcer{rules: compiled}, nil
}

// Evaluate uses govaluate to check rules against StepExecutionContext.
// The first rule that evaluates to true determines the PolicyDecision.
// If no rules match, a default "allow all" decision is returned.
func (ppe *PaymentPolicyEnforcer) Evaluate(
	domainCtx context.DomainContext, // domainCtx might be needed for some parameters not in stepCtx
	stepCtx context.StepExecutionContext,
) (PolicyDecision, error) {
	parameters := map[string]interface{}{
		"amount":       stepCtx.AmountCents,
		"currency":     stepCtx.Currency,
		"region":       stepCtx.Region,
		"merchantTier": stepCtx.MerchantTier,
		"fraudScore":   stepCtx.FraudScore,
		// Example of using a field from domainCtx if needed:
		// "merchantID":   domainCtx.MerchantID,
	}

	for _, cr := range ppe.rules {
		result, err := cr.expression.Evaluate(parameters)
		if err != nil {
			// Error during evaluation of a specific rule (e.g., type mismatch if params are wrong, or undefined function)
			// This indicates a problem with the rule expression or parameters provided.
			return PolicyDecision{}, fmt.Errorf("error evaluating rule ID '%s' (expression: '%s'): %w. Parameters: %+v", cr.id, cr.expression.String(), err, parameters)
		}

		if boolResult, ok := result.(bool); ok && boolResult {
			// Rule matched, return its predefined decision
			// fmt.Printf("Rule '%s' matched. Decision: %+v\n", cr.id, cr.decision) // For debugging
			return cr.decision, nil
		}
	}

	// Default decision if no rules match: allow retry, don't skip fallback, don't escalate.
	return PolicyDecision{
		AllowRetry:     true,
		SkipFallback:   false,
		EscalateManual: false,
	}, nil
}
