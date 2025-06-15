// Package policy implements the PaymentPolicyEnforcer, a DSL-based rule engine.
// It evaluates dynamic business policies (e.g., for fraud, retries, routing)
// against the current payment context to make decisions that influence the
// payment flow.
// Refer to spec2_1.md, section 8.1 for more on PaymentPolicyEnforcer.
package policy

import (
	"fmt"
	"log"
	"strconv" // For parsing fraud_score
	"strings"   // For ToLower

	"github.com/Knetic/govaluate"
	"github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// PolicyDecision represents the outcome of a policy evaluation.
type PolicyDecision struct {
	AllowRetry     bool              // Whether a failed step can be retried
	SkipFallback   bool              // Whether to skip fallback providers
	EscalateManual bool              // Whether this payment needs manual review
	NextAction     string            // Suggested next action, e.g., "BLOCK", "ALLOW", "REVIEW"
	Reason         string            // Reason for the decision
	Metadata       map[string]string // Additional decision metadata
}

// PaymentPolicyEnforcer evaluates dynamic business policies.
type PaymentPolicyEnforcer struct {
	compiledRules map[string]*govaluate.EvaluableExpression
	rawRules      map[string]string
}

// RuleConfig defines a named rule expression.
type RuleConfig struct {
	Name       string
	Expression string
}

// NewPaymentPolicyEnforcer creates a new PaymentPolicyEnforcer.
// It accepts a list of RuleConfig objects, compiles their expressions, and stores them.
func NewPaymentPolicyEnforcer(rules []RuleConfig) (*PaymentPolicyEnforcer, error) {
	compiled := make(map[string]*govaluate.EvaluableExpression)
	raw := make(map[string]string)

	if rules == nil {
		log.Println("NewPaymentPolicyEnforcer: No rules provided, initializing with empty rule set.")
		rules = []RuleConfig{}
	}

	log.Printf("NewPaymentPolicyEnforcer: Compiling %d rules...", len(rules))
	for _, ruleCfg := range rules {
		if ruleCfg.Name == "" {
			log.Printf("Warning: Skipping rule with empty name (Expression: '%s')", ruleCfg.Expression)
			continue
		}
		if ruleCfg.Expression == "" {
			log.Printf("Warning: Skipping rule '%s' with empty expression", ruleCfg.Name)
			continue
		}

		log.Printf("Compiling rule '%s': %s", ruleCfg.Name, ruleCfg.Expression)
		expression, err := govaluate.NewEvaluableExpression(ruleCfg.Expression)
		if err != nil {
			log.Printf("Error compiling rule '%s' ('%s'): %v", ruleCfg.Name, ruleCfg.Expression, err)
			return nil, fmt.Errorf("failed to compile rule '%s' ('%s'): %w", ruleCfg.Name, ruleCfg.Expression, err)
		}
		compiled[ruleCfg.Name] = expression
		raw[ruleCfg.Name] = ruleCfg.Expression
	}

	log.Printf("NewPaymentPolicyEnforcer: Successfully compiled %d rules.", len(compiled))
	return &PaymentPolicyEnforcer{
		compiledRules: compiled,
		rawRules:      raw,
	}, nil
}

// Evaluate is the method that evaluates policies for a given payment step.
func (ppe *PaymentPolicyEnforcer) Evaluate(
	domainCtx context.DomainContext, // Re-added DomainContext
	stepCtx context.StepExecutionContext,
	step *orchestratorinternalv1.PaymentStep,
	stepResult *orchestratorinternalv1.StepResult,
) (PolicyDecision, error) {
	if ppe.compiledRules == nil {
		log.Println("PaymentPolicyEnforcer.Evaluate: No compiled rules available. Returning default decision.")
		// Default behavior if stepResult is also nil (should not happen if called after a step attempt)
		allowR := true
		if stepResult != nil && stepResult.GetSuccess() {
			allowR = false
		}
		return PolicyDecision{AllowRetry: allowR, NextAction: "PROCEED", Reason: "No rules configured", Metadata: make(map[string]string)}, nil
	}

	parameters := make(map[string]interface{})

	// Populate parameters
	if step != nil {
		parameters["amount_units"] = step.GetAmount()
		parameters["currency"] = strings.ToLower(step.GetCurrency())
		parameters["provider_name"] = strings.ToLower(step.GetProviderName())
		if region, ok := step.GetMetadata()["region"]; ok {
			parameters["region"] = strings.ToLower(region)
		} else if defRegion, ok := domainCtx.UserPreferences["default_region"]; ok { // Fallback to domain context's user preferences
			parameters["region"] = strings.ToLower(defRegion)
		} else {
			parameters["region"] = ""
		}
	} else { // Should ideally not happen if called with valid step
		parameters["amount_units"] = int64(0)
		parameters["currency"] = ""
		parameters["provider_name"] = ""
		parameters["region"] = ""
	}

	if stepResult != nil {
		parameters["step_success"] = stepResult.GetSuccess()
		parameters["step_error_code"] = strings.ToLower(stepResult.GetErrorCode())
		// StepResult proto uses bool Success, not a Status enum.
		// We can derive a status string if needed by rules.
		if stepResult.GetSuccess() {
			parameters["step_status_derived"] = "succeeded"
		} else {
			parameters["step_status_derived"] = "failed"
		}
	} else { // Should ideally not happen if called with valid stepResult
		parameters["step_success"] = false
		parameters["step_error_code"] = ""
		parameters["step_status_derived"] = "unknown"
	}

	parameters["attempt_number"] = stepCtx.AttemptNumber // Assuming StepExecutionContext has AttemptNumber

	// Accessing DomainContext fields
	if fraudScoreStr, ok := domainCtx.UserPreferences["fraud_score"]; ok {
		if fs, err := strconv.ParseFloat(fraudScoreStr, 64); err == nil {
			parameters["fraud_score"] = fs
		} else {
			parameters["fraud_score"] = 0.0
		}
	} else {
		parameters["fraud_score"] = 0.0
	}
	parameters["merchant_id"] = domainCtx.MerchantID
	if tier, ok := domainCtx.ActiveMerchantConfig.UserPrefs["merchant_tier"]; ok { // UserPrefs on MerchantConfig
		parameters["merchant_tier"] = strings.ToLower(tier)
	} else {
		parameters["merchant_tier"] = "standard"
	}

	log.Printf("PaymentPolicyEnforcer.Evaluate: Evaluating StepID %s with parameters: %+v", step.GetStepId(), parameters)

	decision := PolicyDecision{
		AllowRetry:     false,
		SkipFallback:   false,
		EscalateManual: false,
		NextAction:     "PROCEED",
		Reason:         "Default policy outcome",
		Metadata:       make(map[string]string),
	}

	if rule, ok := ppe.compiledRules["RetryRule"]; ok {
		result, err := rule.Evaluate(parameters)
		if err != nil {
			log.Printf("Error evaluating RetryRule for StepID %s: %v. Parameters: %+v", step.GetStepId(), err, parameters)
			decision.Metadata["RetryRuleError"] = err.Error()
		} else if resBool, typeOk := result.(bool); typeOk && resBool {
			decision.AllowRetry = true
			decision.Reason = "RetryRule allowed retry."
			decision.Metadata["RetryRuleOutcome"] = "AllowRetry=true"
		} else {
			decision.Metadata["RetryRuleOutcome"] = fmt.Sprintf("AllowRetry=false (result: %v, type: %T)", result, result)
		}
	} else {
		// Default retry logic: allow retry on first failure if step actually failed
		if !parameters["step_success"].(bool) && parameters["attempt_number"].(int) < 2 {
			decision.AllowRetry = true
			decision.Reason = "Default: Allow retry on first failure (no RetryRule)."
			decision.Metadata["RetryRuleOutcome"] = "DefaultAllowFirstRetry"
		} else if !parameters["step_success"].(bool) {
			decision.Reason = "Default: No retry (not first failure or step succeeded; no RetryRule)."
			decision.Metadata["RetryRuleOutcome"] = "DefaultNoRetry"
		}
		// If step succeeded, AllowRetry remains false, reason "Default policy outcome" or from other rules.
	}

	if rule, ok := ppe.compiledRules["FraudRule"]; ok {
		result, err := rule.Evaluate(parameters)
		if err != nil {
			log.Printf("Error evaluating FraudRule for StepID %s: %v. Parameters: %+v", step.GetStepId(), err, parameters)
			decision.Metadata["FraudRuleError"] = err.Error()
		} else if resStr, typeOk := result.(string); typeOk && resStr != "" {
			decision.NextAction = strings.ToUpper(resStr)
			decision.Reason = fmt.Sprintf("%s; FraudRule action: %s", decision.Reason, decision.NextAction)
			decision.Metadata["FraudRuleOutcome"] = decision.NextAction
		}
	}

	if rule, ok := ppe.compiledRules["EscalationRule"]; ok {
		result, err := rule.Evaluate(parameters)
		if err != nil {
			log.Printf("Error evaluating EscalationRule for StepID %s: %v. Parameters: %+v", step.GetStepId(), err, parameters)
			decision.Metadata["EscalationRuleError"] = err.Error()
		} else if resBool, typeOk := result.(bool); typeOk && resBool {
			decision.EscalateManual = true
			decision.Reason = fmt.Sprintf("%s; EscalationRule triggered manual review.", decision.Reason)
			decision.Metadata["EscalationRuleOutcome"] = "EscalateManual=true"
		}
	}

	if decision.NextAction == "BLOCK" {
		decision.AllowRetry = false
		decision.SkipFallback = true
	}
	if decision.NextAction == "REVIEW" {
		decision.EscalateManual = true
	}

	// If the step was successful, override AllowRetry to false, regardless of rules.
	if stepResult != nil && stepResult.GetSuccess() {
		if decision.AllowRetry { // Only log/adjust reason if a rule had tried to allow retry for a success
			decision.Reason = fmt.Sprintf("%s (Retry overridden for successful step)", decision.Reason)
		}
		decision.AllowRetry = false
	}


	log.Printf("PaymentPolicyEnforcer.Evaluate: Final decision for StepID %s: %+v", step.GetStepId(), decision)
	return decision, nil
}
