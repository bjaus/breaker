// Package breaker implements the circuit breaker pattern for resilient distributed systems.
//
// breaker protects services from cascading failures by:
//
//   - Tracking Failures: Consecutive errors trip the circuit open
//   - Fast Rejection: Open circuits reject calls immediately without load
//   - Gradual Recovery: Half-open state tests if the service has recovered
//   - Lifecycle Hooks: OnStateChange, OnCall, OnReject for observability
//   - Zero Dependencies: Only the Go standard library
//
// # Quick Start
//
// Create a circuit and protect calls:
//
//	circuit := breaker.New("payment-service")
//
//	err := circuit.Do(ctx, func(ctx context.Context) error {
//	    return client.Charge(ctx, amount)
//	})
//	if breaker.IsOpen(err) {
//	    return handleFallback()
//	}
//
// For functions that return values, use the generic Run helper:
//
//	user, err := breaker.Run(ctx, circuit, func(ctx context.Context) (*User, error) {
//	    return client.GetUser(ctx, id)
//	})
//
// # Circuit States
//
// The circuit breaker has three states:
//
//	Closed (normal):
//	    - Requests flow through to the protected function
//	    - Failures are counted
//	    - When failures reach threshold, circuit opens
//
//	Open (tripped):
//	    - Requests are rejected immediately with ErrOpen
//	    - After timeout, circuit transitions to half-open
//
//	HalfOpen (testing):
//	    - Limited requests are allowed through
//	    - Success closes the circuit
//	    - Failure reopens it
//
// # Configuration
//
// Configure thresholds and timing with options:
//
//	circuit := breaker.New("api",
//	    breaker.WithFailureThreshold(5),      // Open after 5 consecutive failures
//	    breaker.WithSuccessThreshold(2),      // Close after 2 consecutive successes
//	    breaker.WithOpenDuration(30*time.Second),  // Wait 30s before half-open
//	    breaker.WithHalfOpenRequests(3),      // Allow 3 requests in half-open
//	)
//
// Default values:
//
//   - FailureThreshold: 5 consecutive failures
//   - SuccessThreshold: 2 consecutive successes
//   - OpenDuration: 30 seconds
//   - HalfOpenRequests: 1 request
//
// # Failure Conditions
//
// By default, any non-nil error counts as a failure. Customize this with If:
//
//	// Only count specific errors as failures
//	circuit := breaker.New("api",
//	    breaker.If(func(err error) bool {
//	        return errors.Is(err, ErrTimeout) || errors.Is(err, ErrUnavailable)
//	    }),
//	)
//
// Use IfNot to exclude certain errors:
//
//	// Don't count 404s as failures
//	circuit := breaker.New("api",
//	    breaker.IfNot(func(err error) bool {
//	        return errors.Is(err, ErrNotFound)
//	    }),
//	)
//
// Use Not to invert any condition:
//
//	isTransient := func(err error) bool { return errors.Is(err, ErrTimeout) }
//	isPermanent := breaker.Not(isTransient)
//
// # Lifecycle Hooks
//
// Hooks provide observability without coupling to a specific logger or metrics system:
//
//	circuit := breaker.New("service",
//	    breaker.OnStateChange(func(name string, from, to breaker.State) {
//	        logger.Info("circuit state change",
//	            "circuit", name,
//	            "from", from,
//	            "to", to,
//	        )
//	        metrics.Gauge("circuit.state", float64(to), "circuit:"+name)
//	    }),
//	    breaker.OnCall(func(name string, state breaker.State, err error) {
//	        if err != nil {
//	            metrics.Increment("circuit.failure", "circuit:"+name)
//	        } else {
//	            metrics.Increment("circuit.success", "circuit:"+name)
//	        }
//	    }),
//	    breaker.OnReject(func(name string) {
//	        metrics.Increment("circuit.rejected", "circuit:"+name)
//	    }),
//	)
//
// Available hooks:
//
//   - OnStateChange: Called when circuit transitions between states
//   - OnCall: Called after each call attempt (success or failure)
//   - OnReject: Called when a call is rejected due to open circuit
//
// # Fallback Pattern
//
// Use IsOpen to detect open circuits and provide fallback behavior:
//
//	func GetUser(ctx context.Context, id string) (*User, error) {
//	    user, err := breaker.Run(ctx, circuit, func(ctx context.Context) (*User, error) {
//	        return client.GetUser(ctx, id)
//	    })
//	    if breaker.IsOpen(err) {
//	        return getCachedUser(id)  // Fallback to cache
//	    }
//	    return user, err
//	}
//
// # Generic Helper
//
// The Run function provides type-safe return values:
//
//	// Returns (T, error) instead of just error
//	result, err := breaker.Run(ctx, circuit, func(ctx context.Context) (MyType, error) {
//	    return doSomething(ctx)
//	})
//
// This avoids the need for closures to capture return values.
//
// # Manual Reset
//
// Reset the circuit to closed state programmatically:
//
//	circuit.Reset()
//
// Useful for admin endpoints or after deploying fixes.
//
// # Inspecting State
//
// Query the circuit's current status:
//
//	state := circuit.State()    // Closed, Open, or HalfOpen
//	name := circuit.Name()      // The circuit's name
//	failures, successes := circuit.Counts()
//
// # Testing
//
// Inject a fake clock to control time in tests:
//
//	type fakeClock struct {
//	    now time.Time
//	}
//
//	func (c *fakeClock) Now() time.Time { return c.now }
//	func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
//
//	func TestCircuitOpensAfterTimeout(t *testing.T) {
//	    clock := &fakeClock{now: time.Now()}
//	    circuit := breaker.New("test",
//	        breaker.WithFailureThreshold(1),
//	        breaker.WithOpenDuration(30*time.Second),
//	        breaker.WithClock(clock),
//	    )
//
//	    // Trip the circuit
//	    _ = circuit.Do(ctx, func(ctx context.Context) error {
//	        return errors.New("fail")
//	    })
//	    assert.Equal(t, breaker.Open, circuit.State())
//
//	    // Advance time past open duration
//	    clock.Advance(31 * time.Second)
//	    assert.Equal(t, breaker.HalfOpen, circuit.State())
//	}
//
// # Best Practices
//
// 1. Name circuits after the service they protect:
//
//	breaker.New("payment-gateway")
//	breaker.New("user-service")
//
// 2. Use hooks for observability instead of wrapping:
//
//	breaker.OnStateChange(func(name string, from, to breaker.State) {
//	    // Log, metric, alert
//	})
//
// 3. Provide fallbacks for open circuits:
//
//	if breaker.IsOpen(err) {
//	    return cachedValue, nil
//	}
//
// 4. Tune thresholds based on your traffic patterns:
//
//	// High-traffic: higher threshold to avoid false positives
//	breaker.WithFailureThreshold(10)
//
//	// Low-traffic: lower threshold for faster detection
//	breaker.WithFailureThreshold(3)
//
// 5. Consider multiple half-open requests for gradual recovery:
//
//	breaker.WithHalfOpenRequests(3)
//	breaker.WithSuccessThreshold(3)
//
// # Comparison to Other Patterns
//
// Circuit breaker vs retry:
//
//   - Retry: Repeats failed calls with backoff
//   - Circuit breaker: Stops calling after repeated failures
//
// They work well together:
//
//	err := retry.Do(ctx, func(ctx context.Context) error {
//	    return circuit.Do(ctx, func(ctx context.Context) error {
//	        return client.Call(ctx)
//	    })
//	}, retry.If(func(err error) bool {
//	    return !breaker.IsOpen(err)  // Don't retry if circuit is open
//	}))
package breaker
