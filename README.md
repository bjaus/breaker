# breaker

[![Go Reference](https://pkg.go.dev/badge/github.com/bjaus/breaker.svg)](https://pkg.go.dev/github.com/bjaus/breaker)
[![Go Report Card](https://goreportcard.com/badge/github.com/bjaus/breaker)](https://goreportcard.com/report/github.com/bjaus/breaker)
[![CI](https://github.com/bjaus/breaker/actions/workflows/ci.yml/badge.svg)](https://github.com/bjaus/breaker/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/bjaus/breaker/branch/main/graph/badge.svg)](https://codecov.io/gh/bjaus/breaker)

Circuit breaker pattern for resilient Go services.

## Features

- **Failure Detection** — Trips after consecutive failures
- **Fast Rejection** — Open circuits reject immediately without load
- **Gradual Recovery** — Half-open state tests if service has recovered
- **Lifecycle Hooks** — OnStateChange, OnCall, OnReject for observability
- **Generic Helper** — Type-safe Run[T] for functions with return values
- **Zero Dependencies** — Only the Go standard library

## Installation

```bash
go get github.com/bjaus/breaker
```

Requires Go 1.25 or later.

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/bjaus/breaker"
)

func main() {
    circuit := breaker.New("payment-service")

    err := circuit.Do(context.Background(), func(ctx context.Context) error {
        return chargeCustomer(ctx)
    })
    if breaker.IsOpen(err) {
        log.Println("Circuit open, using fallback")
        return
    }
    if err != nil {
        log.Fatal(err)
    }
}
```

## Usage

### Basic Circuit

```go
circuit := breaker.New("api-gateway",
    breaker.WithFailureThreshold(5),       // Open after 5 consecutive failures
    breaker.WithSuccessThreshold(2),       // Close after 2 consecutive successes
    breaker.WithOpenDuration(30*time.Second),  // Wait before half-open
)

err := circuit.Do(ctx, func(ctx context.Context) error {
    return client.Call(ctx)
})
```

### Generic Helper

For functions that return values:

```go
user, err := breaker.Run(ctx, circuit, func(ctx context.Context) (*User, error) {
    return client.GetUser(ctx, id)
})
```

### Fallback Pattern

```go
func GetUser(ctx context.Context, id string) (*User, error) {
    user, err := breaker.Run(ctx, circuit, func(ctx context.Context) (*User, error) {
        return client.GetUser(ctx, id)
    })
    if breaker.IsOpen(err) {
        return getCachedUser(id)  // Fallback
    }
    return user, err
}
```

### Custom Failure Conditions

```go
// Only count specific errors as failures
circuit := breaker.New("api",
    breaker.If(func(err error) bool {
        return errors.Is(err, ErrTimeout)
    }),
)

// Don't count 404s as failures
circuit := breaker.New("api",
    breaker.IfNot(func(err error) bool {
        return errors.Is(err, ErrNotFound)
    }),
)
```

### Lifecycle Hooks

```go
circuit := breaker.New("service",
    breaker.OnStateChange(func(name string, from, to breaker.State) {
        logger.Info("circuit changed", "from", from, "to", to)
        metrics.Gauge("circuit.state", float64(to))
    }),
    breaker.OnCall(func(name string, state breaker.State, err error) {
        if err != nil {
            metrics.Increment("circuit.failure")
        }
    }),
    breaker.OnReject(func(name string) {
        metrics.Increment("circuit.rejected")
    }),
)
```

### Manual Reset

```go
circuit.Reset()  // Force circuit back to closed
```

## Circuit States

```
     ┌─────────┐
     │  Closed │ ◄──────────────────────┐
     └────┬────┘                        │
          │ failures >= threshold       │ successes >= threshold
          ▼                             │
     ┌─────────┐      timeout      ┌────┴────┐
     │   Open  │ ─────────────────►│HalfOpen │
     └─────────┘                   └────┬────┘
          ▲                             │
          │ failure                     │
          └─────────────────────────────┘
```

| State | Behavior |
|-------|----------|
| **Closed** | Normal operation, requests flow through |
| **Open** | Requests rejected immediately with ErrOpen |
| **HalfOpen** | Limited requests allowed to test recovery |

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `WithFailureThreshold(n)` | 5 | Consecutive failures before opening |
| `WithSuccessThreshold(n)` | 2 | Consecutive successes to close |
| `WithOpenDuration(d)` | 30s | Time before transitioning to half-open |
| `WithHalfOpenRequests(n)` | 1 | Requests allowed in half-open state |
| `If(cond)` | err != nil | Condition for counting as failure |
| `IfNot(cond)` | - | Inverted condition |
| `WithClock(c)` | real time | Clock interface for testing |

## Hooks

| Hook | Called When |
|------|-------------|
| `OnStateChange(fn)` | Circuit transitions between states |
| `OnCall(fn)` | After each call attempt |
| `OnReject(fn)` | When call is rejected (circuit open) |

## Testing

Inject a fake clock to control time:

```go
type fakeClock struct {
    now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestCircuit(t *testing.T) {
    clock := &fakeClock{now: time.Now()}
    circuit := breaker.New("test",
        breaker.WithFailureThreshold(1),
        breaker.WithOpenDuration(30*time.Second),
        breaker.WithClock(clock),
    )

    // Trip the circuit
    _ = circuit.Do(ctx, func(ctx context.Context) error {
        return errors.New("fail")
    })
    assert.Equal(t, breaker.Open, circuit.State())

    // Advance past timeout
    clock.Advance(31 * time.Second)
    assert.Equal(t, breaker.HalfOpen, circuit.State())
}
```

## With Retry

Circuit breaker and retry work well together:

```go
err := retry.Do(ctx, func(ctx context.Context) error {
    return circuit.Do(ctx, func(ctx context.Context) error {
        return client.Call(ctx)
    })
}, retry.If(func(err error) bool {
    return !breaker.IsOpen(err)  // Don't retry if circuit is open
}))
```

## License

MIT License - see [LICENSE](LICENSE) for details.
