package breaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bjaus/breaker"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

type BreakerSuite struct {
	suite.Suite
	clock *fakeClock
}

func TestBreakerSuite(t *testing.T) {
	suite.Run(t, new(BreakerSuite))
}

func (s *BreakerSuite) SetupTest() {
	s.clock = newFakeClock()
}

func (s *BreakerSuite) TestNew_CreatesCircuitWithDefaults() {
	c := breaker.New("test")

	s.Equal("test", c.Name())
	s.Equal(breaker.Closed, c.State())
}

func (s *BreakerSuite) TestNew_CreatesCircuitWithOptions() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(3),
		breaker.WithSuccessThreshold(2),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithClock(s.clock),
	)

	s.Equal("test", c.Name())
}

func (s *BreakerSuite) TestDo_SucceedsOnFirstAttempt() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	err := c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	s.NoError(err)
}

func (s *BreakerSuite) TestDo_ReturnsFunctionError() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	err := c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	})

	s.ErrorIs(err, errTest)
}

func (s *BreakerSuite) TestDo_CountsConsecutiveFailures() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(3),
		breaker.WithClock(s.clock),
	)

	for range 2 {
		s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		}), errTest)
	}

	s.Equal(breaker.Closed, c.State(), "expected Closed after 2 failures")

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State(), "expected Open after 3 failures")
}

func (s *BreakerSuite) TestDo_ResetsFailureCountOnSuccess() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(3),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	failures, _ := c.Counts()
	s.Equal(2, failures)

	s.NoError(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	}))

	failures, _ = c.Counts()
	s.Equal(0, failures, "expected 0 failures after success")
}

func (s *BreakerSuite) TestDo_RejectsCallsWhenOpen() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State())

	called := false
	err := c.Do(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})

	s.False(called, "expected function not to be called when circuit is open")
	s.True(breaker.IsOpen(err))
}

func (s *BreakerSuite) TestDo_RespectsContext() {
	c := breaker.New("test", breaker.WithClock(s.clock))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Do(ctx, func(ctx context.Context) error {
		return ctx.Err()
	})

	s.ErrorIs(err, context.Canceled)
}

func (s *BreakerSuite) TestStateTransitions_ClosedToOpenAfterFailures() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(2),
		breaker.WithClock(s.clock),
	)

	s.Equal(breaker.Closed, c.State())

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State())
}

func (s *BreakerSuite) TestStateTransitions_OpenToHalfOpenAfterDuration() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithOpenDuration(30*time.Second),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State())

	s.clock.Advance(29 * time.Second)
	s.Equal(breaker.Open, c.State(), "expected Open before duration")

	s.clock.Advance(2 * time.Second)
	s.Equal(breaker.HalfOpen, c.State(), "expected HalfOpen after duration")
}

func (s *BreakerSuite) TestStateTransitions_HalfOpenToClosedAfterSuccesses() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithSuccessThreshold(2),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithHalfOpenRequests(2),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.clock.Advance(11 * time.Second)

	s.Equal(breaker.HalfOpen, c.State())

	s.NoError(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	}))

	s.Equal(breaker.HalfOpen, c.State(), "expected HalfOpen after 1 success")

	s.NoError(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	}))

	s.Equal(breaker.Closed, c.State(), "expected Closed after 2 successes")
}

func (s *BreakerSuite) TestStateTransitions_HalfOpenToOpenOnFailure() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.clock.Advance(11 * time.Second)

	s.Equal(breaker.HalfOpen, c.State())

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State(), "expected Open after failure in half-open")
}

func (s *BreakerSuite) TestHalfOpenRequests_LimitsRequestsInHalfOpen() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithHalfOpenRequests(1),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.clock.Advance(11 * time.Second)

	s.Equal(breaker.HalfOpen, c.State())

	calls := 0
	for range 5 {
		err := c.Do(context.Background(), func(ctx context.Context) error {
			calls++
			return nil
		})
		if calls > 1 {
			s.True(breaker.IsOpen(err), "expected ErrOpen for call %d", calls)
		}
	}

	s.Equal(1, calls, "expected 1 call allowed in half-open")
}

func (s *BreakerSuite) TestHalfOpenRequests_AllowsMultipleRequestsWhenConfigured() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithHalfOpenRequests(3),
		breaker.WithSuccessThreshold(5),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.clock.Advance(11 * time.Second)

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

	s.Equal(3, calls, "expected 3 calls allowed in half-open")
	s.Equal(2, rejected, "expected 2 rejected")
}

func (s *BreakerSuite) TestCondition_CustomConditionDeterminesFailure() {
	transient := errors.New("transient")
	permanent := errors.New("permanent")

	c := breaker.New("test",
		breaker.WithFailureThreshold(2),
		breaker.WithClock(s.clock),
		breaker.If(func(err error) bool {
			return errors.Is(err, transient)
		}),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return permanent
	}), permanent)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return permanent
	}), permanent)

	s.Equal(breaker.Closed, c.State(), "expected Closed (permanent errors not counted)")

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return transient
	}), transient)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return transient
	}), transient)

	s.Equal(breaker.Open, c.State(), "expected Open after transient errors")
}

func (s *BreakerSuite) TestCondition_IfNotSkipsMatchingErrors() {
	skipThis := errors.New("skip this")
	countThis := errors.New("count this")

	c := breaker.New("test",
		breaker.WithFailureThreshold(2),
		breaker.WithClock(s.clock),
		breaker.IfNot(func(err error) bool {
			return errors.Is(err, skipThis)
		}),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return skipThis
	}), skipThis)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return skipThis
	}), skipThis)

	s.Equal(breaker.Closed, c.State(), "expected Closed (skipThis errors NOT counted)")

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return countThis
	}), countThis)
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return countThis
	}), countThis)

	s.Equal(breaker.Open, c.State(), "expected Open after countThis errors")
}

func (s *BreakerSuite) TestCondition_NotInvertsCondition() {
	alwaysTrue := func(err error) bool { return true }
	alwaysFalse := func(err error) bool { return false }

	inverted := breaker.Not(alwaysTrue)
	s.False(inverted(errTest), "expected Not(alwaysTrue) to return false")

	inverted = breaker.Not(alwaysFalse)
	s.True(inverted(errTest), "expected Not(alwaysFalse) to return true")
}

func (s *BreakerSuite) TestHooks_OnStateChangeCalledOnTransition() {
	var transitions []struct {
		name     string
		from, to breaker.State
	}

	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
		breaker.OnStateChange(func(name string, from, to breaker.State) {
			transitions = append(transitions, struct {
				name     string
				from, to breaker.State
			}{name, from, to})
		}),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Require().Len(transitions, 1)
	s.Equal("test", transitions[0].name)
	s.Equal(breaker.Closed, transitions[0].from)
	s.Equal(breaker.Open, transitions[0].to)
}

func (s *BreakerSuite) TestHooks_OnCallCalledAfterEachAttempt() {
	var calls []struct {
		name  string
		state breaker.State
		err   error
	}

	c := breaker.New("test",
		breaker.WithClock(s.clock),
		breaker.OnCall(func(name string, state breaker.State, err error) {
			calls = append(calls, struct {
				name  string
				state breaker.State
				err   error
			}{name, state, err})
		}),
	)

	s.NoError(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	}))
	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Require().Len(calls, 2)
	s.NoError(calls[0].err)
	s.ErrorIs(calls[1].err, errTest)
}

func (s *BreakerSuite) TestHooks_OnRejectCalledWhenCircuitOpen() {
	var rejects []string

	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
		breaker.OnReject(func(name string) {
			rejects = append(rejects, name)
		}),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.True(breaker.IsOpen(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})))
	s.True(breaker.IsOpen(c.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})))

	s.Require().Len(rejects, 2)
	s.Equal("test", rejects[0])
	s.Equal("test", rejects[1])
}

func (s *BreakerSuite) TestReset_ResetsCircuitToClosed() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	s.Equal(breaker.Open, c.State())

	c.Reset()

	s.Equal(breaker.Closed, c.State())

	failures, successes := c.Counts()
	s.Zero(failures)
	s.Zero(successes)
}

func (s *BreakerSuite) TestReset_TriggersOnStateChange() {
	var transitions []breaker.State

	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
		breaker.OnStateChange(func(name string, from, to breaker.State) {
			transitions = append(transitions, to)
		}),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	c.Reset()

	s.Require().Len(transitions, 2)
	s.Equal(breaker.Closed, transitions[1])
}

func (s *BreakerSuite) TestReset_WhenAlreadyClosedIsNoOp() {
	stateChanges := 0
	c := breaker.New("test",
		breaker.WithClock(s.clock),
		breaker.OnStateChange(func(name string, from, to breaker.State) {
			stateChanges++
		}),
	)

	s.Equal(breaker.Closed, c.State())

	c.Reset()

	s.Zero(stateChanges)
}

func (s *BreakerSuite) TestCounts_TracksFailures() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(10),
		breaker.WithClock(s.clock),
	)

	for range 3 {
		s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
			return errTest
		}), errTest)
	}

	failures, successes := c.Counts()
	s.Equal(3, failures)
	s.Zero(successes)
}

func (s *BreakerSuite) TestCounts_TracksSuccessesInHalfOpen() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithSuccessThreshold(5),
		breaker.WithHalfOpenRequests(5),
		breaker.WithOpenDuration(10*time.Second),
		breaker.WithClock(s.clock),
	)

	s.ErrorIs(c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)
	s.clock.Advance(11 * time.Second)

	for range 3 {
		s.NoError(c.Do(context.Background(), func(ctx context.Context) error {
			return nil
		}))
	}

	_, successes := c.Counts()
	s.Equal(3, successes)
}

func TestIsOpen(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"returns true for ErrOpen":      {err: breaker.ErrOpen, want: true},
		"returns false for other error": {err: errTest, want: false},
		"returns false for nil":         {err: nil, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, breaker.IsOpen(tc.err))
		})
	}
}

func TestState_String(t *testing.T) {
	tests := map[string]struct {
		state breaker.State
		want  string
	}{
		"closed":    {state: breaker.Closed, want: "closed"},
		"open":      {state: breaker.Open, want: "open"},
		"half-open": {state: breaker.HalfOpen, want: "half-open"},
		"unknown":   {state: breaker.State(99), want: "unknown"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.state.String())
		})
	}
}

func TestRealClock(t *testing.T) {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithOpenDuration(50*time.Millisecond),
	)

	require.ErrorIs(t, c.Do(context.Background(), func(ctx context.Context) error {
		return errTest
	}), errTest)

	require.Equal(t, breaker.Open, c.State())

	time.Sleep(60 * time.Millisecond)

	require.Equal(t, breaker.HalfOpen, c.State())
}
