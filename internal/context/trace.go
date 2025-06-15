package context

import (
	"context" // Standard context package
	"github.com/google/uuid"
)

// TraceContext carries cross-cutting concerns for observability.
// It includes a standard context.Context for propagation with OpenTelemetry.
type TraceContext struct {
	stdCtx  context.Context   // Standard context for propagation
	TraceID string            // Globally unique ID for logs and spans
	SpanID  string            // Current span identifier (can be updated with NewSpan)
	Baggage map[string]string // Optional key-value flags
}

// NewTraceContext creates a new TraceContext.
// If parentCtx is nil, context.Background() is used.
// It initializes with a new TraceID and SpanID.
func NewTraceContext(parentCtx context.Context) TraceContext {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return TraceContext{
		stdCtx:  parentCtx,
		TraceID: uuid.NewString(),
		SpanID:  uuid.NewString(), // Initial span
		Baggage: make(map[string]string),
	}
}

// NewTraceContextWithIDs creates a new TraceContext using an existing standard context,
// trace ID, and span ID. This is useful when creating a TraceContext from an
// existing OpenTelemetry span.
func NewTraceContextWithIDs(ctx context.Context, traceID string, spanID string) TraceContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return TraceContext{
		stdCtx:  ctx,
		TraceID: traceID,
		SpanID:  spanID,
		Baggage: make(map[string]string),
	}
}

// Context returns the underlying standard context.Context.
func (tc *TraceContext) Context() context.Context {
	if tc.stdCtx == nil {
		return context.Background() // Should not happen if constructed properly
	}
	return tc.stdCtx
}

// GetTraceID returns the TraceID.
func (tc *TraceContext) GetTraceID() string {
	return tc.TraceID
}

// GetSpanID returns the current SpanID.
func (tc *TraceContext) GetSpanID() string {
	return tc.SpanID
}

// NewSpan updates the TraceContext with a new SpanID for a child operation
// within the same trace. It returns the new SpanID.
// Note: This modifies the existing TraceContext.
func (tc *TraceContext) NewSpan() string {
	newSpanID := uuid.NewString()
	tc.SpanID = newSpanID
	return newSpanID
}

// WithStdContext returns a new TraceContext with the stdCtx replaced.
// Useful for updating the context after it's modified by OpenTelemetry.
func (tc TraceContext) WithStdContext(ctx context.Context) TraceContext {
	tc.stdCtx = ctx
	return tc
}
