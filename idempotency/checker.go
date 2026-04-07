package idempotency

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aisgo/ais-pkg/cache/redis"
)

/* ========================================================================
 * Idempotency - 幂等性检查器
 * ========================================================================
 * 职责: 提供基于 Redis 的幂等性检查，防止消息/请求重复处理
 * 技术: Redis SetNX + TTL
 *
 * 模式说明:
 *   - required: 严格模式，Redis 不可用时返回错误
 *   - best_effort: 尽力模式，Redis 不可用时降级为不检查
 *   - disabled: 禁用模式，跳过所有检查
 *
 * 环境变量: IDEMPOTENCY_MODE（默认 best_effort）
 *
 * 使用示例:
 *
 *   checker := idempotency.New(redisClient, idempotency.Config{
 *       KeyPrefix: "myapp:processed:",
 *   })
 *
 *   // 方式一：先检查后标记（两步）
 *   if processed, _ := checker.Check(ctx, "event-123"); processed {
 *       return // 已处理
 *   }
 *   // ... 处理逻辑 ...
 *   checker.Mark(ctx, "event-123")
 *
 *   // 方式二：原子性检查并标记（推荐）
 *   if exists, _ := checker.CheckAndMark(ctx, "event-123"); exists {
 *       return // 已处理
 *   }
 *   // ... 处理逻辑 ...
 * ======================================================================== */

const (
	// ModeRequired 严格模式：Redis 不可用时返回错误
	ModeRequired = "required"
	// ModeBestEffort 尽力模式：Redis 不可用时降级
	ModeBestEffort = "best_effort"
	// ModeDisabled 禁用模式：跳过检查
	ModeDisabled = "disabled"

	// DefaultTTL 默认幂等性 key 过期时间（24 小时）
	DefaultTTL = 24 * time.Hour
	// DefaultEnvKey 默认环境变量名
	DefaultEnvKey = "IDEMPOTENCY_MODE"
)

// Checker 幂等性检查器
type Checker struct {
	redisClient *redis.Client
	keyPrefix   string
	ttl         time.Duration
	mode        string
}

// Config 幂等性配置
type Config struct {
	// KeyPrefix key 前缀，如 "myapp:processed:"
	KeyPrefix string
	// TTL key 过期时间，默认 24 小时
	TTL time.Duration
	// Mode 运行模式，可选 required/best_effort/disabled
	// 留空时从环境变量读取，默认 best_effort
	Mode string
	// EnvModeKey 自定义环境变量名（可选）
	// 如 "MYAPP_IDEMPOTENCY_MODE"，留空时使用 "IDEMPOTENCY_MODE"
	EnvModeKey string
}

// New 创建幂等性检查器
func New(client *redis.Client, cfg Config) *Checker {
	// 确定运行模式
	mode := cfg.Mode
	if mode == "" {
		envKey := cfg.EnvModeKey
		if envKey == "" {
			envKey = DefaultEnvKey
		}
		mode = strings.TrimSpace(strings.ToLower(os.Getenv(envKey)))
	}
	if mode == "" {
		mode = ModeBestEffort
	}

	// 确定 TTL
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}

	return &Checker{
		redisClient: client,
		keyPrefix:   cfg.KeyPrefix,
		ttl:         ttl,
		mode:        mode,
	}
}

// Mode 返回当前运行模式
func (c *Checker) Mode() string {
	return c.mode
}

// IsDisabled 检查是否禁用
func (c *Checker) IsDisabled() bool {
	return c.mode == ModeDisabled
}

// IsAvailable 检查 Redis 是否可用
func (c *Checker) IsAvailable() bool {
	return c.redisClient != nil
}

// Check 检查事件是否已处理
// 返回: (已处理, 错误)
//
// 降级逻辑：
//   - mode=required: Redis 不可用或错误时返回错误
//   - mode=best_effort: Redis 不可用时返回 (false, nil)
//   - mode=disabled: 始终返回 (false, nil)
//
// Deprecated: Check + Mark 是两步操作，存在竞态窗口——Check 返回 false 后、
// Mark 执行前，另一个请求可能已标记同一个 key，导致重复处理。
// 请使用 CheckAndMark 进行原子性检查并标记。
func (c *Checker) Check(ctx context.Context, key string) (bool, error) {
	if c.mode == ModeDisabled {
		return false, nil
	}

	if c.redisClient == nil {
		if c.mode == ModeRequired {
			return false, fmt.Errorf("idempotency check failed: redis not available (mode=%s)", c.mode)
		}
		return false, nil
	}

	fullKey := c.keyPrefix + key
	exists, err := c.redisClient.Exists(ctx, fullKey)
	if err != nil {
		if c.mode == ModeRequired {
			return false, fmt.Errorf("idempotency check failed: %w", err)
		}
		// best_effort 模式：Redis 异常时降级，假定未处理（允许继续）
		// 记录 warn 日志以便运维感知 Redis 故障，但不阻断业务流程
		log.Printf("WARN: idempotency check degraded (best_effort): %v", err)
		return false, nil
	}

	return exists > 0, nil
}

// Mark 标记事件为已处理
//
// Deprecated: 与 Check 配合使用存在竞态窗口，请使用 CheckAndMark。
func (c *Checker) Mark(ctx context.Context, key string) error {
	if c.mode == ModeDisabled {
		return nil
	}

	if c.redisClient == nil {
		if c.mode == ModeRequired {
			return fmt.Errorf("idempotency mark failed: redis not available (mode=%s)", c.mode)
		}
		return nil
	}

	fullKey := c.keyPrefix + key
	return c.redisClient.Set(ctx, fullKey, "1", c.ttl)
}

// CheckAndMark 原子性检查并标记（使用 SetNX）
// 返回: (已存在, 错误)
// 如果返回 (true, nil)，表示已被其他进程处理
// 如果返回 (false, nil)，表示本次成功标记
func (c *Checker) CheckAndMark(ctx context.Context, key string) (bool, error) {
	if c.mode == ModeDisabled {
		return false, nil
	}

	if c.redisClient == nil {
		if c.mode == ModeRequired {
			return false, fmt.Errorf("idempotency check-and-mark failed: redis not available (mode=%s)", c.mode)
		}
		return false, nil
	}

	fullKey := c.keyPrefix + key
	set, err := c.redisClient.SetNX(ctx, fullKey, "1", c.ttl)
	if err != nil {
		if c.mode == ModeRequired {
			return false, fmt.Errorf("idempotency check-and-mark failed: %w", err)
		}
		// best_effort 模式：Redis 异常时降级，假定未处理（允许继续）
		// 记录 warn 日志以便运维感知 Redis 故障，但不阻断业务流程
		log.Printf("WARN: idempotency check-and-mark degraded (best_effort): %v", err)
		return false, nil
	}

	// SetNX 返回 true 表示设置成功（之前不存在），返回 false 表示已存在
	return !set, nil
}
