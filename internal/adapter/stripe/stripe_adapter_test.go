package stripe

import (
	stdcontext "context" // Correctly aliasing standard context
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStripeAdapter(t *testing.T) {
	adapter := NewStripeAdapter(nil)
	require.NotNil(t, adapter)
	assert.Equal(t, "stripe", adapter.GetName())
	assert.NotNil(t, adapter.httpClient)
	assert.Equal(t, stripeAPIBaseURL, adapter.apiBaseURL)
}

func TestGenerateIdempotencyKey(t *testing.T) {
    key1 := generateIdempotencyKey("trace1", "step1")
    key2 := generateIdempotencyKey("trace1", "step1") // Should be different due to uuid.NewString()
    key3 := generateIdempotencyKey("trace2", "step1")

    assert.NotEmpty(t, key1)
    assert.NotEqual(t, key1, key2)
    assert.NotEqual(t, key1, key3)
    assert.True(t, len(key1) <= 255)
}

func TestBuildStripePayload(t *testing.T) {
    step := &internalv1.PaymentStep{
        StepId: "s1", Amount: 12345, Currency: "USD",
        ProviderPayload: map[string]string{
            "stripe_token": "tok_test_custom",
            "description": "Custom Description",
        },
    }
    payload, err := buildStripePayload(step)
    require.NoError(t, err)
    assert.Equal(t, "12345", payload.Get("amount"))
    assert.Equal(t, "usd", payload.Get("currency"))
    assert.Equal(t, "tok_test_custom", payload.Get("source"))
    assert.Equal(t, "Custom Description", payload.Get("description"))

    stepNoPayload := &internalv1.PaymentStep{StepId: "s2", Amount: 500, Currency: "EUR", ProviderPayload: make(map[string]string)}
    payloadNoPayload, err := buildStripePayload(stepNoPayload)
    require.NoError(t, err)
    assert.Equal(t, "500", payloadNoPayload.Get("amount"))
    assert.Equal(t, "eur", payloadNoPayload.Get("currency"))
    assert.Equal(t, "tok_visa", payloadNoPayload.Get("source")) // Default token
    assert.Equal(t, "Charge for Step s2", payloadNoPayload.Get("description")) // Default description
}


func TestStripeAdapter_Process_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/charges", r.URL.Path) // Corrected path
		authHeader := r.Header.Get("Authorization")
		assert.True(t, strings.HasPrefix(authHeader, "Bearer sk_test_"), "Auth header mismatch")
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		bodyBytes, _ := ioutil.ReadAll(r.Body)
		bodyString := string(bodyBytes)
		assert.Contains(t, bodyString, "amount=1099")
		assert.Contains(t, bodyString, "currency=usd")
		assert.Contains(t, bodyString, "source=tok_visa")


		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "ch_12345",
			"status": "succeeded",
			"amount": 1099,
		})
	}))
	defer server.Close()

	adapter := NewStripeAdapter(server.Client())
	adapter.apiBaseURL = server.URL // Point to mock server

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{
		StepId:   "step_succ",
		Amount:   1099, // In cents
		Currency: "USD",
		ProviderPayload: map[string]string{"stripe_token": "tok_visa"},
	}
	stepCtx := context.StepExecutionContext{
		ProviderCredentials: context.Credentials{APIKey: "sk_test_apikey"},
		StartTime: time.Now(),
	}

	result, err := adapter.Process(traceCtx, step, stepCtx)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "ch_12345", result.TransactionID)
	assert.Equal(t, "ch_12345", result.Details["provider_transaction_id"])
	assert.Equal(t, "succeeded", result.Details["stripe_status"])
	assert.NotEmpty(t, result.RawResponse)
}

func TestStripeAdapter_Process_CardDeclined(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired) // 402
		json.NewEncoder(w).Encode(StripeErrorResponse{
			Error: struct {
				Type string `json:"type"`
				Code string `json:"code"`
				Message string `json:"message"`
				DeclineCode string `json:"decline_code"`
			}{
				Type: "card_error",
				Code: "card_declined",
				Message: "Your card was declined.",
				DeclineCode: "insufficient_funds",
			},
		})
	}))
	defer server.Close()

	adapter := NewStripeAdapter(server.Client())
	adapter.apiBaseURL = server.URL

	traceCtx := context.NewTraceContext()
	step := &internalv1.PaymentStep{StepId: "step_decl", Amount: 2000, Currency: "EUR", ProviderPayload: map[string]string{"stripe_token":"tok_chargeDeclined"}}
	stepCtx := context.StepExecutionContext{ProviderCredentials: context.Credentials{APIKey: "sk_test_key"}, StartTime: time.Now()}

	result, err := adapter.Process(traceCtx, step, stepCtx)
	require.NoError(t, err, "Adapter itself should not error for a card decline")
	assert.False(t, result.Success)
	assert.Equal(t, "insufficient_funds", result.ErrorCode) // Decline code should be preferred
	assert.Equal(t, "Your card was declined.", result.ErrorMessage)
	assert.Equal(t, "card_error", result.Details["stripe_error_type"])
}

func TestStripeAdapter_Process_ServerError_WithRetry(t *testing.T) {
    attemptCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attemptCount++
        if attemptCount <= defaultRetryAttempts { // Fail first N attempts
            w.WriteHeader(http.StatusInternalServerError)
            json.NewEncoder(w).Encode(map[string]interface{}{"error": "transient server issue"})
            return
        }
        // Success on the last attempt
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{"id": "ch_retry_ok", "status": "succeeded"})
    }))
    defer server.Close()

    adapter := NewStripeAdapter(server.Client())
    adapter.apiBaseURL = server.URL

    traceCtx := context.NewTraceContext()
    step := &internalv1.PaymentStep{StepId: "step_retry", Amount: 500, Currency: "USD", ProviderPayload: map[string]string{"stripe_token":"tok_visa"}}
    stepCtx := context.StepExecutionContext{ProviderCredentials: context.Credentials{APIKey: "sk_test_retry"}, StartTime: time.Now()}

    result, err := adapter.Process(traceCtx, step, stepCtx)
    require.NoError(t, err)
    assert.True(t, result.Success, "Should succeed after retries")
    assert.Equal(t, "ch_retry_ok", result.TransactionID)
    assert.Equal(t, defaultRetryAttempts+1, attemptCount, "Should have retried specified number of times + initial attempt")
}

func TestStripeAdapter_Process_ServerError_AllRetriesFail(t *testing.T) {
    attemptCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attemptCount++
        w.WriteHeader(http.StatusServiceUnavailable) // Consistently fail
        json.NewEncoder(w).Encode(map[string]interface{}{"error": "service down"})
    }))
    defer server.Close()

    adapter := NewStripeAdapter(server.Client())
    adapter.apiBaseURL = server.URL

    traceCtx := context.NewTraceContext()
    step := &internalv1.PaymentStep{StepId: "step_all_fail", Amount: 300, Currency: "GBP", ProviderPayload: map[string]string{"stripe_token":"tok_visa"}}
    stepCtx := context.StepExecutionContext{ProviderCredentials: context.Credentials{APIKey: "sk_test_allfail"}, StartTime: time.Now()}

    result, err := adapter.Process(traceCtx, step, stepCtx)
    // Adapter.Process returns an error if all retries for server errors fail
    require.Error(t, err, "Adapter should return an error if all retries fail for server issues")
    assert.Contains(t, err.Error(), "stripe: API request failed after retries with HTTP 503") // Corrected case
    assert.False(t, result.Success)
    assert.Equal(t, "STRIPE_HTTP_503", result.ErrorCode)
    assert.Contains(t, result.ErrorMessage, "Stripe API request failed after retries with HTTP 503") // This one is from the body, might be different
    assert.Equal(t, defaultRetryAttempts+1, attemptCount)
}

func TestStripeAdapter_Process_NetworkError_AllRetriesFail(t *testing.T) {
    // No server, httpClient.Do will fail
    customClient := &http.Client{
        Transport: &http.Transport{
            DialContext: func(ctx stdcontext.Context, network, addr string) (net.Conn, error) {
                return nil, fmt.Errorf("simulated network error")
            },
        },
        Timeout: 100 * time.Millisecond, // Short timeout for test
    }


    adapter := NewStripeAdapter(customClient)
    // This URL will not be reached due to the custom transport DialContext error.
    // However, setting it to something that would normally be invalid if the DialContext worked.
    adapter.apiBaseURL = "http://nonexistent-stripe-endpoint.example.com"


    traceCtx := context.NewTraceContext()
    step := &internalv1.PaymentStep{StepId: "step_net_err", Amount: 100, Currency: "USD", ProviderPayload: map[string]string{"stripe_token":"tok_visa"}}
    stepCtx := context.StepExecutionContext{ProviderCredentials: context.Credentials{APIKey: "sk_test_neterr"}, StartTime: time.Now()}

    result, err := adapter.Process(traceCtx, step, stepCtx)
    require.Error(t, err, "Adapter should return an error for persistent network issues")
    // The error from httpClient.Do will be wrapped by the adapter's error message.
    assert.Contains(t, err.Error(), "stripe: http client error on attempt")
    assert.Contains(t, err.Error(), "simulated network error")
    assert.False(t, result.Success)
    assert.Equal(t, "STRIPE_NETWORK_ERROR", result.ErrorCode)
    assert.Contains(t, result.ErrorMessage, "simulated network error")
}
