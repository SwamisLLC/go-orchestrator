package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProvider = "test_provider"

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker()
	assert.True(t, cb.IsHealthy(testProvider), "Initially, provider should be healthy (Closed state)")
	assert.Equal(t, Closed, cb.GetState(testProvider))
}

func TestCircuitBreaker_TripToOpenState(t *testing.T) {
	cb := NewCircuitBreakerWithSettings(2, 1*time.Second, 1) // 2 failures to open

	cb.RecordFailure(testProvider) // 1st failure
	assert.True(t, cb.IsHealthy(testProvider), "Still healthy after 1 failure")
	assert.Equal(t, Closed, cb.GetState(testProvider))
	cb.mu.RLock()
	require.NotNil(t, cb.providers[testProvider])
	assert.Equal(t, 1, cb.providers[testProvider].consecutiveFailures)
	cb.mu.RUnlock()

	cb.RecordFailure(testProvider) // 2nd failure, trips the circuit
	cb.mu.RLock()
	require.NotNil(t, cb.providers[testProvider])
	// consecutiveFailures is not reset when tripping from Closed to Open by current logic
	assert.Equal(t, 2, cb.providers[testProvider].consecutiveFailures)
	cb.mu.RUnlock()

	assert.False(t, cb.IsHealthy(testProvider), "Should be unhealthy (Open state) after 2 failures")
	assert.Equal(t, Open, cb.GetState(testProvider))
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	openTimeout := 50 * time.Millisecond
	cb := NewCircuitBreakerWithSettings(1, openTimeout, 1)

	cb.RecordFailure(testProvider) // Trips to Open
	assert.Equal(t, Open, cb.GetState(testProvider))
	assert.False(t, cb.IsHealthy(testProvider), "Should be Open")

	time.Sleep(openTimeout + (10 * time.Millisecond)) // Wait for timeout to expire

	// IsHealthy will transition from Open to HalfOpen if timeout expired
	assert.True(t, cb.IsHealthy(testProvider), "Should transition to HalfOpen and allow a request")
	assert.Equal(t, HalfOpen, cb.GetState(testProvider)) // GetState itself doesn't transition
}

func TestCircuitBreaker_HalfOpen_SuccessToClosed(t *testing.T) {
	openTimeout := 50 * time.Millisecond
	halfOpenSuccessesNeeded := 2
	cb := NewCircuitBreakerWithSettings(1, openTimeout, halfOpenSuccessesNeeded)

	cb.RecordFailure(testProvider) // Open
	time.Sleep(openTimeout + (10 * time.Millisecond))

	require.True(t, cb.IsHealthy(testProvider)) // Transitions to HalfOpen, allows first request
	cb.RecordSuccess(testProvider) // First success in HalfOpen
	assert.Equal(t, HalfOpen, cb.GetState(testProvider), "Still HalfOpen after 1 success (needs 2)")
	cb.mu.RLock()
	require.NotNil(t, cb.providers[testProvider])
    assert.Equal(t, 1, cb.providers[testProvider].consecutiveSuccesses)
    cb.mu.RUnlock()
	assert.True(t, cb.IsHealthy(testProvider)) // Still allows requests

	cb.RecordSuccess(testProvider) // Second success in HalfOpen
	assert.Equal(t, Closed, cb.GetState(testProvider), "Should transition to Closed after 2 successes in HalfOpen")
	cb.mu.RLock()
	require.NotNil(t, cb.providers[testProvider])
    assert.Equal(t, 0, cb.providers[testProvider].consecutiveSuccesses)
    assert.Equal(t, 0, cb.providers[testProvider].consecutiveFailures) // Reset on transition to Closed
    cb.mu.RUnlock()
	assert.True(t, cb.IsHealthy(testProvider))
}

func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	openTimeout := 50 * time.Millisecond
	cb := NewCircuitBreakerWithSettings(1, openTimeout, 1)

	cb.RecordFailure(testProvider) // Open
	time.Sleep(openTimeout + (10 * time.Millisecond))

	require.True(t, cb.IsHealthy(testProvider)) // Transitions to HalfOpen
	assert.Equal(t, HalfOpen, cb.GetState(testProvider))


	cb.RecordFailure(testProvider) // Failure in HalfOpen
	assert.Equal(t, Open, cb.GetState(testProvider), "Should re-open after failure in HalfOpen")
	assert.False(t, cb.IsHealthy(testProvider))
	cb.mu.RLock()
	require.NotNil(t, cb.providers[testProvider])
    assert.Equal(t, 0, cb.providers[testProvider].consecutiveFailures) // Reset when moving from HalfOpen to Open
    assert.Equal(t, 0, cb.providers[testProvider].consecutiveSuccesses)
    cb.mu.RUnlock()
}

func TestCircuitBreaker_SuccessResetsFailuresInClosed(t *testing.T) {
    cb := NewCircuitBreakerWithSettings(3, 1*time.Second, 1)

    cb.RecordFailure(testProvider) // 1 failure
    cb.RecordFailure(testProvider) // 2 failures
    assert.Equal(t, Closed, cb.GetState(testProvider))

    // Access provider state directly for detailed assertion (requires careful locking in real code if exposed)
    cb.mu.RLock()
    internalPS := cb.providers[testProvider]
    assert.Equal(t, 2, internalPS.consecutiveFailures)
    cb.mu.RUnlock()

    cb.RecordSuccess(testProvider) // Success should reset failures
    cb.mu.RLock()
    internalPS = cb.providers[testProvider]
    assert.Equal(t, 0, internalPS.consecutiveFailures)
    cb.mu.RUnlock()
    assert.True(t, cb.IsHealthy(testProvider))

    cb.RecordFailure(testProvider) // 1 failure again
    cb.mu.RLock()
    internalPS = cb.providers[testProvider]
    assert.Equal(t, 1, internalPS.consecutiveFailures)
    cb.mu.RUnlock()
    assert.True(t, cb.IsHealthy(testProvider))
}

func TestCircuitBreaker_MultipleProviders(t *testing.T) {
    cb := NewCircuitBreakerWithSettings(1, 100*time.Millisecond, 1)
    provider1 := "p1"
    provider2 := "p2"

    cb.RecordFailure(provider1) // p1 opens
    assert.Equal(t, Open, cb.GetState(provider1))
    assert.False(t, cb.IsHealthy(provider1))
    assert.True(t, cb.IsHealthy(provider2), "p2 should remain Closed")

    cb.RecordFailure(provider2) // p2 opens
    assert.Equal(t, Open, cb.GetState(provider2))
    assert.False(t, cb.IsHealthy(provider2))

    time.Sleep(110 * time.Millisecond) // Both should be ready for HalfOpen

    assert.True(t, cb.IsHealthy(provider1)) // Transitions p1 to HalfOpen
    assert.Equal(t, HalfOpen, cb.GetState(provider1)) // GetState reflects current state

    assert.True(t, cb.IsHealthy(provider2)) // Transitions p2 to HalfOpen
    assert.Equal(t, HalfOpen, cb.GetState(provider2))

    cb.RecordSuccess(provider1) // p1 Closes
    assert.Equal(t, Closed, cb.GetState(provider1))

    cb.RecordFailure(provider2) // p2 Re-Opens from HalfOpen
    assert.Equal(t, Open, cb.GetState(provider2))
}

func TestCircuitBreaker_GetState_OpenToHalfOpenTransitionByTime(t *testing.T) {
    openTimeout := 50 * time.Millisecond
    cb := NewCircuitBreakerWithSettings(1, openTimeout, 1)

    cb.RecordFailure(testProvider) // Trips to Open
    assert.Equal(t, Open, cb.GetState(testProvider), "Should be Open initially")

    time.Sleep(openTimeout + 10 * time.Millisecond) // Wait for timeout

    // GetState itself does not transition. IsHealthy does.
    // The logic in GetState for Open -> HalfOpen based on time was removed for simplicity
    // as transitions are driven by IsHealthy or Record calls.
    assert.Equal(t, Open, cb.GetState(testProvider), "GetState should still show Open even if time passed")

    // Now call IsHealthy to trigger the transition
    assert.True(t, cb.IsHealthy(testProvider), "IsHealthy should allow request and transition to HalfOpen")
    assert.Equal(t, HalfOpen, cb.GetState(testProvider), "State should now be HalfOpen")
}
