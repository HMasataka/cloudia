package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway"
	"github.com/HMasataka/cloudia/internal/logging"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Cloudia",
		RunE:  runStart,
	}
}

func pidFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cloudia", "cloudia.pid"), nil
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	pidPath, err := pidFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine pid file path: %w", err)
	}

	pidDir := filepath.Dir(pidPath)
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("failed to create pid directory: %w", err)
	}

	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cloudia is already running: %w", err)
	}
	fmt.Fprintf(f, "%d", os.Getpid())
	if err := f.Close(); err != nil {
		logger.Warn("failed to close pid file", zap.Error(err))
	}

	defer os.Remove(pidPath)

	ctx := context.Background()

	dockerClient, err := docker.NewClient(cfg.Docker, logger)
	if err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}

	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}
	defer dockerClient.Close()

	adminHandler := admin.NewHandler(dockerClient, logger)
	router := gateway.NewRouter(adminHandler, logger, cfg.Server.Timeout)
	server := gateway.NewServer(cfg.Server, router, logger)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	return nil
}
