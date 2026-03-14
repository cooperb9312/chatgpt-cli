package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/toby1991/chatgpt-cli/driver"
	"github.com/toby1991/chatgpt-cli/output"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示 ChatGPT Desktop App 运行状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, model, err := driver.GetStatus()
		if err != nil {
			return fmt.Errorf("failed to read status: %w", err)
		}
		output.PrintStatus(status, model)
		return nil
	},
}
