package breaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	// Closed is the normal operating state. Requests flow through.
	Closed State = iota

	// Open is the tripped state. Requests are rejected immediately.
	Open

	// HalfOpen is the recovery testing state. Limited requests are allowed.
	HalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Func is the function signature for protected operations.
type Func func(ctx context.Context) error

// Condition determines whether an error should count as a failure.
type Condition func(error) bool

// OnStateChangeFunc is called when the circuit changes state.
type OnStateChangeFunc func(name string, from, to State)

// OnCallFunc is called after each call attempt.
type OnCallFunc func(name string, state State, err error)

// OnRejectFunc is called when a call is rejected due to open circuit.
type OnRejectFunc func(name string)

// ErrOpen is returned when the circuit is open and rejecting requests.
var ErrOpen = errors.New("circuit open")

// IsOpen reports whether err is because the circuit is open.
func IsOpen(err error) bool {
	return errors.Is(err, ErrOpen)
}

// Default values.
const (
	DefaultFailureThreshold = 5
	DefaultSuccessThreshold = 2
	DefaultOpenDuration     = 30 * time.Second
	DefaultHalfOpenRequests = 1
)

// Circuit is a circuit breaker. Safe for concurrent use.
type Circuit struct {
	name string
	cfg  config

	mu          sync.Mutex
	state       State
	failures    int
	successes   int
	halfOpenCnt int
	openedAt    time.Time
}

// New creates a Circuit with the given options.
func New(name string, opts ...Option) *Circuit {
	cfg := config{
		failureThreshold: DefaultFailureThreshold,
		successThreshold: DefaultSuccessThreshold,
		openDuration:     DefaultOpenDuration,
		halfOpenRequests: DefaultHalfOpenRequests,
		condition:        defaultCondition,
		clock:            realClock{},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Circuit{
		name:  name,
		cfg:   cfg,
		state: Closed,
	}
}

// Do executes fn with circuit breaker protection.
func (c *Circuit) Do(ctx context.Context, fn Func) error {
	state, err := c.allow()
	if err != nil {
		if c.cfg.onReject != nil {
			c.cfg.onReject(c.name)
		}
		return err
	}

	fnErr := fn(ctx)

	c.record(fnErr)

	if c.cfg.onCall != nil {
		c.cfg.onCall(c.name, state, fnErr)
	}

	return fnErr
}

// State returns the current state.
func (c *Circuit) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentState()
}

// Reset manually resets the circuit to closed state.
func (c *Circuit) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setState(Closed)
}

// Name returns the circuit name.
func (c *Circuit) Name() string {
	return c.name
}

// Counts returns the current failure and success counts.
func (c *Circuit) Counts() (failures, successes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failures, c.successes
}

func (c *Circuit) allow() (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.currentState()
	switch state {
	case Open:
		return state, ErrOpen
	case HalfOpen:
		if c.halfOpenCnt >= c.cfg.halfOpenRequests {
			return state, ErrOpen
		}
		c.halfOpenCnt++
	}
	return state, nil
}

func (c *Circuit) record(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	isFailure := c.cfg.condition(err)

	switch c.currentState() {
	case Closed:
		if isFailure {
			c.failures++
			if c.failures >= c.cfg.failureThreshold {
				c.setState(Open)
			}
		} else {
			c.failures = 0
		}

	case HalfOpen:
		if isFailure {
			c.setState(Open)
		} else {
			c.successes++
			if c.successes >= c.cfg.successThreshold {
				c.setState(Closed)
			}
		}
	}
}

func (c *Circuit) currentState() State {
	if c.state == Open && c.cfg.clock.Now().Sub(c.openedAt) >= c.cfg.openDuration {
		c.setState(HalfOpen)
	}
	return c.state
}

func (c *Circuit) setState(to State) {
	if c.state == to {
		return
	}
	from := c.state
	c.state = to

	c.failures = 0
	c.successes = 0
	c.halfOpenCnt = 0

	if to == Open {
		c.openedAt = c.cfg.clock.Now()
	}

	if c.cfg.onStateChange != nil {
		c.cfg.onStateChange(c.name, from, to)
	}
}

func defaultCondition(err error) bool {
	return err != nil
}
