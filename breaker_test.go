package breaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bjaus/breaker"
)

var errTest = errors.New("test error")

// fakeClock is a test clock that allows manual time control.
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestNew(t *testing.T) {
	t.Run("creates circuit with defaults", func(t *testing.T) {
		c := breaker.New("test")

		if c.Name() != "test" {
			t.Fatalf("expected name 'test', got %q", c.Name())
		}
		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed state, got %v", c.State())
		}
	})

	t.Run("creates circuit with options", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(3),
			breaker.WithSuccessThreshold(2),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithClock(clock),
		)

		if c.Name() != "test" {
			t.Fatalf("expected name 'test', got %q", c.Name())
		}
	})
}

func TestDo(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		err := c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("returns function error", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))

		err := c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if !errors.Is(err, errTest) {
			t.Fatalf("expected errTest, got %v", err)
		}
	})

	t.Run("counts consecutive failures", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(3),
			breaker.WithClock(newFakeClock()),
		)

		for range 2 {
			_ = c.Do(context.Background(), func(ctx context.Context) error {
				return errTest
			})
		}

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed after 2 failures, got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open after 3 failures, got %v", c.State())
		}
	})

	t.Run("resets failure count on success", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(3),
			breaker.WithClock(newFakeClock()),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		failures, _ := c.Counts()
		if failures != 2 {
			t.Fatalf("expected 2 failures, got %d", failures)
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})

		failures, _ = c.Counts()
		if failures != 0 {
			t.Fatalf("expected 0 failures after success, got %d", failures)
		}
	})

	t.Run("rejects calls when open", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open state, got %v", c.State())
		}

		called := false
		err := c.Do(context.Background(), func(ctx context.Context) error {
			called = true
			return nil
		})

		if called {
			t.Fatal("expected function not to be called when circuit is open")
		}
		if !breaker.IsOpen(err) {
			t.Fatalf("expected ErrOpen, got %v", err)
		}
	})

	t.Run("respects context", func(t *testing.T) {
		c := breaker.New("test", breaker.WithClock(newFakeClock()))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := c.Do(ctx, func(ctx context.Context) error {
			return ctx.Err()
		})

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestStateTransitions(t *testing.T) {
	t.Run("closed to open after failures", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(2),
			breaker.WithClock(newFakeClock()),
		)

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed, got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open, got %v", c.State())
		}
	})

	t.Run("open to half-open after duration", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithOpenDuration(30*time.Second),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open, got %v", c.State())
		}

		clock.Advance(29 * time.Second)
		if c.State() != breaker.Open {
			t.Fatalf("expected Open before duration, got %v", c.State())
		}

		clock.Advance(2 * time.Second)
		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen after duration, got %v", c.State())
		}
	})

	t.Run("half-open to closed after successes", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithSuccessThreshold(2),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithHalfOpenRequests(2),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		clock.Advance(11 * time.Second)

		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen, got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})

		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen after 1 success, got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed after 2 successes, got %v", c.State())
		}
	})

	t.Run("half-open to open on failure", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		clock.Advance(11 * time.Second)

		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen, got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open after failure in half-open, got %v", c.State())
		}
	})
}

func TestHalfOpenRequests(t *testing.T) {
	t.Run("limits requests in half-open", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithHalfOpenRequests(1),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		clock.Advance(11 * time.Second)

		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen, got %v", c.State())
		}

		calls := 0
		for range 5 {
			err := c.Do(context.Background(), func(ctx context.Context) error {
				calls++
				return nil
			})
			if calls > 1 && !breaker.IsOpen(err) {
				t.Fatalf("expected ErrOpen for call %d, got %v", calls, err)
			}
		}

		if calls != 1 {
			t.Fatalf("expected 1 call allowed in half-open, got %d", calls)
		}
	})

	t.Run("allows multiple requests when configured", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithHalfOpenRequests(3),
			breaker.WithSuccessThreshold(5),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		clock.Advance(11 * time.Second)

		calls := 0
		rejected := 0
		for range 5 {
			err := c.Do(context.Background(), func(ctx context.Context) error {
				calls++
				return nil
			})
			if breaker.IsOpen(err) {
				rejected++
			}
		}

		if calls != 3 {
			t.Fatalf("expected 3 calls allowed in half-open, got %d", calls)
		}
		if rejected != 2 {
			t.Fatalf("expected 2 rejected, got %d", rejected)
		}
	})
}

func TestCondition(t *testing.T) {
	t.Run("custom condition determines failure", func(t *testing.T) {
		transient := errors.New("transient")
		permanent := errors.New("permanent")

		c := breaker.New("test",
			breaker.WithFailureThreshold(2),
			breaker.WithClock(newFakeClock()),
			breaker.If(func(err error) bool {
				return errors.Is(err, transient)
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return permanent
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return permanent
		})

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed (permanent errors not counted), got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return transient
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return transient
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open after transient errors, got %v", c.State())
		}
	})

	t.Run("IfNot skips matching errors", func(t *testing.T) {
		skipThis := errors.New("skip this")
		countThis := errors.New("count this")

		c := breaker.New("test",
			breaker.WithFailureThreshold(2),
			breaker.WithClock(newFakeClock()),
			breaker.IfNot(func(err error) bool {
				return errors.Is(err, skipThis)
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return skipThis
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return skipThis
		})

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed (skipThis errors NOT counted), got %v", c.State())
		}

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return countThis
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return countThis
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open after countThis errors, got %v", c.State())
		}
	})

	t.Run("Not inverts condition", func(t *testing.T) {
		alwaysTrue := func(err error) bool { return true }
		alwaysFalse := func(err error) bool { return false }

		inverted := breaker.Not(alwaysTrue)
		if inverted(errTest) {
			t.Fatal("expected Not(alwaysTrue) to return false")
		}

		inverted = breaker.Not(alwaysFalse)
		if !inverted(errTest) {
			t.Fatal("expected Not(alwaysFalse) to return true")
		}
	})
}

func TestHooks(t *testing.T) {
	t.Run("OnStateChange called on transition", func(t *testing.T) {
		var transitions []struct {
			name     string
			from, to breaker.State
		}

		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
			breaker.OnStateChange(func(name string, from, to breaker.State) {
				transitions = append(transitions, struct {
					name     string
					from, to breaker.State
				}{name, from, to})
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if len(transitions) != 1 {
			t.Fatalf("expected 1 transition, got %d", len(transitions))
		}
		if transitions[0].name != "test" {
			t.Fatalf("expected name 'test', got %q", transitions[0].name)
		}
		if transitions[0].from != breaker.Closed {
			t.Fatalf("expected from Closed, got %v", transitions[0].from)
		}
		if transitions[0].to != breaker.Open {
			t.Fatalf("expected to Open, got %v", transitions[0].to)
		}
	})

	t.Run("OnCall called after each attempt", func(t *testing.T) {
		var calls []struct {
			name  string
			state breaker.State
			err   error
		}

		c := breaker.New("test",
			breaker.WithClock(newFakeClock()),
			breaker.OnCall(func(name string, state breaker.State, err error) {
				calls = append(calls, struct {
					name  string
					state breaker.State
					err   error
				}{name, state, err})
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if len(calls) != 2 {
			t.Fatalf("expected 2 calls, got %d", len(calls))
		}
		if calls[0].err != nil {
			t.Fatalf("expected first call err nil, got %v", calls[0].err)
		}
		if !errors.Is(calls[1].err, errTest) {
			t.Fatalf("expected second call err errTest, got %v", calls[1].err)
		}
	})

	t.Run("OnReject called when circuit open", func(t *testing.T) {
		var rejects []string

		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
			breaker.OnReject(func(name string) {
				rejects = append(rejects, name)
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})

		if len(rejects) != 2 {
			t.Fatalf("expected 2 rejects, got %d", len(rejects))
		}
		if rejects[0] != "test" || rejects[1] != "test" {
			t.Fatalf("expected rejects for 'test', got %v", rejects)
		}
	})
}

func TestReset(t *testing.T) {
	t.Run("resets circuit to closed", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open, got %v", c.State())
		}

		c.Reset()

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed after reset, got %v", c.State())
		}

		failures, successes := c.Counts()
		if failures != 0 || successes != 0 {
			t.Fatalf("expected counts reset to 0, got failures=%d successes=%d", failures, successes)
		}
	})

	t.Run("reset triggers OnStateChange", func(t *testing.T) {
		var transitions []breaker.State

		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithClock(newFakeClock()),
			breaker.OnStateChange(func(name string, from, to breaker.State) {
				transitions = append(transitions, to)
			}),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		c.Reset()

		if len(transitions) != 2 {
			t.Fatalf("expected 2 transitions, got %d", len(transitions))
		}
		if transitions[1] != breaker.Closed {
			t.Fatalf("expected second transition to Closed, got %v", transitions[1])
		}
	})
}

func TestIsOpen(t *testing.T) {
	t.Run("returns true for ErrOpen", func(t *testing.T) {
		if !breaker.IsOpen(breaker.ErrOpen) {
			t.Fatal("expected IsOpen(ErrOpen) to be true")
		}
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		if breaker.IsOpen(errTest) {
			t.Fatal("expected IsOpen(errTest) to be false")
		}
	})

	t.Run("returns false for nil", func(t *testing.T) {
		if breaker.IsOpen(nil) {
			t.Fatal("expected IsOpen(nil) to be false")
		}
	})
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state breaker.State
		want  string
	}{
		{breaker.Closed, "closed"},
		{breaker.Open, "open"},
		{breaker.HalfOpen, "half-open"},
		{breaker.State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestCounts(t *testing.T) {
	t.Run("tracks failures", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(10),
			breaker.WithClock(newFakeClock()),
		)

		for range 3 {
			_ = c.Do(context.Background(), func(ctx context.Context) error {
				return errTest
			})
		}

		failures, successes := c.Counts()
		if failures != 3 {
			t.Fatalf("expected 3 failures, got %d", failures)
		}
		if successes != 0 {
			t.Fatalf("expected 0 successes, got %d", successes)
		}
	})

	t.Run("tracks successes in half-open", func(t *testing.T) {
		clock := newFakeClock()
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithSuccessThreshold(5),
			breaker.WithHalfOpenRequests(5),
			breaker.WithOpenDuration(10*time.Second),
			breaker.WithClock(clock),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})
		clock.Advance(11 * time.Second)

		for range 3 {
			_ = c.Do(context.Background(), func(ctx context.Context) error {
				return nil
			})
		}

		_, successes := c.Counts()
		if successes != 3 {
			t.Fatalf("expected 3 successes, got %d", successes)
		}
	})
}

func TestResetWhenAlreadyClosed(t *testing.T) {
	t.Run("reset when already closed is no-op", func(t *testing.T) {
		stateChanges := 0
		c := breaker.New("test",
			breaker.WithClock(newFakeClock()),
			breaker.OnStateChange(func(name string, from, to breaker.State) {
				stateChanges++
			}),
		)

		if c.State() != breaker.Closed {
			t.Fatalf("expected Closed, got %v", c.State())
		}

		c.Reset()

		if stateChanges != 0 {
			t.Fatalf("expected no state changes, got %d", stateChanges)
		}
	})
}

func TestRealClock(t *testing.T) {
	t.Run("uses real clock when not injected", func(t *testing.T) {
		c := breaker.New("test",
			breaker.WithFailureThreshold(1),
			breaker.WithOpenDuration(50*time.Millisecond),
		)

		_ = c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		})

		if c.State() != breaker.Open {
			t.Fatalf("expected Open, got %v", c.State())
		}

		time.Sleep(60 * time.Millisecond)

		if c.State() != breaker.HalfOpen {
			t.Fatalf("expected HalfOpen after wait, got %v", c.State())
		}
	})
}
