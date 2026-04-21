package rocketmq

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type testLifecycle struct {
	hooks []fx.Hook
}

func (l *testLifecycle) Append(h fx.Hook) {
	l.hooks = append(l.hooks, h)
}

func TestProvideProducerRejectsNilConfig(t *testing.T) {
	lc := &testLifecycle{}
	if _, err := ProvideProducer(lc, ProducerParams{
		Config: nil,
		Logger: zap.NewNop(),
	}); err == nil {
		t.Fatalf("expected nil config error")
	}
}

func TestProvideConsumerRejectsNilConfig(t *testing.T) {
	lc := &testLifecycle{}
	if _, err := ProvideConsumer(lc, ConsumerParams{
		Config: nil,
		Logger: zap.NewNop(),
	}); err == nil {
		t.Fatalf("expected nil config error")
	}
}
