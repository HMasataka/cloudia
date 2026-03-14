package imds

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// Server はIMDS専用のHTTPサーバーです。
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// New は Server を生成します。
// addr にはリッスンアドレスを指定します（例: "169.254.169.254:80"、テスト時は "127.0.0.1:0"）。
func New(addr string, store state.Store, logger *zap.Logger) *Server {
	tokenStore := NewTokenStore()
	handler := NewHandler(store, tokenStore, logger)

	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		logger: logger,
	}
}

// Start は goroutine でIMDSサーバーを起動します。
func (s *Server) Start() error {
	s.logger.Info("imds server starting", zap.String("addr", s.httpServer.Addr))

	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("imds: failed to listen on %s: %w", s.httpServer.Addr, err)
	}

	// 実際にバインドされたアドレスを保存（":0" の場合に備えて）
	s.httpServer.Addr = ln.Addr().String()

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("imds server error", zap.Error(err))
		}
	}()

	return nil
}

// Shutdown はサーバーをグレースフルにシャットダウンします。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr はリッスンアドレスを返します。
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
