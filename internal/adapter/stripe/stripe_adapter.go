package stripe

import (
	// "bytes" // Will be added back when Process is implemented if needed
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"log"


	"github.com/google/uuid"
	// "github.com/yourorg/payment-orchestrator/internal/adapter" // For interface compliance
	orchestratorinternalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
)

// StripeAdapter handles communication with the Stripe API.
type StripeAdapter struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
}

const MaxRetries = 1 // Exported for test access if needed, or keep unexported if only internal
const retryDelay = 500 * time.Millisecond // Will be used by Process method


// NewStripeAdapter creates a new instance of StripeAdapter.
func NewStripeAdapter(client *http.Client, apiKey string, apiEndpoint string) *StripeAdapter {
	if client == nil {
		client = &http.Client{}
	}
	if apiEndpoint == "" {
		apiEndpoint = "https://api.stripe.com/v1/payment_intents"
	}
	if apiKey == "" {
		log.Println("StripeAdapter: Warning - API key is empty.")
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
	}
	return strings.NewReader(data.Encode()), nil
}

// StripeError, StripeErrorResponse, StripePaymentIntent structs and Process method will be added next.
// GetName method will also be added to satisfy the ProviderAdapter interface.
