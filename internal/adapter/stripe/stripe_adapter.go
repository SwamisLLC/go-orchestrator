package stripe

import (
	// "bytes" // No longer used
	stdcontext "context" // Standard Go context, aliased to avoid collision
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context" // For our custom TraceContext, StepExecutionContext
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// StripeAdapter handles communication with the Stripe API.
type StripeAdapter struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string // e.g., "https://api.stripe.com/v1/payment_intents"
}

// NewStripeAdapter creates a new instance of StripeAdapter.
func NewStripeAdapter(client *http.Client, apiKey string, apiEndpoint string) *StripeAdapter {
	if client == nil {
		client = &http.Client{}
	}
	if apiEndpoint == "" {
		apiEndpoint = "https://api.stripe.com/v1/payment_intents"
	}
	if apiKey == "" {
		log.Println("StripeAdapter: Warning - API key is empty.") // Or panic, depending on desired strictness
	}
	return &StripeAdapter{
		httpClient: client,
		apiKey:     apiKey,
		endpoint:   apiEndpoint,
	}
}

// generateIdempotencyKey creates a unique key for Stripe idempotency.
func (sa *StripeAdapter) generateIdempotencyKey() string {
	return uuid.NewString()
}

// buildStripePayload creates an io.Reader for the x-www-form-urlencoded Stripe request body.
func (sa *StripeAdapter) buildStripePayload(step *orchestratorinternalv1.PaymentStep) (io.Reader, error) {
	if step == nil {
		return nil, fmt.Errorf("stripe: payment step cannot be nil")
	}
	if step.GetAmount() <= 0 {
		return nil, fmt.Errorf("stripe: payment amount must be positive, got %d", step.GetAmount())
	}
	if step.GetCurrency() == "" {
		return nil, fmt.Errorf("stripe: currency code cannot be empty")
	}

	data := url.Values{}
	data.Set("amount", fmt.Sprintf("%d", step.GetAmount()))
	data.Set("currency", strings.ToLower(step.GetCurrency()))
	data.Set("payment_method_types[]", "card")

	if desc, ok := step.GetMetadata()["description"]; ok {
		data.Set("description", desc)
	}
	if sd, ok := step.GetMetadata()["statement_descriptor"]; ok {
		data.Set("statement_descriptor", sd)
	}
	if customerID, ok := step.GetProviderPayload()["stripe_customer_id"]; ok {
		data.Set("customer", customerID)
	}
	if paymentMethodID, ok := step.GetProviderPayload()["stripe_payment_method_id"]; ok {
		data.Set("payment_method", paymentMethodID)
		// data.Set("confirm", "true") // Auto-confirm if payment_method is provided; consider if this is always desired
	}
	return strings.NewReader(data.Encode()), nil
}

// --- Stripe API Response Structs ---
type StripeError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Type        string `json:"type"`
	DeclineCode string `json:"decline_code,omitempty"`
}

type StripeErrorResponse struct {
	Error StripeError `json:"error"`
}

type StripePaymentIntent struct {
	ID                   string            `json:"id"`
	Amount               int64             `json:"amount"`
	Currency             string            `json:"currency"`
	Status               string            `json:"status"`
	ClientSecret         string            `json:"client_secret,omitempty"`
	LastPaymentError     *StripeError      `json:"last_payment_error,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

// --- Process Method ---
const MaxRetries = 1 // Exported for test access
const retryDelay = 500 * time.Millisecond

// Process handles a single payment step using the Stripe provider logic.
func (sa *StripeAdapter) Process(
	traceCtx context.TraceContext, // Our custom TraceContext
	step *orchestratorinternalv1.PaymentStep,
	stepCtx context.StepExecutionContext, // Our custom StepExecutionContext
) (adapter.ProviderResult, error) {

	payload, err := sa.buildStripePayload(step)
	if err != nil {
		return adapter.ProviderResult{
			StepID:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "PAYLOAD_BUILD_ERROR",
			ErrorMessage: err.Error(),
			Provider:     "stripe",
		}, fmt.Errorf("StripeAdapter.Process: failed to build payload: %w", err)
	}

	idempotencyKey := sa.generateIdempotencyKey()
	var lastHttpErr error
	var httpResp *http.Response
	var latency time.Duration

	// Use stdcontext.Background() for the HTTP request context for now.
	// Ideally, a context would be passed down from the initial request, incorporating timeouts/cancellation.
	// stepCtx.RemainingBudgetMs could be used to create a derived context with timeout.
	reqCtx := stdcontext.Background()
	if stepCtx.RemainingBudgetMs > 0 {
		timeout := time.Duration(stepCtx.RemainingBudgetMs) * time.Millisecond
		var cancel stdcontext.CancelFunc
		reqCtx, cancel = stdcontext.WithTimeout(reqCtx, timeout)
		defer cancel() // Ensure cancel is called to free resources
	}


	for attempt := 0; attempt <= MaxRetries; attempt++ { // Use exported MaxRetries
		// For retries, the payload might need to be re-created if it's an io.Reader that gets consumed.
		// strings.NewReader can be read multiple times, but if it were from a one-time stream, care is needed.
		// For url.Values, data.Encode() can be called again or the string stored.
		// Since buildStripePayload returns a new strings.NewReader each time, it's fine to call it again if needed,
		// but for now, we assume the payload reader can be reused or the body is not the cause of retryable errors.
		// If payload was an `io.ReadCloser` it would be more complex. `strings.NewReader` is fine.

		var currentPayload io.Reader = payload
		if attempt > 0 { // If retrying, ensure payload can be read again
		    currentPayload, err = sa.buildStripePayload(step) // Rebuild to be safe for retries
		    if err != nil {
		        return adapter.ProviderResult{
					StepID: step.GetStepId(), Success: false, ErrorCode: "PAYLOAD_REBUILD_ERROR", ErrorMessage: err.Error(), Provider: "stripe"},
					fmt.Errorf("StripeAdapter.Process: failed to rebuild payload for retry: %w", err)
			}
		}


		httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sa.endpoint, currentPayload)
		if err != nil {
			// This is a non-retryable error for this attempt, likely fatal for the process.
			return adapter.ProviderResult{
				StepID:       step.GetStepId(),
				Success:      false,
				ErrorCode:    "REQUEST_CREATION_ERROR",
				ErrorMessage: err.Error(),
				Provider:     "stripe",
			}, fmt.Errorf("StripeAdapter.Process: failed to create http request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+sa.apiKey)
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		httpReq.Header.Set("Idempotency-Key", idempotencyKey)
		// Consider adding "Stripe-Version" header

		startTime := time.Now()
		httpResp, err = sa.httpClient.Do(httpReq)
		latency = time.Since(startTime)

		if err != nil {
			lastHttpErr = fmt.Errorf("attempt %d: http client error: %w", attempt, err)
			log.Printf("StripeAdapter.Process: StepID %s, Attempt %d: HTTP client error: %v. Retrying after %v...", step.GetStepId(), attempt, err, retryDelay)
			if attempt < MaxRetries { // Use exported MaxRetries
				time.Sleep(retryDelay)
				continue
			}
			break // Max retries reached for this type of error
		}

		// Successful HTTP request, now check status code
		if httpResp.StatusCode >= 500 || httpResp.StatusCode == http.StatusTooManyRequests {
			respBodyBytes, _ := io.ReadAll(httpResp.Body) // Read to log/include in error
			httpResp.Body.Close() // Close body immediately after read

			lastHttpErr = fmt.Errorf("attempt %d: received HTTP %d: %s", attempt, httpResp.StatusCode, string(respBodyBytes))
			log.Printf("StripeAdapter.Process: StepID %s, Attempt %d: Received HTTP %d. Retrying after %v...", step.GetStepId(), attempt, httpResp.StatusCode, retryDelay)
			if attempt < MaxRetries { // Use exported MaxRetries
				time.Sleep(retryDelay)
				continue
			}
			break // Max retries reached for server-side/rate limit errors
		}

		// Non-retryable HTTP error or success, break retry loop
		lastHttpErr = nil // Clear lastHttpErr if we break loop due to non-retryable status or success
		break
	}

	// Ensure body is eventually closed if httpResp is not nil
	if httpResp != nil && httpResp.Body != nil {
		defer func() {
			io.Copy(io.Discard, httpResp.Body)
			httpResp.Body.Close()
		}()
	}

	if lastHttpErr != nil { // All retries failed with network/HTTP errors classified as retryable initially
		res := adapter.ProviderResult{
			StepID:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "NETWORK_ERROR",
			ErrorMessage: lastHttpErr.Error(),
			Provider:     "stripe",
			LatencyMs:    latency.Milliseconds(),
		}
		if httpResp != nil {
			res.HTTPStatus = httpResp.StatusCode
		}
		return res, lastHttpErr
	}

	if httpResp == nil { // Should not happen if lastHttpErr is nil
		err := errors.New("StripeAdapter.Process: HTTP response is nil after retry loop without error (internal logic error)")
		return adapter.ProviderResult{
			StepID:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "INTERNAL_ADAPTER_ERROR",
			ErrorMessage: err.Error(),
			Provider:     "stripe",
		}, err
	}

	respBodyBytes, bodyReadErr := io.ReadAll(httpResp.Body)
	if bodyReadErr != nil {
		return adapter.ProviderResult{
			StepID:       step.GetStepId(),
			Success:      false,
			ErrorCode:    "BODY_READ_ERROR",
			ErrorMessage: fmt.Sprintf("Failed to read response body: %v", bodyReadErr),
			Provider:     "stripe",
			LatencyMs:    latency.Milliseconds(),
			HTTPStatus:   httpResp.StatusCode,
		}, fmt.Errorf("StripeAdapter.Process: failed to read response body: %w", bodyReadErr)
	}

	providerResult := adapter.ProviderResult{
		StepID:       step.GetStepId(),
		RawResponse:  respBodyBytes, // Changed to []byte
		LatencyMs:    latency.Milliseconds(),
		HTTPStatus:   httpResp.StatusCode,
		Provider:     "stripe",
	}

	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		var stripeIntent StripePaymentIntent
		if err := json.Unmarshal(respBodyBytes, &stripeIntent); err != nil {
			providerResult.Success = false
			providerResult.ErrorCode = "RESPONSE_UNMARSHAL_ERROR"
			providerResult.ErrorMessage = fmt.Sprintf("Failed to unmarshal successful Stripe response: %v. Body: %s", err, string(respBodyBytes))
			return providerResult, fmt.Errorf("StripeAdapter.Process: %s", providerResult.ErrorMessage)
		}
		providerResult.TransactionID = stripeIntent.ID
		switch stripeIntent.Status {
		case "succeeded", "processing":
			providerResult.Success = true
		case "requires_payment_method", "requires_confirmation", "requires_action", "canceled":
			providerResult.Success = false
			if stripeIntent.LastPaymentError != nil {
				providerResult.ErrorCode = stripeIntent.LastPaymentError.Code
				providerResult.ErrorMessage = stripeIntent.LastPaymentError.Message
			} else {
				providerResult.ErrorCode = stripeIntent.Status
				providerResult.ErrorMessage = fmt.Sprintf("PaymentIntent status: %s", stripeIntent.Status)
			}
		default:
			providerResult.Success = false
			providerResult.ErrorCode = "UNKNOWN_STRIPE_STATUS"
			providerResult.ErrorMessage = fmt.Sprintf("Unknown PaymentIntent status: %s", stripeIntent.Status)
		}
	} else { // Non-2xx response
		var stripeErrResp StripeErrorResponse
		if err := json.Unmarshal(respBodyBytes, &stripeErrResp); err != nil {
			providerResult.Success = false
			providerResult.ErrorCode = "ERROR_RESPONSE_UNMARSHAL_ERROR"
			providerResult.ErrorMessage = fmt.Sprintf("Failed to unmarshal error Stripe response: %v. Body: %s", err, string(respBodyBytes))
			return providerResult, fmt.Errorf("StripeAdapter.Process: %s", providerResult.ErrorMessage)
		}
		providerResult.Success = false
		if stripeErrResp.Error.Code != "" {
		    providerResult.ErrorCode = stripeErrResp.Error.Code
		} else {
		    providerResult.ErrorCode = fmt.Sprintf("HTTP_%d", httpResp.StatusCode) // Generic error if code is empty
		}
		providerResult.ErrorMessage = stripeErrResp.Error.Message
	}
	return providerResult, nil
}

// GetName returns the name of the provider.
func (sa *StripeAdapter) GetName() string {
	return "stripe"
}
