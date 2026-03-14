package cli

import (
	"context"
	"fmt"
	"os"

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

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Docker is not available: %v\n", err)
		os.Exit(1)
	}
	defer dockerClient.Close()

	ctx := context.Background()
	if err := dockerClient.CleanupOrphans(ctx); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Println("Cleaned up all Cloudia-managed resources")
	return nil
}
