package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/logging"
)

func newCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up Cloudia-managed resources",
		RunE:  runCleanup,
	}
}

func runCleanup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	dockerClient, err := docker.NewClient(cfg.Docker, logger)
	if err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}
	defer dockerClient.Close()

	ctx := context.Background()
	n, err := dockerClient.CleanupOrphans(ctx)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("Cleaned up %d Cloudia-managed resource(s)\n", n)
	return nil
}
