package breaker_test

import (
	"context"
	"testing"

	"github.com/bjaus/breaker"
	"github.com/stretchr/testify/suite"
)

type testResult struct {
	value string
}

type RunSuite struct {
	suite.Suite
	clock *fakeClock
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(RunSuite))
}

func (s *RunSuite) SetupTest() {
	s.clock = newFakeClock()
}

func (s *RunSuite) TestRun_ReturnsValueOnSuccess() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
		return &testResult{value: "hello"}, nil
	})

	s.Require().NoError(err)
	s.Equal("hello", result.value)
}

func (s *RunSuite) TestRun_ReturnsErrorOnFailure() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
		return nil, errTest
	})

	s.Require().ErrorIs(err, errTest)
	s.Nil(result)
}

func (s *RunSuite) TestRun_ReturnsErrOpenWhenCircuitOpen() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(1),
		breaker.WithClock(s.clock),
	)

	_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
		return nil, errTest
	})

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) (*testResult, error) {
		return &testResult{value: "should not reach"}, nil
	})

	s.True(breaker.IsOpen(err))
	s.Nil(result)
}

func (s *RunSuite) TestRun_WorksWithValueTypes() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
		return 42, nil
	})

	s.Require().NoError(err)
	s.Equal(42, result)
}

func (s *RunSuite) TestRun_ReturnsZeroValueOnError() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
		return 0, errTest
	})

	s.Require().ErrorIs(err, errTest)
	s.Zero(result)
}

func (s *RunSuite) TestRun_WorksWithSlices() {
	c := breaker.New("test", breaker.WithClock(s.clock))

	result, err := breaker.Run(ctx(), c, func(ctx context.Context) ([]string, error) {
		return []string{"a", "b", "c"}, nil
	})

	s.Require().NoError(err)
	s.Len(result, 3)
}

func (s *RunSuite) TestRun_CountsFailures() {
	c := breaker.New("test",
		breaker.WithFailureThreshold(2),
		breaker.WithClock(s.clock),
	)

	_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
		return 0, errTest
	})
	_, _ = breaker.Run(ctx(), c, func(ctx context.Context) (int, error) {
		return 0, errTest
	})

	s.Equal(breaker.Open, c.State())
}

func ctx() context.Context {
	return context.Background()
}
