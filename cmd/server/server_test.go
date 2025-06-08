package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	externalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorexternalv1"
	"github.com/yourorg/payment-orchestrator/internal/orchestrator"
	"google.golang.org/protobuf/encoding/protojson" // Added for marshalling in test
)

// Setup router for testing
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	// It's critical that setupApplication is called to initialize handlers' dependencies
	if err := setupApplication(); err != nil {
	    panic(fmt.Sprintf("Test setup failed: %v", err))
	}

	r := gin.Default() // Use a new engine for each test setup to avoid route conflicts if tests run in parallel
	r.POST("/process-payment", processPaymentHandler)
	return r
}

// TestMain is not strictly necessary here as setupTestRouter is called by each test
// func TestMain(m *testing.M) {
// 	os.Exit(m.Run())
// }


func TestProcessPaymentHandler_ValidRequest_MockSuccess(t *testing.T) {
	router := setupTestRouter()

	payload := externalv1.ExternalRequest{
		RequestId:  "testreq001",
		MerchantId: "merchant456", // This merchant uses mock_success by default
		Amount:     1000,
		Currency:   "EUR",
		Customer:   &externalv1.CustomerDetails{Name: "Test Customer", Email: "test@example.com"},
		PaymentMethod: &externalv1.PaymentMethod{
			Method: &externalv1.PaymentMethod_Card{ // Using CardInfo as oneof, actual value doesn't matter for mock
				Card: &externalv1.CardInfo{CardNumber: "tok_mocksuccess"},
			},
		},
	}
	body, errMarshal := protojson.Marshal(&payload)
	require.NoError(t, errMarshal)
	req, _ := http.NewRequest("POST", "/process-payment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "HTTP status code should be OK for successful mock payment")

	var paymentResult orchestrator.PaymentResult
	err := json.Unmarshal(rr.Body.Bytes(), &paymentResult)
	require.NoError(t, err, "Should be able to unmarshal response")
	assert.Equal(t, "SUCCESS", paymentResult.Status)
	require.NotEmpty(t, paymentResult.StepResults)
	assert.True(t, paymentResult.StepResults[0].Success)
	assert.Equal(t, "mock_success", paymentResult.StepResults[0].ProviderName)
}

func TestProcessPaymentHandler_ValidRequest_Stripe_Simulated(t *testing.T) {
    if os.Getenv("SKIP_STRIPE_INTEGRATION_TESTS") == "true" {
        t.Skip("Skipping Stripe integration test for handler")
    }

	router := setupTestRouter()
	payload := externalv1.ExternalRequest{
		RequestId:  "testreq_stripe_001",
		MerchantId: "merchant123", // Uses stripe by default
		Amount:     1500,
		Currency:   "USD",
		Customer:   &externalv1.CustomerDetails{Name: "Stripe User"},
		PaymentMethod: &externalv1.PaymentMethod{
			Method: &externalv1.PaymentMethod_Card{Card: &externalv1.CardInfo{CardNumber: "unused_for_default_token"}},
		},
		// StripeAdapter uses "tok_visa" by default if "stripe_token" is not in ProviderPayload.
		// PlanBuilder currently doesn't move ExternalRequest.Metadata into Step.ProviderPayload by default.
		// So this test will rely on StripeAdapter's default "tok_visa".
	}
	body, errMarshal := protojson.Marshal(&payload)
	require.NoError(t, errMarshal)
	req, _ := http.NewRequest("POST", "/process-payment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var paymentResult orchestrator.PaymentResult
	err := json.Unmarshal(rr.Body.Bytes(), &paymentResult)
	require.NoError(t, err)

    // This test is more of an integration test. The actual outcome depends on:
    // 1. The configured Stripe API key ("sk_test_yourstripeapikey")
    // 2. Stripe's test environment behavior with "tok_visa" (usually succeeds or requires specific amounts for errors)
    // 3. Fallback logic (Router is configured with "mock_success" as fallback)

	if paymentResult.Status == "SUCCESS" {
		assert.Equal(t, http.StatusOK, rr.Code)
		require.NotEmpty(t, paymentResult.StepResults)
		firstStepResult := paymentResult.StepResults[0]
		assert.True(t, firstStepResult.Success)
		// It could be "stripe" if the test key and "tok_visa" worked, or "mock_success" if Stripe failed and fallback occurred.
		assert.True(t, firstStepResult.ProviderName == "stripe" || firstStepResult.ProviderName == "mock_success", "Provider should be stripe or mock_success")
		if firstStepResult.ProviderName == "mock_success" {
		    assert.Equal(t, "true", firstStepResult.Details["is_fallback"], "Should be marked as fallback if mock_success was used")
		}
	} else if paymentResult.Status == "FAILURE" {
        assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
        require.NotEmpty(t, paymentResult.StepResults)
        assert.False(t, paymentResult.StepResults[0].Success)
        // If Stripe failed and fallback also failed (if mock_success was configured to fail, which it isn't by default)
	} else { // PARTIAL_SUCCESS or other unexpected statuses
	     t.Logf("Stripe test payment result: Code=%d, Status=%s, Results=%+v", rr.Code, paymentResult.Status, paymentResult.StepResults)
	}
}


func TestProcessPaymentHandler_InvalidJSON(t *testing.T) {
	router := setupTestRouter()

	invalidJson := []byte(`{"amount": 100, "currency": "USD", malformed...`)
	req, _ := http.NewRequest("POST", "/process-payment", bytes.NewBuffer(invalidJson))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var errorResponse map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	require.NoError(t, err)
	assert.Contains(t, errorResponse["error"], "Invalid request payload")
}

func TestProcessPaymentHandler_MerchantNotFound(t *testing.T) {
    router := setupTestRouter()
    payload := externalv1.ExternalRequest{
		RequestId:  "test_merchant_not_found",
		MerchantId: "nonexistent_merchant",
		Amount:     100, Currency: "USD",
	}
	body, errMarshal := protojson.Marshal(&payload)
	require.NoError(t, errMarshal)
	req, _ := http.NewRequest("POST", "/process-payment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Expected 500 when merchant config is not found")
	var errorResponse map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	require.NoError(t, err)
	assert.Contains(t, errorResponse["error"], "Failed to build request context")
	assert.Contains(t, errorResponse["error"], "merchant config not found for ID: nonexistent_merchant")
}
