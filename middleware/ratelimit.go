package middleware

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aisgo/ais-pkg/response"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"
)

const (
	defaultRateLimit  = 1000
	defaultRatePeriod = time.Second
)

// RateLimitKeyFunc returns an identifier used for rate limiting.
type RateLimitKeyFunc func(fiber.Ctx) string

var (
	// WARNING: 以下限速器与 key 函数是包级全局状态。
	// 多个模块、测试或不同路由组共享它们时，会共享同一份限速配额与 key 规则，
	// 可能导致彼此互相影响、配额串用或测试相互污染。
	// 仅在“整个应用明确共用一个限速器”时使用这些全局变量；
	// 更稳妥的做法是为每个路由组显式调用 NewRateLimitMiddleware(lim, keyFunc)。
	rateLimiterMu      sync.RWMutex
	rateLimiter        *limiter.Limiter
	defaultLimiter     *limiter.Limiter
	defaultLimiterOnce sync.Once

	rateLimitKeyMu   sync.RWMutex
	rateLimitKeyFunc RateLimitKeyFunc
)

// SetRateLimiter replaces the global limiter and returns the previous one.
func SetRateLimiter(lim *limiter.Limiter) *limiter.Limiter {
	rateLimiterMu.Lock()
	defer rateLimiterMu.Unlock()
	prev := rateLimiter
	rateLimiter = lim
	return prev
}

// SetRateLimitKeyFunc replaces the key function and returns the previous one.
func SetRateLimitKeyFunc(fn RateLimitKeyFunc) RateLimitKeyFunc {
	rateLimitKeyMu.Lock()
	defer rateLimitKeyMu.Unlock()
	prev := rateLimitKeyFunc
	rateLimitKeyFunc = fn
	return prev
}

// InitRateLimiter initializes a redis-based limiter with default settings.
func InitRateLimiter(client *redis.Client) error {
	if client == nil {
		return nil
	}
	store, err := redisstore.NewStore(client)
	if err != nil {
		return err
	}
	lim := limiter.New(store, limiter.Rate{Period: defaultRatePeriod, Limit: defaultRateLimit})
	SetRateLimiter(lim)
	return nil
}

// NewRateLimitMiddleware returns a rate-limiting handler bound to the given limiter and
// key function, with no global state. Prefer this over RateLimitMiddleware for test
// isolation and concurrent use across multiple route groups.
func NewRateLimitMiddleware(lim *limiter.Limiter, keyFunc RateLimitKeyFunc) fiber.Handler {
	return func(c fiber.Ctx) error {
		key := "ip:" + c.IP()
		if keyFunc != nil {
			if k := strings.TrimSpace(keyFunc(c)); k != "" {
				key = k
			}
		}
		return checkRateLimit(c, lim, key)
	}
}

// RateLimitMiddleware 应用全局共享的限速器。
//
// 注意：此函数使用包级全局限速器，适合在整个应用统一限速的场景。
// 若需要为不同路由组设置独立限速器，或有并发隔离需求，
// 请改用 NewRateLimitMiddleware(lim, keyFunc)，避免全局状态带来的干扰。
func RateLimitMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		return checkRateLimit(c, currentRateLimiter(), rateLimitKey(c))
	}
}

// checkRateLimit 执行限速检查、设置响应头、判断是否超限（共用逻辑）
func checkRateLimit(c fiber.Ctx, lim *limiter.Limiter, key string) error {
	ctx, err := lim.Get(c.Context(), key)
	if err != nil {
		return response.ErrorWithCode(c, fiber.StatusInternalServerError, fmt.Errorf("rate limit check failed: %w", err))
	}

	c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", ctx.Limit))
	c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", ctx.Remaining))

	if ctx.Reached {
		return response.ErrorWithCode(c, fiber.StatusTooManyRequests, fmt.Errorf("too many requests"))
	}

	return c.Next()
}

func currentRateLimiter() *limiter.Limiter {
	rateLimiterMu.RLock()
	if rateLimiter != nil {
		lim := rateLimiter
		rateLimiterMu.RUnlock()
		return lim
	}
	rateLimiterMu.RUnlock()

	defaultLimiterOnce.Do(func() {
		store := memory.NewStore()
		defaultLimiter = limiter.New(store, limiter.Rate{Period: defaultRatePeriod, Limit: defaultRateLimit})
	})

	return defaultLimiter
}

func rateLimitKey(c fiber.Ctx) string {
	rateLimitKeyMu.RLock()
	fn := rateLimitKeyFunc
	rateLimitKeyMu.RUnlock()
	if fn != nil {
		key := strings.TrimSpace(fn(c))
		if key != "" {
			return key
		}
	}
	return "ip:" + c.IP()
}
