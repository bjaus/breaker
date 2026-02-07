package breaker

import "context"

// Run executes fn and returns its result with circuit breaker protection.
// This is a convenience wrapper for functions that return a value.
func Run[T any](ctx context.Context, c *Circuit, fn func(context.Context) (T, error)) (T, error) {
	var result T
	err := c.Do(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}
