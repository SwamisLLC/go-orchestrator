package stripe_test

import (
	// "bytes" // No longer used
	stdcontext "context" // Standard Go context
	"encoding/json"
	"fmt"
	// "io" // No longer used directly
	"net/http"
	"net/http/httptest"
	"net/url" // Used by url.Error check
	// "strings" // No longer used
	"testing"
	"time"

	// "github.com/google/uuid" // No longer used directly
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// "github.com/yourorg/payment-orchestrator/internal/adapter" // Not directly used
	stripe_adapter "github.com/yourorg/payment-orchestrator/internal/adapter/stripe" // aliasing for clarity
	custom_context "github.com/yourorg/payment-orchestrator/internal/context"
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

func TestNewStripeAdapter(t *testing.T) {
	t.Run("Default client and endpoint", func(t *testing.T) {
		sa := stripe_adapter.NewStripeAdapter(nil, "sk_test_fakekey", "")
		require.NotNil(t, sa)
		// Private fields, so can't directly assert them without helpers or reflection.
		// We can infer by testing behavior in Process or by exposing getters if needed.
		// For now, just test creation.
	})

	t.Run("Custom client and endpoint", func(t *testing.T) {
		customClient := &http.Client{Timeout: 10 * time.Second}
		customEndpoint := "http://localhost/custom_stripe"
		sa := stripe_adapter.NewStripeAdapter(customClient, "sk_test_anotherkey", customEndpoint)
		require.NotNil(t, sa)
		// Again, direct assertion of private fields is tricky.
	})

	t.Run("Empty API key logs warning", func(t *testing.T) {
		// This test is a bit hard to assert directly without capturing log output.
		// For now, we just call it and rely on manual inspection or more advanced log capture if critical.
		_ = stripe_adapter.NewStripeAdapter(nil, "", "")
		// Expect a log message like "StripeAdapter: Warning - API key is empty."
	})
}

// TestStripeAdapter_buildStripePayload needs to be a method on a test struct or pass adapter instance
// to access buildStripePayload if it were private. Since it's an exported method (though probably shouldn't be),
// we can test it directly IF we instantiate an adapter.
// However, buildStripePayload is a private method `(sa *StripeAdapter) buildStripePayload`.
// We can't test it directly from `stripe_test` package.
// We'll test it indirectly via the `Process` method's behavior regarding payload.
// For demonstration, if it were EXPORTED, it would look like this:
/*
func TestStripeAdapter_BuildStripePayload_Exported(t *testing.T) {
	sa := stripe_adapter.NewStripeAdapter(nil, "sk_test_fakekey", "")
	t.Run("Valid step", func(t *testing.T) {
		step := &orchestratorinternalv1.PaymentStep{
			Amount:       1000,
			Currency:     "USD",
			Metadata:     map[string]string{"description": "Test charge"},
			ProviderPayload: map[string]string{"stripe_customer_id": "cus_123"},
		}
		reader, err := sa.BuildStripePayload(step) // If it were exported
		require.NoError(t, err)
		require.NotNil(t, reader)
		payloadBytes, _ := io.ReadAll(reader)
		payloadString := string(payloadBytes)

		expectedValues := url.Values{}
		expectedValues.Set("amount", "1000")
		expectedValues.Set("currency", "usd")
		expectedValues.Set("payment_method_types[]", "card")
		expectedValues.Set("description", "Test charge")
		expectedValues.Set("customer", "cus_123")
		assert.Equal(t, expectedValues.Encode(), payloadString)
	})

	t.Run("Nil step", func(t *testing.T) {
		_, err := sa.BuildStripePayload(nil) // If it were exported
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payment step cannot be nil")
	})

	t.Run("Zero amount", func(t *testing.T) {
		step := &orchestratorinternalv1.PaymentStep{Amount: 0, Currency: "USD"}
		_, err := sa.BuildStripePayload(step) // If it were exported
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payment amount must be positive")
	})

	t.Run("Empty currency", func(t *testing.T) {
		step := &orchestratorinternalv1.PaymentStep{Amount: 100, Currency: ""}
		_, err := sa.BuildStripePayload(step) // If it were exported
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "currency code cannot be empty")
	})
}
*/

// Test for generateIdempotencyKey (private method) would also be indirect,
// or tested if it were a public helper. We test its effect via Process header check.

// --- Test TestStripeAdapter_Process ---

func setupTestServerAndAdapter(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *stripe_adapter.StripeAdapter) {
	server := httptest.NewServer(handler)
	// Use server.Client() to ensure requests are routed to the test server
	adapter := stripe_adapter.NewStripeAdapter(server.Client(), "sk_test_fakekey", server.URL)
	return server, adapter
}

func TestStripeAdapter_Process_SuccessfulPayment(t *testing.T) {
	expectedAmount := int64(12345)
	expectedCurrency := "usd"
	expectedDescription := "Test Purchase"
	expectedStripeTxnID := "pi_3ExampleSUCCESS"

	handler := func(w http.ResponseWriter, r *http.Request) {
		// 1. Verify Headers
		assert.Equal(t, "Bearer sk_test_fakekey", r.Header.Get("Authorization"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key")) // Check if present

		// 2. Verify Body
		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%d", expectedAmount), r.Form.Get("amount"))
		assert.Equal(t, expectedCurrency, r.Form.Get("currency"))
		assert.Equal(t, "card", r.Form.Get("payment_method_types[]"))
		assert.Equal(t, expectedDescription, r.Form.Get("description"))

		// 3. Respond
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := stripe_adapter.StripePaymentIntent{ // Use exported type from stripe_adapter for testing response structure
			ID:       expectedStripeTxnID,
			Amount:   expectedAmount,
			Currency: expectedCurrency,
			Status:   "succeeded",
		}
		json.NewEncoder(w).Encode(response)
	}

	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	traceCtx := custom_context.NewTraceContext()
	step := &orchestratorinternalv1.PaymentStep{
		Amount:   expectedAmount,
		Currency: expectedCurrency,
		Metadata: map[string]string{"description": expectedDescription},
	}
	stepCtx := custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000}

	result, err := sa.Process(traceCtx, step, stepCtx)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, expectedStripeTxnID, result.TransactionID)
	assert.Equal(t, "stripe", result.Provider)
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	require.NotEmpty(t, result.RawResponse)
	assert.True(t, result.LatencyMs >= 0) // Latency can be very small, >=0
}

func TestStripeAdapter_Process_StripeApiError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired) // 402
		response := stripe_adapter.StripeErrorResponse{
			Error: stripe_adapter.StripeError{
				Code:    "card_declined",
				Message: "Your card was declined.",
				Type:    "card_error",
				DeclineCode: "generic_decline",
			},
		}
		json.NewEncoder(w).Encode(response)
	}

	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	traceCtx := custom_context.NewTraceContext()
	step := &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}
	stepCtx := custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000}

	result, err := sa.Process(traceCtx, step, stepCtx)

	require.NoError(t, err) // Process itself doesn't error for API errors, error is in result
	assert.False(t, result.Success)
	assert.Equal(t, "card_declined", result.ErrorCode)
	assert.Equal(t, "Your card was declined.", result.ErrorMessage)
	assert.Equal(t, http.StatusPaymentRequired, result.HTTPStatus)
}


func TestStripeAdapter_Process_RetryOn500_ThenSuccess(t *testing.T) {
	numRequests := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		numRequests++
		idempotencyKey := r.Header.Get("Idempotency-Key")
		require.NotEmpty(t, idempotencyKey) // Should be present on all attempts

		if numRequests == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Second attempt
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := stripe_adapter.StripePaymentIntent{ID: "pi_retrySuccess", Status: "succeeded"}
		json.NewEncoder(w).Encode(response)
	}

	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	traceCtx := custom_context.NewTraceContext()
	step := &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}
	stepCtx := custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000}

	result, err := sa.Process(traceCtx, step, stepCtx)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, numRequests)
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, "pi_retrySuccess", result.TransactionID)
}

func TestStripeAdapter_Process_Retry_MaxRetriesExhausted(t *testing.T) {
	numRequests := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		numRequests++
		w.WriteHeader(http.StatusInternalServerError)
	}

	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	traceCtx := custom_context.NewTraceContext()
	step := &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}
	stepCtx := custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000}

	result, err := sa.Process(traceCtx, step, stepCtx)

	require.Error(t, err) // Should return the last error
	assert.False(t, result.Success)
	assert.Equal(t, "NETWORK_ERROR", result.ErrorCode)
	assert.Contains(t, result.ErrorMessage, "received HTTP 500")
	assert.Equal(t, http.StatusInternalServerError, result.HTTPStatus)
	assert.Equal(t, stripe_adapter.MaxRetries+1, numRequests) // Initial attempt + MaxRetries
}

func TestStripeAdapter_Process_InvalidJsonResponse_SuccessStatus(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id": "pi_123", "status": "succeeded", malformed_json`) // Invalid JSON
	}
	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	result, err := sa.Process(custom_context.NewTraceContext(), &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}, custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000})

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "RESPONSE_UNMARSHAL_ERROR", result.ErrorCode)
	assert.Contains(t, err.Error(), "Failed to unmarshal successful Stripe response")
}

func TestStripeAdapter_Process_InvalidJsonResponse_ErrorStatus(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest) // 400
		fmt.Fprint(w, `{"error": {"code": "invalid_request", malformed_json`) // Invalid JSON
	}
	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	result, err := sa.Process(custom_context.NewTraceContext(), &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}, custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 5000})

	require.Error(t, err)
	assert.False(t, result.Success)
	// The adapter code has ERROR_RESPONSE_UNMARSHAL_ERROR for this.
	assert.Equal(t, "ERROR_RESPONSE_UNMARSHAL_ERROR", result.ErrorCode)
	assert.Contains(t, err.Error(), "Failed to unmarshal error Stripe response")
}

func TestStripeAdapter_Process_RequestTimeout(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Sleep longer than context timeout
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"pi_timeout","status":"succeeded"}`)
	}
	server, sa := setupTestServerAndAdapter(t, handler)
	defer server.Close()

	traceCtx := custom_context.NewTraceContext()
	// Short budget for timeout
	stepCtx := custom_context.StepExecutionContext{StartTime: time.Now(), RemainingBudgetMs: 50}
	step := &orchestratorinternalv1.PaymentStep{Amount: 1000, Currency: "usd"}

	result, err := sa.Process(traceCtx, step, stepCtx)

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "NETWORK_ERROR", result.ErrorCode) // Context deadline exceeded is a network error
	// Check that the error is indeed a context deadline exceeded error
	if ue, ok := err.(*url.Error); ok { // HTTP client errors are often wrapped in url.Error
		assert.ErrorIs(t, ue.Err, stdcontext.DeadlineExceeded)
	} else {
		assert.ErrorIs(t, err, stdcontext.DeadlineExceeded) // Fallback check
	}
}

func TestStripeAdapter_GetName(t *testing.T) {
	sa := stripe_adapter.NewStripeAdapter(nil, "sk_test_fakekey", "")
	assert.Equal(t, "stripe", sa.GetName())
}
