package breaker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bjaus/breaker"
)

type testResult struct {
	value string
}

func TestRun(t *testing.T) {
	t.Run("returns value on success", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
			return &testResult{value: "hello"}, nil
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if result.value != "hello" {
			t.Fatalf("expected 'hello', got %q", result.value)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
			return nil, errTest
		})

		if !errors.Is(err, errTest) {
			t.Fatalf("expected errTest, got %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil result, got %v", result)
		}
	})

	t.Run("returns ErrOpen when circuit open", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
		)

		_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
			return nil, errTest
		})

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
			return &testResult{value: "should not reach"}, nil
		})

		if !breaker.IsOpen(err) {
			t.Fatalf("expected ErrOpen, got %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil result, got %v", result)
		}
	})

	t.Run("works with value types", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
			return 42, nil
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if result != 42 {
			t.Fatalf("expected 42, got %d", result)
		}
	})

	t.Run("returns zero value on error", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
			return 0, errTest
		})

		if !errors.Is(err, errTest) {
			t.Fatalf("expected errTest, got %v", err)
		}
		if result != 0 {
			t.Fatalf("expected 0, got %d", result)
		}
	})

	t.Run("works with slices", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		result, err := breaker.Run(ctx(), c, func(ctx context.Context) ([]string, error) {
			return []string{"a", "b", "c"}, nil
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}
	})

	t.Run("counts failures from Run", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(2),
			breaker.WithClock(newFakeClock()),
		)

		_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
			return 0, errTest
		})
		_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
			return 0, errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open after 2 failures, got %v", c.State())
		}
	})
}

func ctx() context.Context {
	return context.Background()
}
