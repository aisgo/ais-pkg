package rocketmq

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

/* ========================================================================
 * Fx 模块 - RocketMQ 依赖注入
 * ========================================================================
 * 职责: 提供 RocketMQ 的 Fx 依赖注入支持
 *
 * 模块选择:
 *   - Module: 严格模式，配置缺失或初始化失败时返回 error，阻塞启动
 *   - OptionalModule: 宽松模式，配置缺失或初始化失败时返回 nil，不阻塞启动
 *
 * 使用场景:
 *   - 核心服务（必须有 MQ）: 使用 Module
 *   - 可选依赖（MQ 降级运行）: 使用 OptionalModule
 * ======================================================================== */

// Module Fx 模块（严格模式）
// 配置缺失或初始化失败时返回 error，阻塞应用启动
var Module = fx.Module("rocketmq",
	fx.Provide(
		ProvideProducer,
		ProvideConsumer,
	),
)

// OptionalModule Fx 模块（宽松模式）
// 配置缺失或初始化失败时返回 nil，不阻塞应用启动
// 适用于 MQ 作为可选依赖的场景（如本地开发、降级运行）
var OptionalModule = fx.Module("rocketmq-optional",
	fx.Provide(
		OptionalProvideProducer,
		OptionalProvideConsumer,
	),
)

// ProducerParams Producer 依赖参数
type ProducerParams struct {
	fx.In

	Config *Config
	Logger *zap.Logger
}

// ProducerResult Producer 返回结果
type ProducerResult struct {
	fx.Out

	Producer *Producer
}

// ConsumerParams Consumer 依赖参数
type ConsumerParams struct {
	fx.In

	Config *Config
	Logger *zap.Logger
}

// ConsumerResult Consumer 返回结果
type ConsumerResult struct {
	fx.Out

	Consumer *Consumer
}

/* ========================================================================
 * 严格模式 Provider - 失败时返回 error
 * ======================================================================== */

// ProvideProducer 提供 Producer（严格模式）
// 配置缺失或初始化失败时返回 error
func ProvideProducer(lc fx.Lifecycle, params ProducerParams) (ProducerResult, error) {
	if params.Config == nil {
		return ProducerResult{}, fmt.Errorf("rocketmq config is required")
	}
	producer, err := NewProducer(params.Config, params.Logger)
	if err != nil {
		return ProducerResult{}, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return producer.Shutdown()
		},
	})

	return ProducerResult{Producer: producer}, nil
}

// ProvideConsumer 提供 Consumer（严格模式）
// 配置缺失或初始化失败时返回 error
func ProvideConsumer(lc fx.Lifecycle, params ConsumerParams) (ConsumerResult, error) {
	if params.Config == nil {
		return ConsumerResult{}, fmt.Errorf("rocketmq config is required")
	}
	consumer, err := NewConsumer(params.Config, params.Logger)
	if err != nil {
		return ConsumerResult{}, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return consumer.Start()
		},
		OnStop: func(ctx context.Context) error {
			return consumer.Shutdown()
		},
	})

	return ConsumerResult{Consumer: consumer}, nil
}

/* ========================================================================
 * 宽松模式 Provider - 失败时返回 nil
 * ======================================================================== */

// OptionalProvideProducer 提供 Producer（宽松模式）
// 配置缺失或初始化失败时返回 nil，不阻塞启动
func OptionalProvideProducer(lc fx.Lifecycle, params ProducerParams) ProducerResult {
	logger := params.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// 配置缺失检查
	if params.Config == nil || len(params.Config.NameServers) == 0 {
		logger.Warn("RocketMQ producer disabled: name servers not configured")
		return ProducerResult{Producer: nil}
	}

	producer, err := NewProducer(params.Config, params.Logger)
	if err != nil {
		logger.Warn("RocketMQ producer disabled: initialization failed",
			zap.Error(err),
		)
		return ProducerResult{Producer: nil}
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down RocketMQ producer")
			return producer.Shutdown()
		},
	})

	logger.Info("RocketMQ producer initialized",
		zap.Strings("name_servers", params.Config.NameServers),
		zap.String("group", params.Config.Producer.GroupName),
	)

	return ProducerResult{Producer: producer}
}

// OptionalProvideConsumer 提供 Consumer（宽松模式）
// 配置缺失或初始化失败时返回 nil，不阻塞启动
// 注意: 宽松模式下 Consumer 不会自动启动，需要调用方手动调用 Start()
func OptionalProvideConsumer(lc fx.Lifecycle, params ConsumerParams) ConsumerResult {
	logger := params.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// 配置缺失检查
	if params.Config == nil || len(params.Config.NameServers) == 0 {
		logger.Warn("RocketMQ consumer disabled: name servers not configured")
		return ConsumerResult{Consumer: nil}
	}

	consumer, err := NewConsumer(params.Config, params.Logger)
	if err != nil {
		logger.Warn("RocketMQ consumer disabled: initialization failed",
			zap.Error(err),
		)
		return ConsumerResult{Consumer: nil}
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down RocketMQ consumer")
			return consumer.Shutdown()
		},
	})

	logger.Info("RocketMQ consumer initialized (not started, call Start() after Subscribe)",
		zap.Strings("name_servers", params.Config.NameServers),
		zap.String("group", params.Config.Consumer.GroupName),
	)

	return ConsumerResult{Consumer: consumer}
}
