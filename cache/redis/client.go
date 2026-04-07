package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/aisgo/ais-pkg/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

/* ========================================================================
 * Redis Client - 缓存 + 分布式锁
 * ========================================================================
 * 职责: 提供 Redis 连接池、缓存操作、分布式锁
 * 技术: go-redis/v9
 *
 * Provider 选择:
 *   - NewClient: 严格模式，连接失败时在 OnStart 阶段返回 error
 *   - OptionalNewClient: 宽松模式，配置缺失或连接失败时返回 nil
 *
 * 使用场景:
 *   - 核心依赖（必须有 Redis）: 使用 NewClient
 *   - 可选依赖（Redis 降级运行）: 使用 OptionalNewClient
 * ======================================================================== */

// Config Redis 配置
type Config struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
}

// IsEmpty 检查配置是否为空（未配置）
func (c Config) IsEmpty() bool {
	return c.Host == ""
}

// Clienter Redis 客户端接口（便于 mock/替换实现）
type Clienter interface {
	Raw() *redis.Client

	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error

	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key string, values ...any) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error

	Ping(ctx context.Context) error
	NewLock(key string, opts ...LockOption) *Lock
}

// Client Redis 客户端封装
type Client struct {
	rdb *redis.Client
	log *logger.Logger
}

var _ Clienter = (*Client)(nil)

type ClientParams struct {
	fx.In
	Lc     fx.Lifecycle
	Config Config
	Logger *logger.Logger
}

// NewClient 创建 Redis 客户端（严格模式）
// 连接失败时在 OnStart 阶段返回 error，阻塞应用启动
func NewClient(p ClientParams) *Client {
	if p.Logger == nil {
		p.Logger = logger.NewNop()
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", p.Config.Host, p.Config.Port),
		Password:     p.Config.Password,
		DB:           p.Config.DB,
		PoolSize:     p.Config.PoolSize,
		MinIdleConns: p.Config.MinIdleConns,
	})

	client := &Client{
		rdb: rdb,
		log: p.Logger,
	}

	p.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 测试连接
			if err := rdb.Ping(ctx).Err(); err != nil {
				p.Logger.Error("Redis connection failed", zap.Error(err))
				return err
			}
			p.Logger.Info("Redis connected",
				zap.String("addr", fmt.Sprintf("%s:%d", p.Config.Host, p.Config.Port)),
			)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Closing Redis connection")
			return rdb.Close()
		},
	})

	return client
}

// OptionalNewClient 创建 Redis 客户端（宽松模式）
// 配置缺失或连接失败时返回 nil，不阻塞应用启动
// 适用于 Redis 作为可选依赖的场景（如本地开发、缓存降级）
func OptionalNewClient(p ClientParams) *Client {
	if p.Logger == nil {
		p.Logger = logger.NewNop()
	}

	// 配置缺失检查
	if p.Config.IsEmpty() {
		p.Logger.Warn("Redis client disabled: host not configured")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", p.Config.Host, p.Config.Port)

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     p.Config.Password,
		DB:           p.Config.DB,
		PoolSize:     p.Config.PoolSize,
		MinIdleConns: p.Config.MinIdleConns,
	})

	// 立即测试连接（在 Provide 阶段而不是 OnStart）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		p.Logger.Warn("Redis client disabled: connection failed",
			zap.String("addr", addr),
			zap.Error(err),
		)
		_ = rdb.Close()
		return nil
	}

	client := &Client{
		rdb: rdb,
		log: p.Logger,
	}

	p.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Closing Redis connection")
			return rdb.Close()
		},
	})

	p.Logger.Info("Redis client initialized",
		zap.String("addr", addr),
		zap.Int("db", p.Config.DB),
	)

	return client
}

// Raw 返回底层 Redis 客户端 (用于高级操作)
func (c *Client) Raw() *redis.Client {
	return c.rdb
}

/* ========================================================================
 * 缓存操作
 * ======================================================================== */

// Get 获取缓存
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

// Set 设置缓存
func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// SetNX 设置缓存 (如果不存在)
func (c *Client) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, expiration).Result()
}

// Del 删除缓存
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// Exists 检查 key 是否存在
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.rdb.Exists(ctx, keys...).Result()
}

// Expire 设置过期时间
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.rdb.Expire(ctx, key, expiration).Err()
}

/* ========================================================================
 * Hash 操作 (用于存储结构化数据)
 * ======================================================================== */

// HGet 获取 Hash 字段
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	return c.rdb.HGet(ctx, key, field).Result()
}

// HSet 设置 Hash 字段
func (c *Client) HSet(ctx context.Context, key string, values ...any) error {
	return c.rdb.HSet(ctx, key, values...).Err()
}

// HGetAll 获取所有 Hash 字段
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.rdb.HGetAll(ctx, key).Result()
}

// HDel 删除 Hash 字段
func (c *Client) HDel(ctx context.Context, key string, fields ...string) error {
	return c.rdb.HDel(ctx, key, fields...).Err()
}

/* ========================================================================
 * 健康检查
 * ======================================================================== */

// Ping 健康检查
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// PoolStats 返回连接池统计信息，用于监控连接池健康状况
func (c *Client) PoolStats() *redis.PoolStats {
	return c.rdb.PoolStats()
}
