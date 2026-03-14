package cmd

import (
	"github.com/spf13/cobra"
	"github.com/toby1991/chatgpt-cli/automation"
	"github.com/toby1991/chatgpt-cli/driver"
)

var dumpCmd = &cobra.Command{
	Use:   "dump [bundle-id]",
	Short: "列出指定 App 窗口所有 AX 元素（诊断用）",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundleID := driver.BundleID
		if len(args) == 1 {
			bundleID = args[0]
		}
		// 先输出窗口详细诊断
		automation.DumpWindowInfo(bundleID)
		// 再输出完整 AX 树
		automation.DumpElements(bundleID)
		return nil
	},
}
