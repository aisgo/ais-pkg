package redis

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aisgo/ais-pkg/logger"

	"go.uber.org/fx"
)

type lifecycleRecorder struct {
	hooks []fx.Hook
}

func (r *lifecycleRecorder) Append(h fx.Hook) {
	r.hooks = append(r.hooks, h)
}

func TestClientCacheOps(t *testing.T) {
	client, server := newTestClientWithServer(t)
	ctx := context.Background()

	if err := client.Set(ctx, "k1", "v1", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	val, err := client.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "v1" {
		t.Fatalf("unexpected value: %s", val)
	}

	exists, err := client.Exists(ctx, "k1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists != 1 {
		t.Fatalf("unexpected exists: %d", exists)
	}

	if err := client.Expire(ctx, "k1", 2*time.Second); err != nil {
		t.Fatalf("expire: %v", err)
	}
	server.FastForward(3 * time.Second)

	exists, err = client.Exists(ctx, "k1")
	if err != nil {
		t.Fatalf("exists after expire: %v", err)
	}
	if exists != 0 {
		t.Fatalf("expected expired key")
	}
}

func TestClientHashOps(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.HSet(ctx, "h1", "f1", "v1", "f2", "v2"); err != nil {
		t.Fatalf("hset: %v", err)
	}
	val, err := client.HGet(ctx, "h1", "f1")
	if err != nil {
		t.Fatalf("hget: %v", err)
	}
	if val != "v1" {
		t.Fatalf("unexpected hget value: %s", val)
	}

	all, err := client.HGetAll(ctx, "h1")
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}
	if all["f2"] != "v2" {
		t.Fatalf("unexpected hgetall value: %v", all)
	}

	if err := client.HDel(ctx, "h1", "f1"); err != nil {
		t.Fatalf("hdel: %v", err)
	}
}

func TestOptionalNewClientDefersPingToOnStart(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	lc := &lifecycleRecorder{}
	client := OptionalNewClient(ClientParams{
		Lc: lc,
		Config: Config{
			Host: "127.0.0.1",
			Port: port,
		},
		Logger: logger.NewNop(),
	})
	if client == nil {
		t.Fatalf("expected optional client to be constructed before OnStart")
	}
	if len(lc.hooks) != 1 || lc.hooks[0].OnStart == nil {
		t.Fatalf("expected lifecycle hook to be registered")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lc.hooks[0].OnStart(ctx); err != nil {
		t.Fatalf("expected optional client startup to degrade without error, got %v", err)
	}
	if raw := client.Raw(); raw != nil {
		t.Fatalf("expected disabled client to hide raw redis handle after ping failure")
	}
	if lc.hooks[0].OnStop != nil {
		if err := lc.hooks[0].OnStop(ctx); err != nil {
			t.Fatalf("stop: %v", err)
		}
	}
}
