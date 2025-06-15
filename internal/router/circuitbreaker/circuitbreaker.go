// Package circuitbreaker implements a circuit breaker mechanism to monitor
// and control requests to payment providers. It tracks the health of each
// provider, opening the circuit (stopping requests) if failure rates exceed
// thresholds, and periodically allowing test requests to check for recovery.
package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the state of the circuit breaker.
type State int

const (
	// StateClosed allows requests to pass through.
	StateClosed State = iota
	// StateOpen blocks requests.
	StateOpen
	// StateHalfOpen allows a limited number of test requests.
	StateHalfOpen
)

// String makes State satisfy the Stringer interface for easier logging.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// providerState holds the current state of a specific provider in the circuit breaker.
type providerState struct {
	name                string
	currentState        State
	consecutiveFailures int
	lastFailureTime     time.Time
	openTime            time.Time // When the circuit was last opened
}

// Config stores the configuration for the CircuitBreaker.
type Config struct {
	FailureThreshold     int           // Number of consecutive failures to open the circuit.
	ResetTimeout         time.Duration // Time after which an Open circuit transitions to HalfOpen.
}

// CircuitBreaker implements the circuit breaker pattern for multiple providers.
type CircuitBreaker struct {
	providerStates map[string]*providerState
	config         Config
	mu             sync.RWMutex // Protects access to providerStates map
}

// NewCircuitBreaker creates a new CircuitBreaker with the given configuration.
// It sets default values if parts of the config are zero.
func NewCircuitBreaker(config Config) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 3 // Default to 3 consecutive failures
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 30 * time.Second // Default to 30 seconds
	}
	return &CircuitBreaker{
		providerStates: make(map[string]*providerState),
		config:         config,
	}
}

// getProviderState retrieves or creates a default (Closed) state for a provider.
// This internal helper must be called with the write lock (cb.mu.Lock()) held.
func (cb *CircuitBreaker) getProviderState(providerName string) *providerState {
	ps, exists := cb.providerStates[providerName]
	if !exists {
		ps = &providerState{
			name:                providerName,
			currentState:        StateClosed,
			consecutiveFailures: 0,
		}
		cb.providerStates[providerName] = ps
	}
	return ps
}

// AllowRequest checks if a request to the given provider should be allowed based on the circuit state.
// It may transition the state from Open to HalfOpen if ResetTimeout has passed.
func (cb *CircuitBreaker) AllowRequest(providerName string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName)

	switch ps.currentState {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(ps.openTime) > cb.config.ResetTimeout {
			ps.currentState = StateHalfOpen
			ps.consecutiveFailures = 0
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful request for the provider.
// It can transition the state from HalfOpen to Closed.
func (cb *CircuitBreaker) RecordSuccess(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps, exists := cb.providerStates[providerName]
	if !exists {
		return
	}

	if ps.currentState == StateHalfOpen {
		ps.currentState = StateClosed
		ps.consecutiveFailures = 0
	} else if ps.currentState == StateClosed {
		if ps.consecutiveFailures > 0 {
			ps.consecutiveFailures = 0
		}
	}
}

// RecordFailure records a failed request for the provider.
// It can transition the state from Closed to Open, or HalfOpen to Open.
func (cb *CircuitBreaker) RecordFailure(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName)
	ps.lastFailureTime = time.Now()

	if ps.currentState == StateHalfOpen {
		ps.currentState = StateOpen
		ps.openTime = time.Now()
		ps.consecutiveFailures = cb.config.FailureThreshold
	} else if ps.currentState == StateClosed {
		ps.consecutiveFailures++
		if ps.consecutiveFailures >= cb.config.FailureThreshold {
			ps.currentState = StateOpen
			ps.openTime = time.Now()
		}
	}
}

// GetProviderStatus is a helper to inspect the state (for testing or monitoring).
func (cb *CircuitBreaker) GetProviderStatus(providerName string) (State, int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	ps, exists := cb.providerStates[providerName]
	if !exists {
		return StateClosed, 0
	}
	return ps.currentState, ps.consecutiveFailures
}
