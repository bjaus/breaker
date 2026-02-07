package breaker

import "time"

type config struct {
	failureThreshold int
	successThreshold int
	openDuration     time.Duration
	halfOpenRequests int
	condition        Condition
	clock            Clock

	onStateChange OnStateChangeFunc
	onCall        OnCallFunc
	onReject      OnRejectFunc
}

// Option configures a Circuit.
type Option func(*config)

// WithFailureThreshold sets consecutive failures before opening the circuit.
// Default is 5.
func WithFailureThreshold(n int) Option {
	return func(c *config) {
		c.failureThreshold = n
	}
}

// WithSuccessThreshold sets consecutive successes in half-open state
// required before closing the circuit. Default is 2.
func WithSuccessThreshold(n int) Option {
	return func(c *config) {
		c.successThreshold = n
	}
}

// WithOpenDuration sets how long the circuit stays open before
// transitioning to half-open. Default is 30 seconds.
func WithOpenDuration(d time.Duration) Option {
	return func(c *config) {
		c.openDuration = d
	}
}

// WithHalfOpenRequests sets how many requests are allowed through
// in the half-open state. Default is 1.
func WithHalfOpenRequests(n int) Option {
	return func(c *config) {
		c.halfOpenRequests = n
	}
}

// If sets the condition that determines whether an error counts as a failure.
// By default, any non-nil error is a failure.
func If(cond Condition) Option {
	return func(c *config) {
		c.condition = cond
	}
}

// IfNot sets a condition where matching errors are NOT counted as failures.
// This is equivalent to If(Not(cond)).
func IfNot(cond Condition) Option {
	return If(Not(cond))
}

// Not inverts a condition.
func Not(cond Condition) Condition {
	return func(err error) bool {
		return !cond(err)
	}
}

// WithClock sets the clock for time operations. Useful for testing.
func WithClock(clock Clock) Option {
	return func(c *config) {
		c.clock = clock
	}
}

// OnStateChange sets a hook called when the circuit changes state.
func OnStateChange(fn OnStateChangeFunc) Option {
	return func(c *config) {
		c.onStateChange = fn
	}
}

// OnCall sets a hook called after each call attempt.
func OnCall(fn OnCallFunc) Option {
	return func(c *config) {
		c.onCall = fn
	}
}

// OnReject sets a hook called when a call is rejected due to open circuit.
func OnReject(fn OnRejectFunc) Option {
	return func(c *config) {
		c.onReject = fn
	}
}
