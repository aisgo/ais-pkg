package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/aisgo/ais-pkg/mq"
)

type blockingConsumerGroup struct {
	consumeStarted chan struct{}
	unblockConsume chan struct{}
}

func newBlockingConsumerGroup() *blockingConsumerGroup {
	return &blockingConsumerGroup{
		consumeStarted: make(chan struct{}),
		unblockConsume: make(chan struct{}),
	}
}

func (g *blockingConsumerGroup) Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
	select {
	case <-g.consumeStarted:
	default:
		close(g.consumeStarted)
	}

	<-g.unblockConsume
	return nil
}

func (g *blockingConsumerGroup) Errors() <-chan error {
	return nil
}

func (g *blockingConsumerGroup) Close() error {
	return nil
}

func (g *blockingConsumerGroup) Pause(partitions map[string][]int32) {}

func (g *blockingConsumerGroup) Resume(partitions map[string][]int32) {}

func (g *blockingConsumerGroup) PauseAll() {}

func (g *blockingConsumerGroup) ResumeAll() {}

func TestConsumerStartTimeoutDoesNotBlockOnHungConsume(t *testing.T) {
	originalStartTimeout := consumerStartTimeout
	originalStopTimeout := consumerStopTimeout
	consumerStartTimeout = 20 * time.Millisecond
	consumerStopTimeout = 15 * time.Millisecond
	t.Cleanup(func() {
		consumerStartTimeout = originalStartTimeout
		consumerStopTimeout = originalStopTimeout
	})

	group := newBlockingConsumerGroup()
	adapter := &ConsumerAdapter{
		client:   group,
		logger:   zap.NewNop(),
		config:   &mq.KafkaConfig{},
		handlers: make(map[string]mq.MessageHandler),
		topics:   make([]string, 0),
		ready:    make(chan struct{}),
	}

	if err := adapter.Subscribe("orders", func(ctx context.Context, msgs []*mq.ConsumedMessage) (mq.ConsumeResult, error) {
		return mq.ConsumeSuccess, nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		startDone <- adapter.Start()
	}()

	select {
	case <-group.consumeStarted:
	case <-time.After(time.Second):
		t.Fatalf("consumer did not start consume loop")
	}

	select {
	case err := <-startDone:
		if err == nil || err.Error() != "kafka consumer start timeout" {
			t.Fatalf("expected startup timeout error, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("start should return after timeout even if consume loop is stuck")
	}

	close(group.unblockConsume)

	waitDone := make(chan struct{})
	go func() {
		adapter.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatalf("consumer goroutine did not exit after unblocking fake consume")
	}
}
