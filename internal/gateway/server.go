package gateway

import (
	"context"
	"fmt"
	"net/http"

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
			Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler: router,
		},
		logger: logger,
	}
}

// Start は goroutine で HTTP サーバーを起動します。
// http.ErrServerClosed はエラーとして扱いません。
func (s *Server) Start() error {
	s.logger.Info("gateway server starting", zap.String("addr", s.httpServer.Addr))

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// Shutdown はサーバーをグレースフルにシャットダウンします。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr はリッスンアドレスを返します。
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
