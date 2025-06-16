package main

import (
	"bytes"
	"encoding/json" // Keep for unmarshalling response
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/payment-orchestrator/internal/orchestrator" // For PaymentResult
)

// setupTestRouter helper function (uses setupRouter from main.go)
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return setupRouter() // Uses the refactored function from main.go
}

func TestProcessPayment_ValidRequest(t *testing.T) {
	router := setupTestRouter()

	// Construct payload as a map to ensure correct JSON structure for protobuf oneof
	payloadMap := map[string]interface{}{
		"merchant_id": "merchant-123",
		"amount":      1000,
		"currency":    "USD",
		"customer": map[string]interface{}{
			"name": "cust-abc",
		},
		"payment_method": map[string]interface{}{
			"card": map[string]interface{}{
				"card_number":  "1234567812345678",
				"expiry_year":  "2025",
				"expiry_month": "12",
				"cvv":          "123",
			},
		},
	}
	jsonValue, err := json.Marshal(payloadMap)
	require.NoError(t, err, "Failed to marshal payload")

	req, err := http.NewRequest(http.MethodPost, "/process-payment", bytes.NewBuffer(jsonValue))
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Status code should be OK")

	var responseBody orchestrator.PaymentResult
	err = json.Unmarshal(w.Body.Bytes(), &responseBody)
	require.NoError(t, err, "Failed to unmarshal response body")

	assert.NotEmpty(t, responseBody.PaymentID, "PaymentID should not be empty")             // Corrected field name
	assert.Equal(t, "SUCCESS", responseBody.Status, "Payment status should be SUCCESS") // Used string literal
	// assert.Equal(t, "merchant-123", responseBody.MerchantId, "MerchantID should match request") // Removed, MerchantId not in PaymentResult
}

func TestProcessPayment_InvalidRequest_BindingError(t *testing.T) {
	router := setupTestRouter()

	req, err := http.NewRequest(http.MethodPost, "/process-payment", bytes.NewBufferString("this is not json"))
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Status code should be Bad Request")

	var errorResponse gin.H
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse["error"], "Invalid request format", "Error message mismatch")
}

func TestProcessPayment_InvalidRequest_ValidationError_MissingMerchantID(t *testing.T) {
	router := setupTestRouter()

	// Construct payload as a map to ensure correct JSON structure
	payloadMap := map[string]interface{}{
		"merchant_id": "", // Invalid
		"amount":      1000,
		"currency":    "USD",
	}
	jsonValue, err := json.Marshal(payloadMap)
	require.NoError(t, err, "Failed to marshal payloadMap")

	req, err := http.NewRequest(http.MethodPost, "/process-payment", bytes.NewBuffer(jsonValue))
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Status code should be Bad Request")

	var errorResponse gin.H
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse["error"], "Validation failed: MerchantId is required", "Error message mismatch")
}

func TestProcessPayment_InvalidRequest_ValidationError_InvalidAmount(t *testing.T) {
	router := setupTestRouter()

	// Construct payload as a map to ensure correct JSON structure
	payloadMap := map[string]interface{}{
		"merchant_id": "merchant-123",
		"amount":      0, // Invalid
		"currency":    "USD",
	}
	jsonValue, err := json.Marshal(payloadMap)
	require.NoError(t, err, "Failed to marshal payloadMap")

	req, err := http.NewRequest(http.MethodPost, "/process-payment", bytes.NewBuffer(jsonValue))
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Status code should be Bad Request")

	var errorResponse gin.H
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse["error"], "Validation failed: Amount must be positive", "Error message mismatch")
}

func TestProcessPayment_InvalidRequest_ValidationError_MissingCurrency(t *testing.T) {
	router := setupTestRouter()

	// Construct payload as a map to ensure correct JSON structure
	payloadMap := map[string]interface{}{
		"merchant_id": "merchant-123",
		"amount":      1000,
		"currency":    "", // Invalid
	}
	jsonValue, err := json.Marshal(payloadMap)
	require.NoError(t, err, "Failed to marshal payloadMap")

	req, err := http.NewRequest(http.MethodPost, "/process-payment", bytes.NewBuffer(jsonValue))
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Status code should be Bad Request")

	var errorResponse gin.H
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse["error"], "Validation failed: Currency is required", "Error message mismatch")
}

// Optional: TestProcessPayment_OrchestratorError (Simplified: Test for an early 500 due to config)
// To properly test this for router.NewRouter or policy.NewPaymentPolicyEnforcer failing,
// we would need to modify processPaymentHandler to allow injecting configurations or mocks for these components.
// For this subtask, this specific type of internal server error test is complex to set up without further
// refactoring of processPaymentHandler for dependency injection of its internal components.
// A more straightforward test for a 500 would be if one of the Build calls failed, e.g. if BuildContexts
// was made to error. However, the current mock setup makes these paths always succeed.

// The current structure of `processPaymentHandler` initializes all its dependencies internally.
// To test a 500 error from `router.NewRouter` failing (as an example for "Internal server configuration error"),
// one would typically inject a faulty configuration or a mock that forces `router.NewRouter` to return an error.
// Since `processPaymentHandler` creates all dependencies itself, this isn't directly testable without
// changing `processPaymentHandler`'s signature or using global variables for mocks, which is not ideal.

// For now, the happy path and input validation error paths cover a good portion of the handler.
// Deeper internal error testing would require more advanced DI patterns for processPaymentHandler's internals.
