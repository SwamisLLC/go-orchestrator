package mock

import (
	// "fmt" // Removed as no longer needed after logging removal
	"time"
	stdcontext "context"

	"github.com/yourorg/payment-orchestrator/internal/adapter"
	"github.com/yourorg/payment-orchestrator/internal/context"
	internalv1 "github.com/yourorg/payment-orchestrator/pkg/gen/protos/orchestratorinternalv1"
	"github.com/google/uuid"
)

// MockAdapter is a mock implementation of the ProviderAdapter interface for testing.
type MockAdapter struct {
	Name          string
	ProcessFunc   func(tc context.TraceContext, step *internalv1.PaymentStep, sc context.StepExecutionContext) (adapter.ProviderResult, error)
	HealthCheckFunc func(ctx stdcontext.Context) error
}

// NewMockAdapter creates a new MockAdapter.
func NewMockAdapter(name string) *MockAdapter {
	return &MockAdapter{Name: name}
}

// Process implements the ProviderAdapter interface.
// It calls ProcessFunc if defined, otherwise returns a default successful result.
func (m *MockAdapter) Process(
	tc context.TraceContext,
	step *internalv1.PaymentStep,
	sc context.StepExecutionContext,
) (adapter.ProviderResult, error) {
	if m.ProcessFunc != nil {
		return m.ProcessFunc(tc, step, sc)
	}

	// Default behavior: success
	latency := time.Since(sc.StartTime).Milliseconds()
	if latency == 0 && step.Amount > 0 {
	    time.Sleep(10 * time.Millisecond)
	    latency = time.Since(sc.StartTime).Milliseconds()
	}

	res := adapter.ProviderResult{
		StepID:        step.StepId,
		Success:       true,
		Provider:      m.Name,
		TransactionID: "",
		LatencyMs:     latency,
		Details:       map[string]string{"mock_processed": "true", "provider_transaction_id": uuid.NewString()},
	}
	return res, nil
}

// GetName implements the ProviderAdapter interface.
func (m *MockAdapter) GetName() string {
	return m.Name
}

// HealthCheck implements an optional part of a ProviderAdapter interface.
// For the mock, it calls HealthCheckFunc if provided.
// func (m *MockAdapter) HealthCheck(ctx stdcontext.Context) error {
// 	if m.HealthCheckFunc != nil {
// 		return m.HealthCheckFunc(ctx)
// 	}
// 	return nil // Default healthy
// }
