package breaker

import (
	"context"
	"errors"
	"testing"
)

func BenchmarkCircuit_Do_Success(b *testing.B) {
	ctx := context.Background()
	circuit := New("bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		circuit.Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkCircuit_Do_Failure(b *testing.B) {
	ctx := context.Background()
	errTest := errors.New("test error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		circuit := New("bench", WithFailureThreshold(b.N+1))
		circuit.Do(ctx, func(ctx context.Context) error {
			return errTest
		})
	}
}

func BenchmarkCircuit_Do_Open(b *testing.B) {
	ctx := context.Background()
	circuit := New("bench", WithFailureThreshold(1))

	circuit.Do(ctx, func(ctx context.Context) error {
		return errors.New("trip")
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		circuit.Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkCircuit_Do_Parallel(b *testing.B) {
	ctx := context.Background()
	circuit := New("bench")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			circuit.Do(ctx, func(ctx context.Context) error {
				return nil
			})
		}
	})
}

func BenchmarkCircuit_State(b *testing.B) {
	circuit := New("bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		circuit.State()
	}
}
