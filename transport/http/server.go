package http

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aisgo/ais-pkg/logger"
	"github.com/aisgo/ais-pkg/metrics"
	"github.com/aisgo/ais-pkg/middleware"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

/* ========================================================================
 * HTTP Server - Fiber v3 HTTP 服务器
 * ========================================================================
 * 职责: 提供 HTTP 服务，健康检查，指标暴露
 * 技术: Fiber v3
 * ======================================================================== */

// Config HTTP 服务器配置
type Config struct {
	Port               int           `yaml:"port"`
	Host               string        `yaml:"host"`
	AppName            string        `yaml:"app_name"`
	ReadTimeout        time.Duration `yaml:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout"`
	IdleTimeout        time.Duration `yaml:"idle_timeout"`
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout"`
	ExposeRuntimeStats bool          `yaml:"expose_runtime_stats"` // 是否在 /readyz 暴露内存和 goroutine 统计，默认 false

	// EnableRecover 是否启用 Panic 恢复中间件，默认 true（生产环境推荐）
	// 设为 false 可在开发/测试环境直接暴露 panic，便于问题定位
	EnableRecover *bool `yaml:"enable_recover"`

	// CORS 跨域资源共享配置
	CORS middleware.CORSConfig `yaml:"cors"`

	// Listen 嵌套 ListenConfig 的可序列化配置项
	Listen ListenOptions `yaml:"listen"`
}

// ListenOptions 包含 Fiber ListenConfig 中可以通过 YAML 配置的字段
// 对于更高级的配置（如 TLSConfigFunc、BeforeServeFunc 等函数类型），
// 请使用 ServerParams 中的 ListenConfigCustomizer
type ListenOptions struct {
	// EnablePrefork 是否启用 Prefork 模式（多进程），默认 false
	// Prefork 模式使用 SO_REUSEPORT socket 选项，允许多个 Go 进程监听同一端口，提升性能
	//
	// 注意事项：
	//   1. Docker 环境：确保使用 `CMD ./app` 或 `CMD ["sh", "-c", "/app"]` 启动
	//   2. 内存隔离：各进程不共享内存，不适合需要进程间共享状态的场景（如内存缓存）
	//   3. 网络类型：只支持 tcp4 或 tcp6，不支持 unix socket
	//   4. 数据库连接：每个子进程会有独立的连接池，需要合理配置连接池大小
	EnablePrefork bool `yaml:"enable_prefork"`

	// 是否禁用启动消息，默认 false
	DisableStartupMessage bool `yaml:"disable_startup_message"`

	// 是否打印所有路由，默认 false
	EnablePrintRoutes bool `yaml:"enable_print_routes"`

	// 监听网络类型（tcp, tcp4, tcp6, unix），默认 tcp4
	// 注意：使用 Prefork 时只能选择 tcp4 或 tcp6
	ListenerNetwork string `yaml:"listener_network"`

	// TLS 证书文件路径
	CertFile string `yaml:"cert_file"`

	// TLS 证书私钥文件路径
	CertKeyFile string `yaml:"cert_key_file"`

	// mTLS 客户端证书文件路径
	CertClientFile string `yaml:"cert_client_file"`

	// 优雅关闭超时时间，默认 10s
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`

	// Unix Socket 文件权限模式，默认 0770
	UnixSocketFileMode uint32 `yaml:"unix_socket_file_mode"`

	// TLS 最低版本，默认 TLS 1.2
	// 可选值: 771 (TLS 1.2), 772 (TLS 1.3)
	TLSMinVersion uint16 `yaml:"tls_min_version"`
}

// ListenConfigCustomizer 自定义 ListenConfig 的函数类型
// 用于配置那些无法通过 YAML 序列化的高级选项（如回调函数、context 等）
type ListenConfigCustomizer func(*fiber.ListenConfig)

// AppConfigCustomizer 自定义 Fiber Config
// 用于配置 Fiber ErrorHandler 或其他高级选项
type AppConfigCustomizer func(*fiber.Config)

type ServerParams struct {
	fx.In
	Lc     fx.Lifecycle
	Config Config
	Logger *logger.Logger
	DB     *gorm.DB `optional:"true"` // 用于健康检查，可选

	// ErrorHandler 可选的 Fiber ErrorHandler
	ErrorHandler fiber.ErrorHandler `optional:"true"`

	// ListenConfigCustomizer 可选的 ListenConfig 自定义函数
	// 使用此函数可以设置更高级的配置，如：
	//   - GracefulContext: 优雅关闭的 context
	//   - TLSConfigFunc: 自定义 TLS 配置函数
	//   - ListenerAddrFunc: 监听地址回调
	//   - BeforeServeFunc: 服务启动前的回调
	//   - AutoCertManager: ACME 自动证书管理器
	ListenConfigCustomizer ListenConfigCustomizer `optional:"true"`

	// AppConfigCustomizer 可选的 Fiber Config 自定义函数
	AppConfigCustomizer AppConfigCustomizer `optional:"true"`

	// CORSConfig 可选的 CORS 配置（优先级高于 Config.CORS）
	// 用于通过依赖注入动态配置 CORS，而不是从配置文件读取
	CORSConfig *middleware.CORSConfig `optional:"true"`
}

// NewHTTPServer 创建 HTTP 服务器并注册生命周期
func NewHTTPServer(p ServerParams) *fiber.App {
	// 应用默认值
	readTimeout := p.Config.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 30 * time.Second
	}
	writeTimeout := p.Config.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 30 * time.Second
	}
	idleTimeout := p.Config.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 120 * time.Second
	}
	appName := p.Config.AppName
	if appName == "" {
		appName = "AIS Go App"
	}

	appConfig := fiber.Config{
		AppName:      appName,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
	}

	if p.AppConfigCustomizer != nil {
		p.AppConfigCustomizer(&appConfig)
	}
	if p.ErrorHandler != nil {
		appConfig.ErrorHandler = p.ErrorHandler
	}

	app := fiber.New(appConfig)

	// 默认启用 Recover 中间件（生产环境必备，防止 panic 导致服务崩溃）
	// 可通过配置 enable_recover: false 在测试环境禁用，便于问题暴露
	enableRecover := true
	if p.Config.EnableRecover != nil {
		enableRecover = *p.Config.EnableRecover
	}

	if enableRecover {
		app.Use(recoverer.New(recoverer.Config{
			EnableStackTrace: true,
			StackTraceHandler: func(c fiber.Ctx, e interface{}) {
				p.Logger.Error("Panic recovered",
					zap.Any("error", e),
					zap.String("path", c.Path()),
					zap.String("method", c.Method()),
					zap.String("ip", c.IP()),
				)
			},
		}))
	}

	// 注册 CORS 中间件
	// 优先使用依赖注入的 CORSConfig，否则使用配置文件中的 CORS
	corsConfig := p.Config.CORS
	if p.CORSConfig != nil {
		corsConfig = *p.CORSConfig
	}
	if corsConfig.Enabled {
		app.Use(middleware.NewCORS(corsConfig))
		p.Logger.Info("CORS middleware enabled",
			zap.Strings("allow_origins", corsConfig.AllowOrigins),
			zap.Bool("allow_credentials", corsConfig.AllowCredentials),
		)
	}

	// 注册健康检查端点
	healthCheckTimeout := p.Config.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 2 * time.Second
	}
	registerHealthEndpoints(app, p.DB, healthCheckTimeout, p.Config.ExposeRuntimeStats, p.Logger)

	// 注册 Prometheus 指标端点
	metrics.RegisterMetricsEndpoint(app)

	var preforkStateMu sync.Mutex
	var preforkEnabled bool

	p.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr := fmt.Sprintf(":%d", p.Config.Port)
			if p.Config.Host != "" {
				addr = fmt.Sprintf("%s:%d", p.Config.Host, p.Config.Port)
			}

			listenConfig := buildListenConfig(p.Config.Listen)
			if p.ListenConfigCustomizer != nil {
				p.ListenConfigCustomizer(&listenConfig)
			}

			preforkStateMu.Lock()
			preforkEnabled = listenConfig.EnablePrefork
			preforkStateMu.Unlock()

			// Prefork 模式：直接使用 app.Listen，让 Fiber 处理多进程和监听器创建
			if listenConfig.EnablePrefork {
				return startPreforkServer(ctx, app, addr, listenConfig, p.Logger)
			}

			// 非 Prefork 模式：预先创建 listener 确保端口绑定成功
			return startSingleProcessServer(ctx, app, addr, listenConfig, p.Logger)
		},
		OnStop: func(ctx context.Context) error {
			preforkStateMu.Lock()
			enabled := preforkEnabled
			preforkStateMu.Unlock()

			if enabled && !fiber.IsChild() {
				// Master 进程：Fiber 会自动向子进程发送信号
				// 我们只需要调用 ShutdownWithContext，Fiber 内部会处理子进程
				p.Logger.Info("Stopping HTTP Server (prefork master)")
			} else if enabled {
				p.Logger.Info("Stopping HTTP Server (prefork child)", zap.Int("pid", os.Getpid()))
			} else {
				p.Logger.Info("Stopping HTTP Server")
			}

			return app.ShutdownWithContext(ctx)
		},
	})

	return app
}

// startPreforkServer 启动 Prefork 模式的服务器
func startPreforkServer(ctx context.Context, app *fiber.App, addr string, listenConfig fiber.ListenConfig, log *logger.Logger) error {
	if err := validatePreforkNetwork(listenConfig.ListenerNetwork); err != nil {
		return err
	}

	isChild := fiber.IsChild()
	pid := os.Getpid()

	// 注册 OnFork hook（仅在 master 进程中有效）
	if !isChild {
		app.Hooks().OnFork(func(childPID int) error {
			log.Info("Forked new child process", zap.Int("child_pid", childPID))
			return nil
		})
	}

	// 注册关闭 hooks
	app.Hooks().OnPreShutdown(func() error {
		if isChild {
			log.Debug("Pre-shutdown hook triggered (prefork child)", zap.Int("pid", pid))
		} else {
			log.Debug("Pre-shutdown hook triggered (prefork master)", zap.Int("pid", pid))
		}
		return nil
	})

	errChan := make(chan error, 1)
	readyChan := make(chan struct{})
	readyOnce := sync.Once{}

	// Prefork 子进程不会触发 OnListen hooks；用 ListenerAddrFunc 作为就绪信号
	if isChild {
		prevListenerAddrFunc := listenConfig.ListenerAddrFunc
		listenConfig.ListenerAddrFunc = func(addr net.Addr) {
			if prevListenerAddrFunc != nil {
				prevListenerAddrFunc(addr)
			}
			readyOnce.Do(func() {
				close(readyChan)
			})
		}
	}

	// 注册 OnListen hook，用于确认服务器已开始监听
	app.Hooks().OnListen(func(data fiber.ListenData) error {
		if isChild {
			log.Info("HTTP Server child process listening",
				zap.Int("pid", pid),
				zap.String("host", data.Host),
				zap.String("port", data.Port),
			)
		} else {
			log.Info("HTTP Server started in Prefork mode",
				zap.String("addr", addr),
				zap.Int("process_count", data.ProcessCount),
				zap.Ints("child_pids", data.ChildPIDs),
			)
		}
		readyOnce.Do(func() {
			close(readyChan)
		})
		return nil
	})

	go func() {
		if err := app.Listen(addr, listenConfig); err != nil {
			log.Error("HTTP Server failed", zap.Error(err), zap.Int("pid", pid))
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	// 等待服务器就绪或出错
	return waitForServerStartup(ctx, readyChan, errChan, func() {
		// 启动超时：主动关闭已启动的 app，避免 Listen goroutine 持续运行但无人管理。
		// OnStop 不会为 OnStart 失败的 hook 调用，因此必须在此处自行清理。
		_ = app.Shutdown()
	})
}

// startSingleProcessServer 启动单进程模式的服务器
func startSingleProcessServer(ctx context.Context, app *fiber.App, addr string, listenConfig fiber.ListenConfig, log *logger.Logger) error {
	listener, err := createListener(addr, listenConfig)
	if err != nil {
		log.Error("Failed to create HTTP listener", zap.Error(err), zap.String("addr", addr))
		return fmt.Errorf("failed to bind to %s: %w", addr, err)
	}

	log.Info("HTTP Server listener created successfully", zap.String("addr", addr))

	readyChan := make(chan struct{})
	errChan := make(chan error, 1)
	readyOnce := sync.Once{}

	app.Hooks().OnListen(func(data fiber.ListenData) error {
		log.Info("HTTP Server started",
			zap.String("host", data.Host),
			zap.String("port", data.Port),
		)
		readyOnce.Do(func() {
			close(readyChan)
		})
		return nil
	})

	go func() {
		if err := app.Listener(listener, listenConfig); err != nil {
			log.Error("HTTP Server failed", zap.Error(err))
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	return waitForServerStartup(ctx, readyChan, errChan, func() {
		_ = app.Shutdown()
	})
}

func waitForServerStartup(ctx context.Context, ready <-chan struct{}, errChan <-chan error, cleanup func()) error {
	select {
	case err := <-errChan:
		return err
	case <-ready:
		return nil
	case <-ctx.Done():
		if cleanup != nil {
			cleanup()
		}
		return ctx.Err()
	}
}

// buildListenConfig 根据 ListenOptions 构建 Fiber ListenConfig，并应用默认值
func buildListenConfig(opts ListenOptions) fiber.ListenConfig {
	config := fiber.ListenConfig{
		EnablePrefork:         opts.EnablePrefork,
		DisableStartupMessage: opts.DisableStartupMessage,
		EnablePrintRoutes:     opts.EnablePrintRoutes,
		CertFile:              opts.CertFile,
		CertKeyFile:           opts.CertKeyFile,
		CertClientFile:        opts.CertClientFile,
	}

	// 应用默认值
	if opts.ListenerNetwork != "" {
		config.ListenerNetwork = opts.ListenerNetwork
	} else {
		config.ListenerNetwork = "tcp4" // 默认 tcp4
	}

	if opts.ShutdownTimeout > 0 {
		config.ShutdownTimeout = opts.ShutdownTimeout
	}
	// 注意：Fiber 默认的 ShutdownTimeout 是 10s，这里不设置则使用 Fiber 的默认值

	if opts.UnixSocketFileMode > 0 {
		config.UnixSocketFileMode = os.FileMode(opts.UnixSocketFileMode)
	}
	// 注意：Fiber 默认的 UnixSocketFileMode 是 0770

	if opts.TLSMinVersion > 0 {
		config.TLSMinVersion = opts.TLSMinVersion
	}
	// 注意：Fiber 默认的 TLSMinVersion 是 tls.VersionTLS12

	return config
}

func validatePreforkNetwork(network string) error {
	switch network {
	case "tcp4", "tcp6":
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("prefork requires listener_network to be tcp4 or tcp6, got %q", network)
	}
}

/* ========================================================================
 * Health Check Endpoints
 * ========================================================================
 * /healthz - 存活探针 (Liveness Probe)
 *   - 用于 K8s 判断容器是否存活
 *   - 只要进程能响应就返回 200
 *
 * /readyz - 就绪探针 (Readiness Probe)
 *   - 用于 K8s 判断容器是否可以接收流量
 *   - 需要检查数据库等依赖是否就绪
 * ======================================================================== */

func registerHealthEndpoints(app *fiber.App, db *gorm.DB, timeout time.Duration, includeRuntimeStats bool, log *logger.Logger) {
	// 存活探针 - 简单返回 OK
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// 就绪探针 - 检查依赖
	app.Get("/readyz", func(c fiber.Ctx) error {
		checks := make(map[string]string)
		healthy := true

		// 检查数据库连接
		if db != nil {
			checkTimeout := timeout
			if checkTimeout <= 0 {
				checkTimeout = 2 * time.Second
			}
			sqlDB, err := db.DB()
			if err != nil {
				// 不把底层错误暴露给公开端点，记录日志即可
				log.Error("readyz: failed to get sql.DB", zap.Error(err))
				checks["database"] = "unhealthy"
				healthy = false
			} else {
				// 基于请求 context 派生超时，使客户端断开/服务关闭时能及时取消
				// Fiber v3: c.Context() 返回 *fasthttp.RequestCtx，实现了 context.Context
				pingCtx, cancel := context.WithTimeout(c.Context(), checkTimeout)
				defer cancel()
				if err := sqlDB.PingContext(pingCtx); err != nil {
					log.Error("readyz: database ping failed", zap.Error(err))
					checks["database"] = "unhealthy"
					healthy = false
				} else {
					checks["database"] = "ok"
				}
			}
		}

		if includeRuntimeStats {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			checks["memory_alloc_mb"] = fmt.Sprintf("%.2f", float64(m.Alloc)/1024/1024)
			checks["goroutines"] = fmt.Sprintf("%d", runtime.NumGoroutine())
		}

		status := "ok"
		statusCode := fiber.StatusOK
		if !healthy {
			status = "unhealthy"
			statusCode = fiber.StatusServiceUnavailable
		}

		return c.Status(statusCode).JSON(fiber.Map{
			"status": status,
			"time":   time.Now().Format(time.RFC3339),
			"checks": checks,
		})
	})
}
