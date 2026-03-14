package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop Cloudia",
		RunE:  runStop,
	}
}

func runStop(cmd *cobra.Command, args []string) error {
	pidPath, err := pidFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine pid file path: %w", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("Cloudia is not running")
		return nil
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		fmt.Println("Cloudia is not running")
		os.Remove(pidPath)
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("Cloudia is not running")
		os.Remove(pidPath)
		return nil
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("Cloudia is not running")
		os.Remove(pidPath)
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	fmt.Printf("Stopped Cloudia (PID: %d)\n", pid)
	return nil
}
