package stripe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	"net/url" // For URL encoding

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/google/uuid"
)

const (
	stripeAPIBaseURL = "https://api.stripe.com/v1"
	defaultRetryAttempts = 2
	defaultRetryDelay    = 500 * time.Millisecond
)

// StripeAdapter implements the ProviderAdapter interface for Stripe.
type StripeAdapter struct {
	httpClient *http.Client
	apiBaseURL string // Allow overriding for testing
}

// NewStripeAdapter creates a new StripeAdapter.
func NewStripeAdapter(client *http.Client) *StripeAdapter {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second} // Default client
	}
	return &StripeAdapter{
		httpClient: client,
		apiBaseURL: stripeAPIBaseURL,
	}
}

// GetName returns the name of the provider.
func (s *StripeAdapter) GetName() string {
	return "stripe"
}

// generateIdempotencyKey creates a unique key for Stripe requests.
// Using traceID + stepID to make it unique per operation attempt if needed.
func generateIdempotencyKey(traceID, stepID string) string {
    key := fmt.Sprintf("%s-%s-%s", traceID, stepID, uuid.NewString())
    if len(key) > 255 { // Stripe max length for idempotency key
         return key[:255]
    }
    return key
}


// buildStripePayload creates the request body for a Stripe charge.
// This is a simplified payload. A real one would be more complex.
// It expects amount in cents.
func buildStripePayload(step *internalv1.PaymentStep) (url.Values, error) {
	payload := url.Values{}
	payload.Set("amount", strconv.FormatInt(step.Amount, 10)) // Stripe expects amount in cents
	payload.Set("currency", strings.ToLower(step.Currency))

	if token, ok := step.ProviderPayload["stripe_token"]; ok && token != "" {
	    payload.Set("source", token)
	} else {
	    payload.Set("source", "tok_visa") // Default test token
	}

	if description, ok := step.ProviderPayload["description"]; ok && description != "" {
	    payload.Set("description", description)
	} else {
	    payload.Set("description", fmt.Sprintf("Charge for Step %s", step.StepId))
	}

	return payload, nil
}

// StripeErrorResponse represents the error structure from Stripe API
type StripeErrorResponse struct {
    Error struct {
        Type string `json:"type"`
        Code string `json:"code"` // e.g., "card_declined"
        Message string `json:"message"`
        DeclineCode string `json:"decline_code"` // e.g. "insufficient_funds"
    } `json:"error"`
}


// Process handles a payment step using the Stripe API.
func (s *StripeAdapter) Process(
	traceCtx context.TraceContext,
	step *internalv1.PaymentStep,
	stepCtx context.StepExecutionContext,
) (adapter.ProviderResult, error) {
	startTime := time.Now()

	chargePayloadValues, errBuild := buildStripePayload(step)
	if errBuild != nil {
		return adapter.ProviderResult{StepID: step.StepId, Provider: s.GetName(), Success: false, ErrorCode: "PAYLOAD_BUILD_ERROR", ErrorMessage: errBuild.Error()},
		       fmt.Errorf("stripe: failed to build charge payload: %w", errBuild)
	}
	requestBody := []byte(chargePayloadValues.Encode())

	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= defaultRetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(defaultRetryDelay)
		}

		idempotencyKey := generateIdempotencyKey(traceCtx.TraceID, step.StepId)

		req, err := http.NewRequest("POST", s.apiBaseURL+"/charges", bytes.NewBuffer(requestBody))
		if err != nil {
			lastErr = fmt.Errorf("stripe: failed to create http request: %w", err)
			// If request creation fails, it's unlikely to succeed on retry, so break.
			// However, per loop structure, let's allow it to be the final lastErr.
			// For robustness, one might return immediately here.
			break
		}

		req.Header.Set("Authorization", "Bearer "+stepCtx.ProviderCredentials.APIKey)
		req.Header.Set("Idempotency-Key", idempotencyKey)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		currentResp, doErr := s.httpClient.Do(req)
		if doErr != nil {
			lastErr = fmt.Errorf("stripe: http client error on attempt %d: %w", attempt+1, doErr)
			if currentResp != nil { // Defensively close body if it exists on error
				currentResp.Body.Close()
			}
			continue // Retry network/client errors
		}

		resp = currentResp // Store successful response

		// Check for retryable HTTP status codes (5xx, 429)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close() // Close this attempt's body
			resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body for potential re-read if this is the last attempt
			lastErr = fmt.Errorf("stripe: received HTTP %d (attempt %d). Response: %s", resp.StatusCode, attempt+1, string(bodyBytes))
			if attempt < defaultRetryAttempts {
			    continue // Retry if not last attempt
			}
		}
		break // If not a specifically retryable server error, or if successful, break to process response.
	}

	latencyMs := time.Since(startTime).Milliseconds()

	if resp == nil { // True if all attempts failed with client/network errors or request creation failed
	    finalError := lastErr
	    if finalError == nil { // Should not happen if resp is nil, but as a safeguard
	        finalError = fmt.Errorf("stripe: unknown error, no response received after retries")
	    }
	    return adapter.ProviderResult{
	        StepID: step.StepId, Provider: s.GetName(), Success: false,
	        ErrorCode: "STRIPE_NETWORK_ERROR", ErrorMessage: finalError.Error(), LatencyMs: latencyMs,
	    }, finalError
	}

	// Ensure body is closed if we broke from loop and have a response
	defer resp.Body.Close()

	// If we exhausted retries and the last response was a server error, process it as a failure
	if attempt := defaultRetryAttempts; attempt == defaultRetryAttempts && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError) {
	    bodyBytes, _ := ioutil.ReadAll(resp.Body)
	    // resp.Body is already deferred to close
	    return adapter.ProviderResult{
	        StepID: step.StepId, Provider: s.GetName(), Success: false,
	        ErrorCode: fmt.Sprintf("STRIPE_HTTP_%d", resp.StatusCode),
	        ErrorMessage: fmt.Sprintf("Stripe API request failed after retries with HTTP %d: %s", resp.StatusCode, string(bodyBytes)),
	        LatencyMs: latencyMs, RawResponse: bodyBytes,
	    }, fmt.Errorf("stripe: API request failed after retries with HTTP %d", resp.StatusCode)
	}

	bodyBytes, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		return adapter.ProviderResult{
			StepID: step.StepId, Provider: s.GetName(), Success: false,
			ErrorCode: "STRIPE_READ_RESPONSE_ERROR", ErrorMessage: fmt.Sprintf("Failed to read Stripe response body: %v", readErr),
			LatencyMs: latencyMs,
		}, fmt.Errorf("stripe: failed to read response body: %w", readErr)
	}

	result := adapter.ProviderResult{
		StepID:      step.StepId,
		Provider:    s.GetName(),
		LatencyMs:   latencyMs,
		RawResponse: bodyBytes,
		Details:     make(map[string]string),
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		var successResponse map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &successResponse); err == nil {
			if id, ok := successResponse["id"].(string); ok {
				result.TransactionID = id
				result.Details["provider_transaction_id"] = id
			}
			if status, ok := successResponse["status"].(string); ok {
			    result.Details["stripe_status"] = status
			}
		}
	} else {
		result.Success = false
		var errorResponse StripeErrorResponse
		if err := json.Unmarshal(bodyBytes, &errorResponse); err == nil && errorResponse.Error.Message != "" {
			result.ErrorCode = errorResponse.Error.Code
			if errorResponse.Error.DeclineCode != "" {
			    result.ErrorCode = errorResponse.Error.DeclineCode
			}
			result.ErrorMessage = errorResponse.Error.Message
			result.Details["stripe_error_type"] = errorResponse.Error.Type
		} else {
			result.ErrorCode = fmt.Sprintf("STRIPE_HTTP_%d", resp.StatusCode)
			result.ErrorMessage = fmt.Sprintf("Stripe API request failed with HTTP %d. Response: %s", resp.StatusCode, string(bodyBytes))
		}
	}
	return result, nil
}
