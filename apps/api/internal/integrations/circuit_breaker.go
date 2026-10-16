package integrations

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is tripping to protect downstream rails.
var ErrCircuitOpen = errors.New("circuit breaker: rail endpoint circuit is open (failing fast)")

// State represents the state of a circuit breaker.
type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF_OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker isolates failing banking or telecom endpoints to prevent cascading timeouts.
type CircuitBreaker struct {
	mu            sync.RWMutex
	name          string
	maxFailures   int
	resetTimeout  time.Duration
	state         State
	failures      int
	lastFailureAt time.Time
	now           func() time.Time
}

// NewCircuitBreaker constructs a circuit breaker with threshold and timeout parameters.
func NewCircuitBreaker(name string, maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
		now:          time.Now,
	}
}

// State returns the current circuit state, transitioning from OPEN to HALF_OPEN after timeout.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.evaluateState()
	return cb.state
}

func (cb *CircuitBreaker) evaluateState() {
	if cb.state == StateOpen {
		now := cb.now()
		if now.Sub(cb.lastFailureAt) >= cb.resetTimeout {
			cb.state = StateHalfOpen
		}
	}
}

// Execute wraps an outbound integration call with circuit safety.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	cb.evaluateState()
	if cb.state == StateOpen {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failures++
		cb.lastFailureAt = cb.now()
		if cb.state == StateHalfOpen || cb.failures >= cb.maxFailures {
			cb.state = StateOpen
		}
		return err
	}

	// Success
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
	} else if cb.failures > 0 {
		cb.failures = 0
	}
	return nil
}
