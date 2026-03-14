package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway/imds"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server は HTTP サーバーを保持します。
type Server struct {
	httpServer    *http.Server
	endpoints     map[string]*http.Server
	imdsServer    *imds.Server
	metricsServer *http.Server
	logger        *zap.Logger
}

// NewServer は Server のコンストラクタです。
func NewServer(cfg config.ServerConfig, endpointsCfg config.EndpointsConfig, metricsCfg config.MetricsConfig, router http.Handler, logger *zap.Logger) *Server {
	return newServer(cfg, endpointsCfg, metricsCfg, router, nil, logger)
}

// NewServerWithStore は Store を受け取り IMDS サーバーも管理する Server を返します。
func NewServerWithStore(cfg config.ServerConfig, endpointsCfg config.EndpointsConfig, metricsCfg config.MetricsConfig, router http.Handler, store state.Store, logger *zap.Logger) *Server {
	return newServer(cfg, endpointsCfg, metricsCfg, router, store, logger)
}

func newServer(cfg config.ServerConfig, endpointsCfg config.EndpointsConfig, metricsCfg config.MetricsConfig, router http.Handler, store state.Store, logger *zap.Logger) *Server {
	endpoints := make(map[string]*http.Server, len(endpointsCfg.Services))
	for name, svc := range endpointsCfg.Services {
		endpoints[name] = &http.Server{
			Addr:              fmt.Sprintf(":%d", svc.Port),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	}

	var imdsSrv *imds.Server
	if cfg.IMDS.Enabled && store != nil {
		imdsSrv = imds.New(cfg.IMDS.Address, store, logger)
	}

	var metricsSrv *http.Server
	if metricsCfg.Enabled {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", metricsCfg.Port),
			Handler:           metricsMux,
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
		endpoints:     endpoints,
		imdsServer:    imdsSrv,
		metricsServer: metricsSrv,
		logger:        logger,
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

	if s.imdsServer != nil {
		if err := s.imdsServer.Start(); err != nil {
			return fmt.Errorf("imds server: %w", err)
		}
	}

	if s.metricsServer != nil {
		if err := s.startServer(s.metricsServer); err != nil {
			return fmt.Errorf("metrics server: %w", err)
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

	if s.imdsServer != nil {
		if err := s.imdsServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("imds server: %w", err))
		}
	}

	if s.metricsServer != nil {
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metrics server: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Addr はリッスンアドレスを返します。
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
