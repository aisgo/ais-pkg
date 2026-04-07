package cache

import (
	"github.com/aisgo/ais-pkg/cache/redis"
	"go.uber.org/fx"
)

/* ========================================================================
 * Cache Module
 * ========================================================================
 * 职责: 提供 Redis 缓存依赖注入模块
 *
 * 模块选择:
 *   - Module: 严格模式，连接失败时阻塞启动
 *   - OptionalModule: 宽松模式，连接失败时返回 nil，不阻塞启动
 * ======================================================================== */

// Module 缓存模块（严格模式）
// 提供: redis.Clienter, *redis.Client
// 连接失败时阻塞应用启动
var Module = fx.Module("cache",
	fx.Provide(
		redis.NewClient,
		func(c *redis.Client) redis.Clienter { return c },
	),
)

// OptionalModule 缓存模块（宽松模式）
// 提供: redis.Clienter, *redis.Client（可能为 nil）
// 配置缺失或连接失败时返回 nil，不阻塞应用启动
var OptionalModule = fx.Module("cache-optional",
	fx.Provide(
		redis.OptionalNewClient,
		func(c *redis.Client) redis.Clienter {
			if c == nil {
				return nil
			}
			return c
		},
	),
)
