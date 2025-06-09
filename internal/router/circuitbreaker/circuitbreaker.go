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
	// HalfOpenSuccessThreshold int // Future: How many successes in half-open to close. For now, 1 success closes.
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
	cb.mu.Lock() // Lock for potential state modification (Open -> HalfOpen)
	defer cb.mu.Unlock()

	ps := cb.getProviderState(providerName) // Ensures state exists

	switch ps.currentState {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(ps.openTime) > cb.config.ResetTimeout {
			// Log.Printf("CircuitBreaker: Provider '%s' transitioning from Open to HalfOpen after timeout.", providerName)
			ps.currentState = StateHalfOpen
			ps.consecutiveFailures = 0 // Reset for half-open test
			return true               // Allow one request in half-open
		}
		return false // Still Open, within timeout
	case StateHalfOpen:
		// Log.Printf("CircuitBreaker: Provider '%s' is HalfOpen, allowing test request.", providerName)
		return true // Allow the test request
	default:
		// Log.Printf("CircuitBreaker: Provider '%s' in unknown state %v, defaulting to allow.", providerName, ps.currentState)
		return true // Should not happen
	}
}

// RecordSuccess records a successful request for the provider.
// It can transition the state from HalfOpen to Closed.
func (cb *CircuitBreaker) RecordSuccess(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps, exists := cb.providerStates[providerName]
	if !exists {
		// If no state exists, it implies AllowRequest was never called or it was for a new provider.
		// A success for an unknown/untracked provider doesn't need to change any state.
		// Or, we could create a state here, but typically AllowRequest would be called first.
		// Log.Printf("CircuitBreaker: RecordSuccess for untracked provider '%s'. No action taken.", providerName)
		return
	}

	if ps.currentState == StateHalfOpen {
		// Log.Printf("CircuitBreaker: Provider '%s' transitioning from HalfOpen to Closed after success.", providerName)
		ps.currentState = StateClosed
		ps.consecutiveFailures = 0
	} else if ps.currentState == StateClosed {
		// If there were some sporadic failures that didn't meet the threshold, a success resets the count.
		if ps.consecutiveFailures > 0 {
			// Log.Printf("CircuitBreaker: Provider '%s' in Closed state, resetting consecutiveFailures from %d to 0 after success.", providerName, ps.consecutiveFailures)
			ps.consecutiveFailures = 0
		}
	}
	// No change if Open (successes during Open state are ignored until it transitions to HalfOpen)
}

// RecordFailure records a failed request for the provider.
// It can transition the state from Closed to Open, or HalfOpen to Open.
func (cb *CircuitBreaker) RecordFailure(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Use getProviderState to ensure state exists, as a failure might be recorded
	// even if AllowRequest wasn't called (e.g., timeout before request could be blocked).
	ps := cb.getProviderState(providerName)
	ps.lastFailureTime = time.Now()

	if ps.currentState == StateHalfOpen {
		// Log.Printf("CircuitBreaker: Provider '%s' transitioning from HalfOpen to Open after failure.", providerName)
		ps.currentState = StateOpen
		ps.openTime = time.Now()
		// consecutiveFailures is already reset when entering HalfOpen, one failure is enough to re-open.
		// We can set it to threshold to ensure it stays open for the full timeout.
		ps.consecutiveFailures = cb.config.FailureThreshold
	} else if ps.currentState == StateClosed {
		ps.consecutiveFailures++
		// Log.Printf("CircuitBreaker: Provider '%s' in Closed state, consecutiveFailures incremented to %d.", providerName, ps.consecutiveFailures)
		if ps.consecutiveFailures >= cb.config.FailureThreshold {
			// Log.Printf("CircuitBreaker: Provider '%s' transitioning from Closed to Open after %d failures.", providerName, ps.consecutiveFailures)
			ps.currentState = StateOpen
			ps.openTime = time.Now()
		}
	}
	// No change if already Open and not yet timed out for HalfOpen attempt.
	// If it was Open and timed out, AllowRequest would have moved it to HalfOpen,
	// and this failure would move it back to Open as per the HalfOpen case above.
}

// GetProviderStatus is a helper to inspect the state (for testing or monitoring).
// Not part of the core CB logic for processing, but useful.
func (cb *CircuitBreaker) GetProviderStatus(providerName string) (State, int) {
	cb.mu.RLock() // Use RLock for read-only access
	defer cb.mu.RUnlock()

	ps, exists := cb.providerStates[providerName]
	if !exists {
		return StateClosed, 0 // Default for unknown provider
	}
	return ps.currentState, ps.consecutiveFailures
}
