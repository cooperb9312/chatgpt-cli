package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ChatGPT Desktop App 已确认可用的模型（来自 AX 树探索）
var availableModels = []struct {
	Name         string
	Description  string
	ButtonPrefix string
}{
	{"GPT-5.3 旗舰", "最新旗舰模型", "GPT-5.3"},
	{"传统模型", "传统 ChatGPT 模型", "传统"},
}

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "列出所有可用的 ChatGPT 模型",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("可用模型（使用 --model <名称前缀> 指定）:")
		fmt.Println()
		for _, m := range availableModels {
			fmt.Printf("  %-24s %s\n", m.Name, m.Description)
			fmt.Printf("    使用: gpt --model %q \"query\"\n", m.ButtonPrefix)
			fmt.Println()
		}
	},
}
