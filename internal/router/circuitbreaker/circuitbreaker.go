package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the state of the circuit breaker.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

const (
	defaultFailureThreshold    = 5       // Number of failures to open the circuit
	defaultOpenStateTimeout    = 30 * time.Second // Time before transitioning from Open to HalfOpen
	defaultHalfOpenSuccessThreshold = 2 // Number of successful requests in HalfOpen to close circuit
)

// providerState holds the current state for a single provider.
type providerState struct {
	state                 State
	consecutiveFailures   int
	consecutiveSuccesses  int       // Used in HalfOpen state
	lastFailureTime       time.Time
	openUntil             time.Time // Time when the circuit breaker will transition from Open to HalfOpen
}

// CircuitBreaker monitors provider health and prevents calls to unhealthy providers.
// This is a basic in-memory implementation.
type CircuitBreaker struct {
	mu              sync.RWMutex
	providers       map[string]*providerState
	failureThreshold    int
	openStateTimeout    time.Duration
	halfOpenSuccessThreshold int
}

// NewCircuitBreaker creates a new CircuitBreaker with default settings.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		providers: make(map[string]*providerState),
		failureThreshold: defaultFailureThreshold,
		openStateTimeout: defaultOpenStateTimeout,
		halfOpenSuccessThreshold: defaultHalfOpenSuccessThreshold,
	}
}

// NewCircuitBreakerWithSettings creates a new CircuitBreaker with custom settings.
func NewCircuitBreakerWithSettings(failThreshold int, openTimeout time.Duration, halfOpenSuccess int) *CircuitBreaker {
    return &CircuitBreaker{
		providers: make(map[string]*providerState),
		failureThreshold: failThreshold,
		openStateTimeout: openTimeout,
		halfOpenSuccessThreshold: halfOpenSuccess,
	}
}


func (cb *CircuitBreaker) getProviderState(providerName string) *providerState {
	// This method assumes a write lock is held by the caller if creation is possible.
	// If only read is needed and provider must exist, RLock would be used by caller.
	ps, exists := cb.providers[providerName]
	if !exists {
		ps = &providerState{state: Closed}
		cb.providers[providerName] = ps
	}
	return ps
}

// IsHealthy (or AllowRequest) checks if requests are allowed for a provider.
func (cb *CircuitBreaker) IsHealthy(providerName string) bool {
	cb.mu.Lock() // Needs full lock because it can change state from Open to HalfOpen
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName)

	switch ps.state {
	case Closed:
		return true
	case Open:
		if time.Now().After(ps.openUntil) {
			// Timeout expired, transition to HalfOpen
			ps.state = HalfOpen
			ps.consecutiveSuccesses = 0 // Reset for HalfOpen state
			return true // Allow requests in HalfOpen
		}
		return false // Still Open
	case HalfOpen:
		// In HalfOpen, allow requests to test the provider.
		// The RecordSuccess/RecordFailure will determine if it closes or re-opens.
		return true
	default:
		// Should not happen, but default to allowing requests to be safe if state is unknown
		ps.state = Closed
		return true
	}
}

// RecordFailure records a failure for the provider.
func (cb *CircuitBreaker) RecordFailure(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName)
	ps.lastFailureTime = time.Now()

	switch ps.state {
	case Closed:
		ps.consecutiveFailures++
		if ps.consecutiveFailures >= cb.failureThreshold {
			ps.state = Open
			ps.openUntil = time.Now().Add(cb.openStateTimeout)
			// ps.consecutiveFailures = 0 // Reset for when it re-enters Closed - No, keep it to show history until success
		}
	case HalfOpen:
		// Failure in HalfOpen state, re-open the circuit immediately.
		ps.state = Open
		ps.openUntil = time.Now().Add(cb.openStateTimeout)
		ps.consecutiveFailures = 0 // Reset failures as it's now Open
		ps.consecutiveSuccesses = 0
	case Open:
	    // If it was already open and somehow a request went through and failed (e.g. timing),
	    // it just remains open. openUntil is not extended here by default,
	    // but could be a strategy (e.g., if failures continue even during supposed open state).
		return
	}
}

// RecordSuccess records a success for the provider.
func (cb *CircuitBreaker) RecordSuccess(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName)

	switch ps.state {
	case Closed:
		ps.consecutiveFailures = 0 // Reset failures on success
	case HalfOpen:
		ps.consecutiveSuccesses++
		if ps.consecutiveSuccesses >= cb.halfOpenSuccessThreshold {
			ps.state = Closed // Transition back to Closed
			ps.consecutiveFailures = 0
			ps.consecutiveSuccesses = 0
		}
	case Open:
        // A success reported while Open is unusual as IsHealthy should prevent calls.
        // However, if it happens (e.g., manual intervention or a race),
        // this could be a signal to reset. For this implementation,
        // success only matters in HalfOpen or Closed. It doesn't transition from Open on success.
		return
	}
}

// GetState returns the current state of a provider's circuit for testing or monitoring.
// This method is read-only and does not transition state (Open->HalfOpen).
// State transitions are handled by IsHealthy or Record calls.
func (cb *CircuitBreaker) GetState(providerName string) State {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    ps, exists := cb.providers[providerName]
	if !exists {
		return Closed // Default to closed if never seen
	}
    return ps.state
}
