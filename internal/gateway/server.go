package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/config"
	"go.uber.org/zap"
)

// Server は HTTP サーバーを保持します。
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// NewServer は Server のコンストラクタです。
func NewServer(cfg config.ServerConfig, router http.Handler, logger *zap.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		logger: logger,
	}
}

// Start は goroutine で HTTP サーバーを起動します。
// net.Listen でポートを事前に確保し、Serve を goroutine で呼び出します。
// http.ErrServerClosed はエラーとして扱いません。
func (s *Server) Start() error {
	s.logger.Info("gateway server starting", zap.String("addr", s.httpServer.Addr))

	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.httpServer.Addr, err)
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("gateway server error", zap.Error(err))
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
