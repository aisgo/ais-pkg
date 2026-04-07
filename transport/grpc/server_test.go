package grpc

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/aisgo/ais-pkg/logger"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type hookRecorder struct {
	hooks []fx.Hook
}

func (r *hookRecorder) Append(h fx.Hook) {
	r.hooks = append(r.hooks, h)
}

func TestRecoveryInterceptor(t *testing.T) {
	log := logger.NewNop()
	interceptor := recoveryInterceptor(log)

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("boom")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error")
	}
	if st.Code() != codes.Internal {
		t.Fatalf("unexpected code: %v", st.Code())
	}
}

func TestLoggingInterceptor(t *testing.T) {
	log := logger.NewNop()
	interceptor := loggingInterceptor(log)

	expectedErr := errors.New("fail")
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewListenerMonolith(t *testing.T) {
	inProc := NewInProcListener()
	listener, err := NewListener(ListenerProviderParams{
		Config: Config{Mode: "monolith"},
		Logger: logger.NewNop(),
	}, inProc)
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	if listener != inProc.Listener {
		t.Fatalf("expected in-proc listener")
	}
}

func TestNewListenerTCP(t *testing.T) {
	inProc := NewInProcListener()
	listener, err := NewListener(ListenerProviderParams{
		Config: Config{Mode: "microservice", Port: 0},
		Logger: logger.NewNop(),
	}, inProc)
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	defer listener.Close()
}

func TestResolveServerMsgSizeLimits(t *testing.T) {
	recv, send := resolveServerMsgSizeLimits(Config{})
	if recv != defaultMsgSize || send != defaultMsgSize {
		t.Fatalf("expected default server limits, got recv=%d send=%d", recv, send)
	}

	recv, send = resolveServerMsgSizeLimits(Config{
		MaxRecvMsgSize: 32 * 1024 * 1024,
		MaxSendMsgSize: 24 * 1024 * 1024,
	})
	if recv != 32*1024*1024 || send != 24*1024*1024 {
		t.Fatalf("expected configured server limits, got recv=%d send=%d", recv, send)
	}
}

func TestResolveClientMsgSizeLimits(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantRecv int
		wantSend int
	}{
		{
			name:     "defaults to 16mb",
			cfg:      Config{},
			wantRecv: defaultMsgSize,
			wantSend: defaultMsgSize,
		},
		{
			name: "inherits server limits",
			cfg: Config{
				MaxRecvMsgSize: 48 * 1024 * 1024,
				MaxSendMsgSize: 40 * 1024 * 1024,
			},
			wantRecv: 48 * 1024 * 1024,
			wantSend: 40 * 1024 * 1024,
		},
		{
			name: "client overrides server limits",
			cfg: Config{
				MaxRecvMsgSize:       48 * 1024 * 1024,
				MaxSendMsgSize:       40 * 1024 * 1024,
				ClientMaxRecvMsgSize: 64 * 1024 * 1024,
				ClientMaxSendMsgSize: 56 * 1024 * 1024,
			},
			wantRecv: 64 * 1024 * 1024,
			wantSend: 56 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recv, send := resolveClientMsgSizeLimits(tt.cfg)
			if recv != tt.wantRecv || send != tt.wantSend {
				t.Fatalf("unexpected client limits: recv=%d send=%d", recv, send)
			}
		})
	}
}

func TestBuildTLSConfigRejectsInvalidPEM(t *testing.T) {
	dir := t.TempDir()
	caFile := dir + "/ca.pem"
	if err := os.WriteFile(caFile, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write ca file: %v", err)
	}

	if _, err := buildTLSConfig(TLSConfig{
		Enable: true,
		CAFile: caFile,
	}); err == nil {
		t.Fatalf("expected invalid pem error")
	}
}

func TestNewServerOnStartReturnsEarlyServeFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	lc := &hookRecorder{}
	_ = NewServer(ServerParams{
		Lc:       lc,
		Config:   Config{Mode: "microservice"},
		Listener: listener,
		Logger:   logger.NewNop(),
	})

	if len(lc.hooks) != 1 || lc.hooks[0].OnStart == nil {
		t.Fatalf("expected lifecycle hook to be registered")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lc.hooks[0].OnStart(ctx); err == nil {
		t.Fatalf("expected serve failure to be returned from OnStart")
	}
}
