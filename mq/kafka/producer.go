package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/aisgo/ais-pkg/mq"
)

/* ========================================================================
 * Kafka Producer - Kafka 消息生产者
 * ========================================================================
 * 职责: 实现 mq.Producer 接口
 * 技术: IBM/sarama
 * ======================================================================== */

// =============================================================================
// 注册工厂
// =============================================================================

func init() {
	mq.RegisterProducerFactory(mq.TypeKafka, NewProducerAdapter)
}

// =============================================================================
// Producer 适配器
// =============================================================================

// syncSendResult 同步发送的内部结果类型，用于将 asyncProducer 的异步结果桥接为同步
type syncSendResult struct {
	result *mq.SendResult
	err    error
}

// syncSendMetadata 同步发送时存储在 ProducerMessage.Metadata 中的上下文与结果 channel
type syncSendMetadata struct {
	ctx context.Context
	ch  chan syncSendResult
}

// ProducerAdapter Kafka 生产者适配器
type ProducerAdapter struct {
	asyncProducer sarama.AsyncProducer
	logger        *zap.Logger
	wg            sync.WaitGroup
	closed        bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewProducerAdapter 创建 Kafka 生产者适配器
func NewProducerAdapter(cfg *mq.Config, logger *zap.Logger) (mq.Producer, error) {
	if cfg.Kafka == nil {
		return nil, fmt.Errorf("kafka config is required")
	}

	kafkaCfg := cfg.Kafka

	// 构建 Sarama 配置
	saramaCfg, err := buildSaramaConfig(kafkaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build sarama config: %w", err)
	}

	// 创建异步生产者（同步发送也通过此实例实现，避免双连接）
	asyncProducer, err := sarama.NewAsyncProducer(kafkaCfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka async producer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	adapter := &ProducerAdapter{
		asyncProducer: asyncProducer,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
	}

	// 启动异步错误处理
	adapter.wg.Add(1)
	go adapter.handleAsyncErrors()

	logger.Info("Kafka producer started",
		zap.Strings("brokers", kafkaCfg.Brokers),
	)

	return adapter, nil
}

// handleAsyncErrors 处理异步发送错误
func (p *ProducerAdapter) handleAsyncErrors() {
	defer p.wg.Done()

	for {
		select {
		case err, ok := <-p.asyncProducer.Errors():
			if !ok {
				return
			}
			// 优先投递给同步等待方；其次调用异步回调；兜底记录日志
			if !p.deliverSyncSendResult(err.Msg.Metadata, syncSendResult{err: err.Err}) {
				if cb, ok := err.Msg.Metadata.(mq.SendCallback); ok && cb != nil {
					cb(nil, err.Err)
				} else {
					p.logger.Error("async producer error",
						zap.String("topic", err.Msg.Topic),
						zap.Error(err.Err),
					)
				}
			}
		case msg, ok := <-p.asyncProducer.Successes():
			if !ok {
				return
			}
			result := &mq.SendResult{
				MsgID:     fmt.Sprintf("%s-%d-%d", msg.Topic, msg.Partition, msg.Offset),
				Topic:     msg.Topic,
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Status:    mq.SendStatusOK,
			}
			// 优先投递给同步等待方；其次调用异步回调；兜底记录日志
			if !p.deliverSyncSendResult(msg.Metadata, syncSendResult{result: result}) {
				if cb, ok := msg.Metadata.(mq.SendCallback); ok && cb != nil {
					cb(result, nil)
				} else {
					p.logger.Debug("async message sent",
						zap.String("topic", msg.Topic),
						zap.Int32("partition", msg.Partition),
						zap.Int64("offset", msg.Offset),
					)
				}
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// deliverSyncSendResult 将异步结果投递到同步 channel，返回是否投递成功。
// 结果通道只允许非阻塞投递，避免迟到结果把后台 goroutine 永久卡住。
func (p *ProducerAdapter) deliverSyncSendResult(metadata any, result syncSendResult) bool {
	m, ok := metadata.(syncSendMetadata)
	if !ok {
		return false
	}
	if m.ctx != nil {
		// 已取消的同步请求不再接收结果，避免迟到回调写入已经放弃等待的接收方。
		select {
		case <-m.ctx.Done():
			return false
		default:
		}

		// 同时监听 ctx 取消与 channel 投递，并在接收方不可用时立即丢弃结果，
		// 避免 channel 已满时永久阻塞。
		select {
		case m.ch <- result:
			return true
		case <-m.ctx.Done():
			return false
		default:
			p.logger.Warn("dropping sync send result because receiver is unavailable")
			return false
		}
	}
	// 无上下文时使用非阻塞投递
	select {
	case m.ch <- result:
		return true
	default:
		p.logger.Warn("dropping sync send result because receiver is unavailable")
		return false
	}
}

// SendSync 同步发送消息（通过异步生产者实现，避免双连接）
func (p *ProducerAdapter) SendSync(ctx context.Context, msg *mq.Message) (*mq.SendResult, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	kafkaMsg := convertToKafkaMessage(msg)
	ch := make(chan syncSendResult, 1)
	kafkaMsg.Metadata = syncSendMetadata{ctx: ctx, ch: ch}

	select {
	case p.asyncProducer.Input() <- kafkaMsg:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case res := <-ch:
		if res.err != nil {
			p.logger.Error("failed to send message",
				zap.String("topic", msg.Topic),
				zap.Error(res.err),
			)
			return nil, res.err
		}
		p.logger.Debug("message sent",
			zap.String("topic", msg.Topic),
			zap.Int32("partition", res.result.Partition),
			zap.Int64("offset", res.result.Offset),
		)
		return res.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendAsync 异步发送消息
func (p *ProducerAdapter) SendAsync(ctx context.Context, msg *mq.Message, callback mq.SendCallback) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	kafkaMsg := convertToKafkaMessage(msg)
	kafkaMsg.Metadata = callback

	// 注意：Sarama 的异步 Producer 不支持单消息回调
	// 回调通过 Successes() 和 Errors() channel 处理（使用 ProducerMessage.Metadata 关联）
	select {
	case p.asyncProducer.Input() <- kafkaMsg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 关闭生产者
func (p *ProducerAdapter) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	var errs []error

	if err := p.asyncProducer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("async producer close error: %w", err))
	}
	// 确保后台 goroutine 退出
	p.cancel()

	p.wg.Wait()

	if len(errs) > 0 {
		p.logger.Error("failed to close producer", zap.Errors("errors", errs))
		return errs[0]
	}

	p.logger.Info("Kafka producer closed")
	return nil
}

// =============================================================================
// 辅助函数
// =============================================================================

func buildSaramaConfig(cfg *mq.KafkaConfig) (*sarama.Config, error) {
	saramaCfg := sarama.NewConfig()

	// 版本
	if cfg.Version != "" {
		version, err := sarama.ParseKafkaVersion(cfg.Version)
		if err != nil {
			return nil, fmt.Errorf("invalid kafka version: %w", err)
		}
		saramaCfg.Version = version
	}

	// Producer 配置
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Return.Errors = true
	saramaCfg.Producer.Retry.Max = cfg.Producer.RetryMax
	saramaCfg.Producer.Timeout = cfg.Producer.Timeout

	// ACKs
	switch cfg.Producer.RequiredAcks {
	case "none":
		saramaCfg.Producer.RequiredAcks = sarama.NoResponse
	case "leader":
		saramaCfg.Producer.RequiredAcks = sarama.WaitForLocal
	case "all":
		saramaCfg.Producer.RequiredAcks = sarama.WaitForAll
	default:
		saramaCfg.Producer.RequiredAcks = sarama.WaitForLocal
	}

	// 压缩
	switch cfg.Producer.Compression {
	case "gzip":
		saramaCfg.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		saramaCfg.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		saramaCfg.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaCfg.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaCfg.Producer.Compression = sarama.CompressionNone
	}

	// 幂等
	saramaCfg.Producer.Idempotent = cfg.Producer.Idempotent
	if cfg.Producer.Idempotent {
		saramaCfg.Net.MaxOpenRequests = 1
	}

	// 消息大小
	if cfg.Producer.MaxMessageBytes > 0 {
		saramaCfg.Producer.MaxMessageBytes = cfg.Producer.MaxMessageBytes
	}

	// SASL
	if err := applySASL(saramaCfg, cfg.SASL); err != nil {
		return nil, err
	}

	// TLS
	if err := applyTLS(saramaCfg, cfg.TLS); err != nil {
		return nil, err
	}

	return saramaCfg, nil
}

// applySASL 将 SASL 配置应用到 Sarama 配置（消费者和生产者共用）
func applySASL(saramaCfg *sarama.Config, cfg mq.KafkaSASLConfig) error {
	if !cfg.Enable {
		return nil
	}
	saramaCfg.Net.SASL.Enable = true
	saramaCfg.Net.SASL.User = cfg.Username
	saramaCfg.Net.SASL.Password = cfg.Password

	switch cfg.Mechanism {
	case "SCRAM-SHA-256":
		saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &XDGSCRAMClient{HashGeneratorFcn: SHA256}
		}
		saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		saramaCfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &XDGSCRAMClient{HashGeneratorFcn: SHA512}
		}
		saramaCfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
	case "PLAIN", "":
		saramaCfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	default:
		return fmt.Errorf("unsupported SASL mechanism %q", cfg.Mechanism)
	}

	return nil
}

// applyTLS 将 TLS 配置应用到 Sarama 配置（消费者和生产者共用）
func applyTLS(saramaCfg *sarama.Config, cfg mq.KafkaTLSConfig) error {
	if !cfg.Enable {
		return nil
	}
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to build TLS config: %w", err)
	}
	saramaCfg.Net.TLS.Enable = true
	saramaCfg.Net.TLS.Config = tlsConfig
	return nil
}

func buildTLSConfig(cfg mq.KafkaTLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.Insecure,
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, fmt.Errorf("failed to append CA certs from PEM")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func convertToKafkaMessage(msg *mq.Message) *sarama.ProducerMessage {
	kafkaMsg := &sarama.ProducerMessage{
		Topic:     msg.Topic,
		Value:     sarama.ByteEncoder(msg.Body),
		Timestamp: time.Now(),
	}

	// Key
	if msg.Key != "" {
		kafkaMsg.Key = sarama.StringEncoder(msg.Key)
	}

	// Headers (properties)
	if len(msg.Properties) > 0 {
		headers := make([]sarama.RecordHeader, 0, len(msg.Properties))
		for k, v := range msg.Properties {
			headers = append(headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}
		kafkaMsg.Headers = headers
	}

	// Tag 作为 header
	if msg.Tag != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte("X-Tag"),
			Value: []byte(msg.Tag),
		})
	}

	// DelayTime 作为 header（Kafka 原生不支持延迟消息，通过 header 传递，消费端自行实现延迟逻辑）
	if msg.DelayTime > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte("X-Delay-Ms"),
			Value: []byte(fmt.Sprintf("%d", msg.DelayTime.Milliseconds())),
		})
	}

	return kafkaMsg
}
