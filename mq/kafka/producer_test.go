package kafka

import (
	"context"
	"os"
	"testing"

	"github.com/aisgo/ais-pkg/mq"
	"go.uber.org/zap"
)

func TestDeliverSyncSendResultDoesNotBlockOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := make(chan syncSendResult, 1)
	adapter := &ProducerAdapter{logger: zap.NewNop()}

	if ok := adapter.deliverSyncSendResult(syncSendMetadata{ctx: ctx, ch: ch}, syncSendResult{err: context.Canceled}); ok {
		t.Fatalf("expected canceled sync send result to be dropped")
	}
	if len(ch) != 0 {
		t.Fatalf("expected no result to be delivered after context cancellation")
	}
}

func TestDeliverSyncSendResultDoesNotBlockOnFullChannel(t *testing.T) {
	ch := make(chan syncSendResult, 1)
	ch <- syncSendResult{}

	adapter := &ProducerAdapter{logger: zap.NewNop()}
	if ok := adapter.deliverSyncSendResult(syncSendMetadata{ctx: context.Background(), ch: ch}, syncSendResult{}); ok {
		t.Fatalf("expected sync send result to be dropped when channel is full")
	}
}

func TestBuildSaramaConfigRejectsUnknownSASLMechanism(t *testing.T) {
	cfg := mq.DefaultKafkaConfig()
	cfg.SASL.Enable = true
	cfg.SASL.Username = "user"
	cfg.SASL.Password = "pass"
	cfg.SASL.Mechanism = "SCRAM-SHA-999"

	if _, err := buildSaramaConfig(cfg); err == nil {
		t.Fatalf("expected unsupported SASL mechanism error")
	}
}

func TestBuildTLSConfigRejectsInvalidPEM(t *testing.T) {
	dir := t.TempDir()
	caFile := dir + "/ca.pem"
	if err := os.WriteFile(caFile, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write ca file: %v", err)
	}

	if _, err := buildTLSConfig(mq.KafkaTLSConfig{
		Enable: true,
		CAFile: caFile,
	}); err == nil {
		t.Fatalf("expected invalid pem error")
	}
}
