package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bjaus/breaker"
)

// ExampleNew demonstrates creating a circuit breaker with default settings.
func ExampleNew() {
	circuit := breaker.New("my-service")

	err := circuit.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	fmt.Println("Error:", err)
	fmt.Println("State:", circuit.State())

	// Output:
	// Error: <nil>
	// State: closed
}

// ExampleNew_withOptions demonstrates creating a circuit breaker with custom settings.
func ExampleNew_withOptions() {
	circuit := breaker.New("payment-service",
		breaker.WithFailureThreshold(3),
		breaker.WithSuccessThreshold(2),
		breaker.WithOpenDuration(30*time.Second),
	)

	fmt.Println("Name:", circuit.Name())
	fmt.Println("State:", circuit.State())

	// Output:
	// Name: payment-service
	// State: closed
}

// ExampleCircuit_Do demonstrates basic circuit breaker usage.
func ExampleCircuit_Do() {
	circuit := breaker.New("api",
		breaker.WithFailureThreshold(2),
	)

	attempts := 0
	for range 5 {
		err := circuit.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return errors.New("service unavailable")
		})
		if breaker.IsOpen(err) {
			fmt.Println("Circuit is open, skipping call")
		}
	}

	fmt.Println("Attempts:", attempts)
	fmt.Println("State:", circuit.State())

	// Output:
	// Circuit is open, skipping call
	// Circuit is open, skipping call
	// Circuit is open, skipping call
	// Attempts: 2
	// State: open
}

// ExampleRun demonstrates the generic helper for returning values.
func ExampleRun() {
	circuit := breaker.New("user-service")

	user, err := breaker.Run(context.Background(), circuit, func(ctx context.Context) (string, error) {
		return "john_doe", nil
	})

	fmt.Println("User:", user)
	fmt.Println("Error:", err)

	// Output:
	// User: john_doe
	// Error: <nil>
}

// ExampleIsOpen demonstrates checking if an error is due to an open circuit.
func ExampleIsOpen() {
	circuit := breaker.New("service",
		breaker.WithFailureThreshold(1),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	err := circuit.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if breaker.IsOpen(err) {
		fmt.Println("Circuit is open, using fallback")
	}

	// Output:
	// Circuit is open, using fallback
}

// ExampleCircuit_Reset demonstrates manually resetting a circuit.
func ExampleCircuit_Reset() {
	circuit := breaker.New("service",
		breaker.WithFailureThreshold(1),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	fmt.Println("Before reset:", circuit.State())

	circuit.Reset()

	fmt.Println("After reset:", circuit.State())

	// Output:
	// Before reset: open
	// After reset: closed
}

// ExampleIf demonstrates custom failure conditions.
func ExampleIf() {
	transient := errors.New("transient error")

	circuit := breaker.New("api",
		breaker.WithFailureThreshold(2),
		breaker.If(func(err error) bool {
			return errors.Is(err, transient)
		}),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("permanent error")
	})
	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("permanent error")
	})

	fmt.Println("After permanent errors:", circuit.State())

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return transient
	})
	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return transient
	})

	fmt.Println("After transient errors:", circuit.State())

	// Output:
	// After permanent errors: closed
	// After transient errors: open
}

// ExampleOnStateChange demonstrates the state change hook.
func ExampleOnStateChange() {
	circuit := breaker.New("service",
		breaker.WithFailureThreshold(1),
		breaker.OnStateChange(func(name string, from, to breaker.State) {
			fmt.Printf("Circuit %s: %s -> %s\n", name, from, to)
		}),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Output:
	// Circuit service: closed -> open
}

// ExampleOnCall demonstrates the call hook for metrics.
func ExampleOnCall() {
	successCount := 0
	failureCount := 0

	circuit := breaker.New("service",
		breaker.OnCall(func(name string, state breaker.State, err error) {
			if err != nil {
				failureCount++
			} else {
				successCount++
			}
		}),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})
	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})
	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	fmt.Println("Successes:", successCount)
	fmt.Println("Failures:", failureCount)

	// Output:
	// Successes: 2
	// Failures: 1
}

// ExampleOnReject demonstrates the reject hook.
func ExampleOnReject() {
	rejectCount := 0

	circuit := breaker.New("service",
		breaker.WithFailureThreshold(1),
		breaker.OnReject(func(name string) {
			rejectCount++
		}),
	)

	_ = circuit.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	for range 3 {
		_ = circuit.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}

	fmt.Println("Rejected:", rejectCount)

	// Output:
	// Rejected: 3
}

// Example_fallback demonstrates graceful degradation when circuit is open.
func Example_fallback() {
	circuit := breaker.New("user-service",
		breaker.WithFailureThreshold(1),
	)

	getUser := func(ctx context.Context, _ int) (string, error) {
		user, err := breaker.Run(ctx, circuit, func(ctx context.Context) (string, error) {
			return "", errors.New("service unavailable")
		})
		if breaker.IsOpen(err) {
			return "guest", nil
		}
		if err != nil {
			return "", err
		}
		return user, nil
	}

	_, err1 := getUser(context.Background(), 1)
	user2, _ := getUser(context.Background(), 2)

	fmt.Println("User 1 error:", err1 != nil)
	fmt.Println("User 2:", user2)

	// Output:
	// User 1 error: true
	// User 2: guest
}

// ExampleState_String demonstrates state string representation.
func ExampleState_String() {
	fmt.Println(breaker.Closed.String())
	fmt.Println(breaker.Open.String())
	fmt.Println(breaker.HalfOpen.String())

	// Output:
	// closed
	// open
	// half-open
}
