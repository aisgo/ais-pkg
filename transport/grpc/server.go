package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/aisgo/ais-pkg/logger"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

/* ========================================================================
 * gRPC Server - 模块间通信
 * ========================================================================
 * 职责: 提供 gRPC 服务，支持 TCP 和 BufConn 模式
 * 技术: google.golang.org/grpc
 * ======================================================================== */

const bufSize = 1024 * 1024

type Config struct {
	Port                 int           `yaml:"port"`
	Mode                 string        `yaml:"mode"` // monolith or microservice
	TLS                  TLSConfig     `yaml:"tls"`
	MaxRecvMsgSize       int           `yaml:"max_recv_msg_size"`        // 服务端最大接收消息字节数，默认 16MB
	MaxSendMsgSize       int           `yaml:"max_send_msg_size"`        // 服务端最大发送消息字节数，默认 16MB
	ClientMaxRecvMsgSize int           `yaml:"client_max_recv_msg_size"` // 客户端最大接收消息字节数，默认继承 MaxRecvMsgSize 或 16MB
	ClientMaxSendMsgSize int           `yaml:"client_max_send_msg_size"` // 客户端最大发送消息字节数，默认继承 MaxSendMsgSize 或 16MB
	StartupGracePeriod   time.Duration `yaml:"startup_grace_period"`     // 启动观察窗口（等待 Serve 早期失败），默认 25ms；容器/高负载环境可适当调大
}

// TLSConfig gRPC 客户端 TLS 配置
type TLSConfig struct {
	Enable     bool   `yaml:"enable"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	ServerName string `yaml:"server_name"`
	Insecure   bool   `yaml:"insecure"` // 跳过证书验证
}

type ListenerProviderParams struct {
	fx.In
	Config Config
	Logger *logger.Logger
}

// InProcListener 是一个全局的 bufconn 监听器，仅在 Monolith 模式下使用
type InProcListener struct {
	*bufconn.Listener
}

func NewInProcListener() *InProcListener {
	return &InProcListener{Listener: bufconn.Listen(bufSize)}
}

// NewListener 创建 gRPC 监听器 (TCP 或 BufConn)
func NewListener(p ListenerProviderParams, inProc *InProcListener) (net.Listener, error) {
	if p.Config.Mode == "monolith" {
		p.Logger.Info("Using In-Memory gRPC Listener (BufConn)")
		return inProc.Listener, nil
	}

	p.Logger.Info("Using TCP gRPC Listener", zap.Int("port", p.Config.Port))
	return net.Listen("tcp", fmt.Sprintf(":%d", p.Config.Port))
}

type ServerParams struct {
	fx.In
	Lc       fx.Lifecycle
	Config   Config
	Listener net.Listener
	Logger   *logger.Logger
}

// recoveryInterceptor 创建 panic 恢复拦截器
func recoveryInterceptor(log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("gRPC panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
					zap.String("stack", string(debug.Stack())),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// loggingInterceptor 创建日志拦截器
func loggingInterceptor(log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)
		duration := time.Since(start)

		if err != nil {
			log.Warn("gRPC request failed",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else if duration > 500*time.Millisecond {
			// 记录慢请求
			log.Warn("gRPC slow request",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
			)
		}

		return resp, err
	}
}

const defaultMsgSize = 16 * 1024 * 1024 // 16MB
const serverStartupGracePeriod = 25 * time.Millisecond

func resolveServerMsgSizeLimits(cfg Config) (recv, send int) {
	recv = cfg.MaxRecvMsgSize
	if recv <= 0 {
		recv = defaultMsgSize
	}

	send = cfg.MaxSendMsgSize
	if send <= 0 {
		send = defaultMsgSize
	}

	return recv, send
}

func resolveClientMsgSizeLimits(cfg Config) (recv, send int) {
	recv = cfg.ClientMaxRecvMsgSize
	if recv <= 0 {
		recv = cfg.MaxRecvMsgSize
	}
	if recv <= 0 {
		recv = defaultMsgSize
	}

	send = cfg.ClientMaxSendMsgSize
	if send <= 0 {
		send = cfg.MaxSendMsgSize
	}
	if send <= 0 {
		send = defaultMsgSize
	}

	return recv, send
}

// NewServer 创建 gRPC Server 并管理生命周期
func NewServer(p ServerParams) *grpc.Server {
	maxRecvMsgSize, maxSendMsgSize := resolveServerMsgSizeLimits(p.Config)

	// 配置拦截器: Recovery, Logging
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor(p.Logger), // Panic 恢复
			loggingInterceptor(p.Logger),  // 日志记录
		),
		// Keepalive 配置，防止空闲连接堆积
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,  // 空闲连接最大时间
			MaxConnectionAge:      30 * time.Minute, // 连接最大生命周期
			MaxConnectionAgeGrace: 10 * time.Second, // 优雅关闭等待时间
			Time:                  30 * time.Second, // 发送 ping 的间隔
			Timeout:               10 * time.Second, // ping 超时时间
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second, // 客户端 ping 最小间隔
			PermitWithoutStream: true,             // 允许没有活跃 stream 时 ping
		}),
		// 限制最大消息大小（防止 OOM），大小通过 Config 配置
		grpc.MaxRecvMsgSize(maxRecvMsgSize),
		grpc.MaxSendMsgSize(maxSendMsgSize),
	}
	s := grpc.NewServer(opts...)

	p.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			errChan := make(chan error, 1)

			go func() {
				p.Logger.Info("Starting gRPC Server")
				if err := s.Serve(p.Listener); err != nil {
					p.Logger.Error("gRPC Server failed", zap.Error(err))
					select {
					case errChan <- err:
					default:
					}
				}
			}()

			// gRPC 没有对外暴露可靠的”开始接受连接”事件。
			// 这里优先等待 Serve 的早期失败；短暂观察窗口后再宣告启动成功，
			// 避免 Serve 立即返回错误时仍被 Fx 视为已启动。
			// 观察窗口通过 Config.StartupGracePeriod 配置（容器/高负载环境可适当调大）。
			gracePeriod := p.Config.StartupGracePeriod
			if gracePeriod <= 0 {
				gracePeriod = serverStartupGracePeriod
			}
			timer := time.NewTimer(gracePeriod)
			defer timer.Stop()

			select {
			case err := <-errChan:
				return err
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Stopping gRPC Server")
			stopped := make(chan struct{})
			go func() {
				s.GracefulStop()
				close(stopped)
			}()

			select {
			case <-stopped:
				return nil
			case <-ctx.Done():
				p.Logger.Warn("gRPC Server graceful stop timeout, forcing stop", zap.Error(ctx.Err()))
				// 强制停止后必须阻塞等待 GracefulStop goroutine 退出，
				// 避免 goroutine 泄漏（Stop 会使 GracefulStop 立即返回）
				s.Stop()
				<-stopped
				return ctx.Err()
			}
		},
	})
	return s
}

// ClientFactory 用于创建 gRPC 客户端
type ClientFactory func(target string) (*grpc.ClientConn, error)

// NewClientFactory 返回一个创建 ClientConn 的函数
// 如果是 Monolith 模式，自动使用 BufConn Dialer
func NewClientFactory(cfg Config, inProc *InProcListener) ClientFactory {
	return func(target string) (*grpc.ClientConn, error) {
		maxRecvMsgSize, maxSendMsgSize := resolveClientMsgSizeLimits(cfg)

		creds := insecure.NewCredentials()
		if cfg.Mode != "monolith" && cfg.TLS.Enable {
			tlsConfig, err := buildTLSConfig(cfg.TLS)
			if err != nil {
				return nil, err
			}
			creds = credentials.NewTLS(tlsConfig)
		}

		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(creds),
			// 添加默认超时配置
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
				grpc.MaxCallSendMsgSize(maxSendMsgSize),
			),
			// 添加连接超时配置
			grpc.WithConnectParams(grpc.ConnectParams{
				Backoff: backoff.Config{
					MaxDelay:  30 * time.Second,
					BaseDelay: 1 * time.Second,
				},
				MinConnectTimeout: 10 * time.Second,
			}),
		}

		if cfg.Mode == "monolith" {
			// 在 Monolith 模式下，忽略 target IP，直接连接 InProcListener
			opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return inProc.Dial()
			}))
			// 使用 passthrough resolver，避免默认 dns resolver 导致 "produced zero addresses"
			target = "passthrough:///bufconn"
		}

		return grpc.NewClient(target, opts...)
	}
}

func buildTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	if cfg.Insecure {
		// nolint:gosec // InsecureSkipVerify is intentionally configurable but dangerous in production.
		// WARNING: setting insecure=true disables certificate verification and is vulnerable to MITM attacks.
		// Only use this in development/testing environments.
		fmt.Println("WARNING: gRPC TLS InsecureSkipVerify=true — certificate verification is disabled. Do NOT use in production.")
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.Insecure, //nolint:gosec
	}

	if cfg.ServerName != "" {
		tlsConfig.ServerName = cfg.ServerName
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
