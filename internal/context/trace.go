package context

import (
	"github.com/google/uuid"
)

// TraceContext carries only cross-cutting concerns needed for observability.
type TraceContext struct {
	TraceID string            // Globally unique ID for logs and spans
	SpanID  string            // Current span identifier
	Baggage map[string]string // Optional key-value flags (e.g., correlation data)
}

// NewTraceContext creates a new TraceContext with a unique TraceID and an initial SpanID.
func NewTraceContext() TraceContext {
	return TraceContext{
		TraceID: uuid.NewString(),
		SpanID:  uuid.NewString(), // Initial span
		Baggage: make(map[string]string),
	}
}

// NewSpan generates a new SpanID for a child operation within the same trace.
func (tc *TraceContext) NewSpan() string {
	tc.SpanID = uuid.NewString()
	return tc.SpanID
}
