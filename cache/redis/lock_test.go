package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLockAcquireRelease(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	lock := client.NewLock("resource", LockOption{TTL: 200 * time.Millisecond, RetryTimes: 1, RetryDelay: 10 * time.Millisecond})
	if err := lock.Acquire(ctx); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	lock2 := client.NewLock("resource", LockOption{TTL: 200 * time.Millisecond, RetryTimes: 1, RetryDelay: 10 * time.Millisecond})
	if err := lock2.Acquire(ctx); !errors.Is(err, ErrLockFailed) {
		t.Fatalf("expected ErrLockFailed, got: %v", err)
	}

	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	if err := lock2.Acquire(ctx); err != nil {
		t.Fatalf("acquire lock after release: %v", err)
	}
}

func TestLockAutoExtendIgnoresParentCancel(t *testing.T) {
	client := newTestClient(t)
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lock := client.NewLock("auto", LockOption{TTL: 120 * time.Millisecond, RetryTimes: 1, AutoExtend: true, ExtendFactor: 0.5, IgnoreParentCancel: true})
	if err := lock.Acquire(parentCtx); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	cancel()
	// 等待超过 TTL，若续期生效则锁仍存在
	time.Sleep(300 * time.Millisecond)

	exists, err := client.Exists(context.Background(), lock.key)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists == 0 {
		t.Fatalf("expected lock to be extended and still exist")
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("release lock: %v", err)
	}
}

func TestLockConcurrentAcquireDoesNotStopAutoExtend(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	opt := LockOption{
		TTL:          200 * time.Millisecond,
		RetryTimes:   1,
		RetryDelay:   5 * time.Millisecond,
		AutoExtend:   true,
		ExtendFactor: 0.5,
	}

	lock := client.NewLock("concurrent-acquire", opt)
	if err := lock.AcquireWithOption(ctx, opt); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	deadline := time.Now().Add(150 * time.Millisecond)
	for {
		lock.mu.Lock()
		cancelSet := lock.extendCancel != nil
		lock.mu.Unlock()
		if cancelSet {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("auto extend did not start in time")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := lock.AcquireWithOption(ctx, opt)
			if err != nil && !errors.Is(err, ErrLockFailed) {
				t.Errorf("unexpected acquire error: %v", err)
			}
		}()
	}
	wg.Wait()

	lock.mu.Lock()
	cancelSet := lock.extendCancel != nil
	lock.mu.Unlock()
	if !cancelSet {
		t.Fatalf("expected auto extend to remain active after concurrent acquire")
	}

	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release lock: %v", err)
	}
}

func TestAcquireWithOptionResetsAcquiredOnReacquireFailure(t *testing.T) {
	client, server := newTestClientWithServer(t)
	ctx := context.Background()
	opt := LockOption{
		TTL:          50 * time.Millisecond,
		RetryTimes:   1,
		RetryDelay:   5 * time.Millisecond,
		AutoExtend:   false,
		ExtendFactor: 0.5,
	}

	lock := client.NewLock("stale-acquired", opt)
	if err := lock.AcquireWithOption(ctx, opt); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	server.FastForward(opt.TTL + 10*time.Millisecond)

	exists, err := client.Exists(ctx, lock.key)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists != 0 {
		t.Fatalf("expected original lock key to expire")
	}

	other := client.NewLock("stale-acquired", opt)
	if err := other.AcquireWithOption(ctx, opt); err != nil {
		t.Fatalf("other acquire lock: %v", err)
	}

	err = lock.AcquireWithOption(ctx, opt)
	if !errors.Is(err, ErrLockFailed) {
		t.Fatalf("expected ErrLockFailed, got: %v", err)
	}

	lock.mu.Lock()
	acquired := lock.acquired
	lock.mu.Unlock()
	if acquired {
		t.Fatalf("expected acquired flag to be reset after failed reacquire")
	}
}
