package shutdown

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aisgo/ais-pkg/logger"
)

func TestShutdownHookTimeout(t *testing.T) {
	m := NewManager(ManagerParams{
		Logger: logger.NewNop(),
		Config: &Config{
			Timeout:     time.Second,
			HookTimeout: 50 * time.Millisecond,
		},
	})

	var fastCalled atomic.Bool

	m.RegisterHookWithPriority("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, PriorityNormal)
	m.RegisterHookWithPriority("fast", func(ctx context.Context) error {
		fastCalled.Store(true)
		return nil
	}, PriorityNormal)

	start := time.Now()
	m.Shutdown(context.Background())
	elapsed := time.Since(start)

	if !fastCalled.Load() {
		t.Fatalf("fast hook not executed")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown took too long: %v", elapsed)
	}
}

func TestShutdownReturnsWhenHookIgnoresContext(t *testing.T) {
	m := NewManager(ManagerParams{
		Logger: logger.NewNop(),
		Config: &Config{
			Timeout:     80 * time.Millisecond,
			HookTimeout: 20 * time.Millisecond,
		},
	})

	blocked := make(chan struct{})
	hookDone := make(chan struct{})
	var fastCalled atomic.Bool

	m.RegisterHookWithPriority("stuck", func(context.Context) error {
		defer close(hookDone)
		<-blocked
		return nil
	}, PriorityNormal)
	m.RegisterHookWithPriority("fast", func(context.Context) error {
		fastCalled.Store(true)
		return nil
	}, PriorityNormal)

	start := time.Now()
	m.Shutdown(context.Background())
	elapsed := time.Since(start)

	if !fastCalled.Load() {
		t.Fatalf("fast hook not executed")
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("shutdown should return after timeout even with stuck hook, got %v", elapsed)
	}

	close(blocked)

	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatalf("stuck hook goroutine did not exit after being unblocked")
	}
}
