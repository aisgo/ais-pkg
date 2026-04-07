package redis

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

/* ========================================================================
 * 分布式锁 - 基于 Redis 的 Redlock 简化实现
 * ========================================================================
 * 职责: 防止高并发下的资源竞争
 * 使用场景: 分布式系统中的并发控制
 * ======================================================================== */

var (
	ErrLockFailed   = errors.New("failed to acquire lock")
	ErrUnlockFailed = errors.New("failed to release lock")
)

// Lock 分布式锁
type Lock struct {
	client       *Client
	key          string
	value        string // 唯一标识，防止误删
	ttl          time.Duration
	defaultOpt   LockOption
	extendCtx    context.Context
	extendCancel context.CancelFunc
	acquired     bool       // 标记是否已获取锁（用于续期失败判断）
	mu           sync.Mutex // 保护 value, ttl, extendCtx, extendCancel, acquired
	acquireMu    sync.Mutex // 串行化 Acquire/续期流程，避免并发竞态
}

// OnLockLostFunc 锁丢失回调函数类型
type OnLockLostFunc func(context.Context)

// LockOption 锁选项
type LockOption struct {
	TTL                time.Duration  // 锁过期时间
	RetryTimes         int            // 重试次数
	RetryDelay         time.Duration  // 重试间隔
	AutoExtend         bool           // 是否自动续期
	ExtendFactor       float64        // 续期触发因子（TTL 的多少比例时触发续期）
	MaxLifetime        time.Duration  // 自动续期最大生命周期（<=0 使用默认值 TTL*10）
	IgnoreParentCancel bool           // 是否忽略父 context 的取消信号
	OnLockLost         OnLockLostFunc // 锁丢失回调（续期失败时调用）
}

// DefaultLockOption 默认锁选项
func DefaultLockOption() LockOption {
	return LockOption{
		TTL:          30 * time.Second,
		RetryTimes:   5,
		RetryDelay:   100 * time.Millisecond,
		AutoExtend:   true, // 默认启用自动续期，防止锁永久锁定
		ExtendFactor: 0.5,  // TTL 的 50% 时续期
		MaxLifetime:  0,
	}
}

// NewLock 创建分布式锁
func (c *Client) NewLock(key string, opts ...LockOption) *Lock {
	opt := DefaultLockOption()
	if len(opts) > 0 {
		opt = opts[0]
	}

	return &Lock{
		client:     c,
		key:        "lock:" + key,
		value:      uuid.New().String(),
		ttl:        opt.TTL,
		defaultOpt: opt,
	}
}

// Acquire 获取锁
func (l *Lock) Acquire(ctx context.Context) error {
	return l.AcquireWithOption(ctx, l.defaultOpt)
}

// AcquireWithOption 带选项获取锁
func (l *Lock) AcquireWithOption(ctx context.Context, opt LockOption) error {
	l.acquireMu.Lock()
	defer l.acquireMu.Unlock()

	ttl := l.setTTL(opt.TTL)
	l.updateOnLockLost(opt.OnLockLost)

	// 已持有锁时尝试续期刷新（仅在本实例之前成功获取过锁时才尝试）
	l.mu.Lock()
	wasAcquired := l.acquired
	l.mu.Unlock()
	if wasAcquired {
		if err := l.Extend(ctx, ttl); err == nil {
			if opt.AutoExtend {
				l.startAutoExtend(ctx, ttl, opt.ExtendFactor, opt.MaxLifetime, opt.IgnoreParentCancel)
			} else {
				l.stopAutoExtend()
			}
			return nil
		} else if !errors.Is(err, ErrLockFailed) {
			// 非"锁不存在"错误（如网络错误），直接返回
			return err
		}
		// ErrLockFailed 表示锁已被其他实例持有，走正常获取流程
		l.mu.Lock()
		l.acquired = false
		l.mu.Unlock()
	}

	value := uuid.New().String()
	for i := 0; i < opt.RetryTimes; i++ {
		ok, err := l.client.SetNX(ctx, l.key, value, ttl)
		if err != nil {
			return err
		}
		if ok {
			l.mu.Lock()
			l.value = value
			l.acquired = true
			l.mu.Unlock()
			if opt.AutoExtend {
				l.startAutoExtend(ctx, ttl, opt.ExtendFactor, opt.MaxLifetime, opt.IgnoreParentCancel)
			} else {
				l.stopAutoExtend()
			}
			return nil
		}

		// 等待重试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opt.RetryDelay):
		}
	}

	l.mu.Lock()
	l.acquired = false
	l.mu.Unlock()
	return ErrLockFailed
}

// startAutoExtend 启动自动续期（线程安全）
func (l *Lock) startAutoExtend(parentCtx context.Context, ttl time.Duration, extendFactor float64, maxLifetime time.Duration, ignoreParentCancel bool) {
	// 先停止旧的续期 goroutine（如果存在）
	l.stopAutoExtend()

	l.mu.Lock()
	defer l.mu.Unlock()

	if extendFactor <= 0 || extendFactor > 1 {
		extendFactor = DefaultLockOption().ExtendFactor
	}

	if maxLifetime <= 0 {
		maxLifetime = ttl * 10
	}

	// 默认继承父 context 的取消信号
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx := parentCtx
	if ignoreParentCancel {
		ctx = context.WithoutCancel(parentCtx)
	}
	ctx, cancel := context.WithCancel(ctx)
	l.extendCtx, l.extendCancel = ctx, cancel

	go l.autoExtendLoop(l.extendCtx, ttl, extendFactor, maxLifetime)
}

// stopAutoExtend 停止自动续期（线程安全）
func (l *Lock) stopAutoExtend() {
	l.mu.Lock()
	cancel := l.extendCancel
	l.extendCancel = nil
	l.extendCtx = nil
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// autoExtendLoop 自动续期循环
func (l *Lock) autoExtendLoop(ctx context.Context, ttl time.Duration, extendFactor float64, maxLifetime time.Duration) {
	// 计算续期间隔
	interval := time.Duration(float64(ttl) * extendFactor)

	// 添加最大生命周期保护（防止无限续期导致 goroutine 泄漏）
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, maxLifetime)
	defer deadlineCancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-deadlineCtx.Done():
			// 超过最大生命周期或被取消
			return

		case <-ticker.C:
			// 尝试续期
			if !l.tryExtend(deadlineCtx, ttl) {
				// 续期失败，可能锁已丢失
				return
			}
		}
	}
}

// tryExtend 尝试续期，返回是否应继续
func (l *Lock) tryExtend(ctx context.Context, ttl time.Duration) bool {
	for i := range 3 {
		extendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := l.Extend(extendCtx, ttl)
		cancel()

		if err == nil {
			return true
		}

		// 锁已被其他实例持有，视为锁丢失，触发回调
		if errors.Is(err, ErrLockFailed) {
			l.onLockLostIfAcquired()
			return false
		}
		// context.Canceled 说明是 stopAutoExtend() 主动取消（如 Release 前调用），
		// 不应触发 lock-lost 回调，直接退出续期循环即可
		if errors.Is(err, context.Canceled) {
			return false
		}

		// 临时错误，指数退避
		backoff := time.Duration(100*(1<<i)) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(backoff):
			continue
		}
	}

	// 重试多次仍失败，视为锁丢失
	l.onLockLostIfAcquired()
	return false
}

// onLockLostIfAcquired 如果锁已被获取过，则异步调用锁丢失回调。
// 回调接收一个带有 5s 超时的 context；调用方如需异步或更细粒度超时，应在回调内部自行处理。
func (l *Lock) onLockLostIfAcquired() {
	l.mu.Lock()
	acquired := l.acquired
	onLockLost := l.defaultOpt.OnLockLost
	l.acquired = false // 标记锁已丢失
	l.mu.Unlock()

	if acquired && onLockLost != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			onLockLost(ctx)
		}()
	}
}

func (l *Lock) setTTL(ttl time.Duration) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ttl > 0 {
		l.ttl = ttl
	}

	return l.ttl
}

func (l *Lock) updateOnLockLost(onLockLost OnLockLostFunc) {
	if onLockLost == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.defaultOpt.OnLockLost = onLockLost
}

// Release 释放锁
// 使用 Lua 脚本保证原子性：只有持有锁的人才能释放
func (l *Lock) Release(ctx context.Context) error {
	// 停止自动续期 goroutine
	l.stopAutoExtend()

	l.mu.Lock()
	value := l.value
	l.mu.Unlock()

	// Lua 脚本: 如果 value 匹配则删除
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result, err := l.client.rdb.Eval(ctx, script, []string{l.key}, value).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrUnlockFailed
	}
	return nil
}

// Extend 延长锁时间
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) error {
	// 加锁保护 value 读取，防止竞态条件
	l.mu.Lock()
	value := l.value
	l.mu.Unlock()

	// Lua 脚本: 如果 value 匹配则延长过期时间
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := l.client.rdb.Eval(ctx, script, []string{l.key}, value, ttl.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLockFailed
	}
	return nil
}

// IsHeld 检查当前实例是否仍持有锁
// 用于主动检查锁状态，特别是在长时间操作后
func (l *Lock) IsHeld(ctx context.Context) (bool, error) {
	l.mu.Lock()
	value := l.value
	l.mu.Unlock()

	// 读取 Redis 中的 value 并与本地比较
	actualValue, err := l.client.rdb.Get(ctx, l.key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil // 锁已不存在
		}
		return false, err
	}

	return actualValue == value, nil
}
