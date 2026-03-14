package gateway

import (
	"context"
	"errors"
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
	endpoints  map[string]*http.Server
	logger     *zap.Logger
}

// NewServer は Server のコンストラクタです。
func NewServer(cfg config.ServerConfig, endpointsCfg config.EndpointsConfig, router http.Handler, logger *zap.Logger) *Server {
	endpoints := make(map[string]*http.Server, len(endpointsCfg.Services))
	for name, svc := range endpointsCfg.Services {
		endpoints[name] = &http.Server{
			Addr:              fmt.Sprintf(":%d", svc.Port),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		endpoints: endpoints,
		logger:    logger,
	}
}

// Start は goroutine で HTTP サーバーを起動します。
// net.Listen でポートを事前に確保し、Serve を goroutine で呼び出します。
// http.ErrServerClosed はエラーとして扱いません。
func (s *Server) Start() error {
	if err := s.startServer(s.httpServer); err != nil {
		return err
	}

	for name, srv := range s.endpoints {
		if err := s.startServer(srv); err != nil {
			return fmt.Errorf("endpoint %q: %w", name, err)
		}
	}

	return nil
}

func (s *Server) startServer(srv *http.Server) error {
	s.logger.Info("gateway server starting", zap.String("addr", srv.Addr))

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", srv.Addr, err)
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("gateway server error", zap.Error(err))
		}
	}()

	return nil
}

// Shutdown はサーバーをグレースフルにシャットダウンします。
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	if err := s.httpServer.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("main server: %w", err))
	}

	for name, srv := range s.endpoints {
		if err := srv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("endpoint %q: %w", name, err))
		}
	}

	return errors.Join(errs...)
}

// Addr はリッスンアドレスを返します。
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
