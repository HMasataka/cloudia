package cli

import (
	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "cloudia",
	Short: "Local cloud service emulator",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file path")

	rootCmd.AddCommand(newStartCmd())
	rootCmd.AddCommand(newStopCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCleanupCmd())
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}
