package circuitbreaker_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
)

const (
	testProvider = "test-provider"
	anotherProvider = "another-provider"
)

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		cfg := circuitbreaker.Config{}
		cb := circuitbreaker.NewCircuitBreaker(cfg)
		require.NotNil(t, cb)
		assert.True(t, cb.AllowRequest(testProvider), "Should allow by default")
		cb.RecordFailure(testProvider)
		cb.RecordFailure(testProvider)
		assert.True(t, cb.AllowRequest(testProvider), "Should still be closed after 2 failures")
		cb.RecordFailure(testProvider)
		assert.False(t, cb.AllowRequest(testProvider), "Should be open after 3 failures with default config")
	})

	t.Run("Custom config", func(t *testing.T) {
		cfg := circuitbreaker.Config{
			FailureThreshold: 2,
			ResetTimeout:     100 * time.Millisecond,
		}
		cb := circuitbreaker.NewCircuitBreaker(cfg)
		require.NotNil(t, cb)
		cb.RecordFailure(testProvider)
		assert.True(t, cb.AllowRequest(testProvider), "Should still be closed after 1 failure")
		cb.RecordFailure(testProvider)
		assert.False(t, cb.AllowRequest(testProvider), "Should be open after 2 failures with custom config")
	})
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cfg := circuitbreaker.Config{
		FailureThreshold:     2,
		ResetTimeout:         50 * time.Millisecond,
	}

	t.Run("Closed_To_Open", func(t *testing.T) {
		cb := circuitbreaker.NewCircuitBreaker(cfg)

		assert.True(t, cb.AllowRequest(testProvider), "Should be initially Closed and allow requests")
		state, failures := cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateClosed, state)
		assert.Equal(t, 0, failures)

		cb.RecordFailure(testProvider)
		state, failures = cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateClosed, state)
		assert.Equal(t, 1, failures)
		assert.True(t, cb.AllowRequest(testProvider), "Still Closed after 1 failure")

		cb.RecordFailure(testProvider)
		state, failures = cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateOpen, state, "Should transition to Open")
		assert.Equal(t, cfg.FailureThreshold, failures)
		assert.False(t, cb.AllowRequest(testProvider), "Should be Open and block requests")
	})

	t.Run("Open_To_HalfOpen", func(t *testing.T) {
		cb := circuitbreaker.NewCircuitBreaker(cfg)
		cb.RecordFailure(testProvider)
		cb.RecordFailure(testProvider)
		require.False(t, cb.AllowRequest(testProvider), "Pre-condition: Should be Open")
		state, _ := cb.GetProviderStatus(testProvider)
		require.Equal(t, circuitbreaker.StateOpen, state)

		time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)

		assert.True(t, cb.AllowRequest(testProvider), "Should allow request (transition to HalfOpen)")
		state, failures := cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateHalfOpen, state, "State should be HalfOpen")
		assert.Equal(t, 0, failures, "Consecutive failures should reset in HalfOpen")
	})

	t.Run("HalfOpen_To_Closed_OnSuccess", func(t *testing.T) {
		cb := circuitbreaker.NewCircuitBreaker(cfg)
		cb.RecordFailure(testProvider)
		cb.RecordFailure(testProvider)
		time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)
		require.True(t, cb.AllowRequest(testProvider), "Should allow request in HalfOpen")
		state, _ := cb.GetProviderStatus(testProvider)
		require.Equal(t, circuitbreaker.StateHalfOpen, state, "Pre-condition: Should be HalfOpen")

		cb.RecordSuccess(testProvider)
		state, failures := cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateClosed, state, "Should transition to Closed after success in HalfOpen")
		assert.Equal(t, 0, failures, "Failures should be reset")
		assert.True(t, cb.AllowRequest(testProvider), "Should allow requests in Closed state")
	})

	t.Run("HalfOpen_To_Open_OnFailure", func(t *testing.T) {
		cb := circuitbreaker.NewCircuitBreaker(cfg)
		cb.RecordFailure(testProvider)
		cb.RecordFailure(testProvider)
		time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)
		require.True(t, cb.AllowRequest(testProvider), "Should allow request in HalfOpen")
		state, _ := cb.GetProviderStatus(testProvider)
		require.Equal(t, circuitbreaker.StateHalfOpen, state, "Pre-condition: Should be HalfOpen")

		cb.RecordFailure(testProvider)
		state, failures := cb.GetProviderStatus(testProvider)
		assert.Equal(t, circuitbreaker.StateOpen, state, "Should transition back to Open after failure in HalfOpen")
		assert.Equal(t, cfg.FailureThreshold, failures, "Failures should be set to threshold")
		assert.False(t, cb.AllowRequest(testProvider), "Should block requests in Open state")

		time.Sleep(cfg.ResetTimeout / 2)
		assert.False(t, cb.AllowRequest(testProvider), "Should still be Open before ResetTimeout passes again")
	})
}

func TestCircuitBreaker_FailuresBelowThreshold(t *testing.T) {
	cfg := circuitbreaker.Config{FailureThreshold: 3}
	cb := circuitbreaker.NewCircuitBreaker(cfg)

	cb.RecordFailure(testProvider)
	state, failures := cb.GetProviderStatus(testProvider)
	assert.Equal(t, circuitbreaker.StateClosed, state)
	assert.Equal(t, 1, failures)
	assert.True(t, cb.AllowRequest(testProvider))

	cb.RecordFailure(testProvider)
	state, failures = cb.GetProviderStatus(testProvider)
	assert.Equal(t, circuitbreaker.StateClosed, state)
	assert.Equal(t, 2, failures)
	assert.True(t, cb.AllowRequest(testProvider))

	cb.RecordSuccess(testProvider)
	state, failures = cb.GetProviderStatus(testProvider)
	assert.Equal(t, circuitbreaker.StateClosed, state)
	assert.Equal(t, 0, failures)
	assert.True(t, cb.AllowRequest(testProvider))
}

func TestCircuitBreaker_MultipleProviders(t *testing.T) {
	cfg := circuitbreaker.Config{FailureThreshold: 1, ResetTimeout: 50 * time.Millisecond}
	cb := circuitbreaker.NewCircuitBreaker(cfg)

	cb.RecordFailure(testProvider)
	assert.False(t, cb.AllowRequest(testProvider), "Provider1 should be Open")

	assert.True(t, cb.AllowRequest(anotherProvider), "Provider2 should be Closed and allow requests")
	cb.RecordFailure(anotherProvider)
	assert.False(t, cb.AllowRequest(anotherProvider), "Provider2 should now be Open")

	assert.False(t, cb.AllowRequest(testProvider), "Provider1 should still be Open")

	time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)
	assert.True(t, cb.AllowRequest(testProvider), "Provider1 should be HalfOpen")
	cb.RecordSuccess(testProvider)
	assert.True(t, cb.AllowRequest(testProvider), "Provider1 should be Closed")

	time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)
	assert.True(t, cb.AllowRequest(anotherProvider), "Provider2 should also be HalfOpen after its timeout")
	cb.RecordSuccess(anotherProvider)
	assert.True(t, cb.AllowRequest(anotherProvider), "Provider2 should be Closed")
}

func TestCircuitBreaker_Idempotency(t *testing.T) {
	cfg := circuitbreaker.Config{FailureThreshold: 1}
	cb := circuitbreaker.NewCircuitBreaker(cfg)

	cb.RecordFailure(testProvider)
	state, failures := cb.GetProviderStatus(testProvider)
	assert.Equal(t, circuitbreaker.StateOpen, state)
	assert.Equal(t, 1, failures)

	cb.RecordFailure(testProvider)
	state, failures = cb.GetProviderStatus(testProvider)
	assert.Equal(t, circuitbreaker.StateOpen, state)
	assert.Equal(t, 1, failures)

	cb.RecordSuccess(anotherProvider)
	state, failures = cb.GetProviderStatus(anotherProvider)
	assert.Equal(t, circuitbreaker.StateClosed, state)
	assert.Equal(t, 0, failures)
	cb.RecordSuccess(anotherProvider)
	state, failures = cb.GetProviderStatus(anotherProvider)
	assert.Equal(t, circuitbreaker.StateClosed, state)
	assert.Equal(t, 0, failures)
}

func TestCircuitBreaker_AllowRequest_CreatesState(t *testing.T) {
    cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{})
    assert.True(t, cb.AllowRequest("new-provider"))
    state, failures := cb.GetProviderStatus("new-provider")
    assert.Equal(t, circuitbreaker.StateClosed, state)
    assert.Equal(t, 0, failures)
}

func TestCircuitBreaker_RecordSuccess_UntrackedProvider(t *testing.T) {
    cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{})
    cb.RecordSuccess("untracked-provider")
    state, failures := cb.GetProviderStatus("untracked-provider")
    assert.Equal(t, circuitbreaker.StateClosed, state)
    assert.Equal(t, 0, failures)
}

func TestCircuitBreaker_State_String(t *testing.T) {
	assert.Equal(t, "Closed", circuitbreaker.StateClosed.String())
	assert.Equal(t, "Open", circuitbreaker.StateOpen.String())
	assert.Equal(t, "HalfOpen", circuitbreaker.StateHalfOpen.String())
	assert.Equal(t, "Unknown", circuitbreaker.State(99).String())
}
