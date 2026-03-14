package cli

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/HMasataka/cloudia/internal/config"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Cloudia status",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	pidPath, err := pidFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine pid file path: %w", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("Cloudia is not running")
		os.Exit(1)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		fmt.Println("Cloudia is not running")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Println("Cloudia is not running")
		os.Exit(1)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Server.Port)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Println("Cloudia is not running")
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("Cloudia is running (PID: %d)\n", pid)
	return nil
}
