package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/toby1991/chatgpt-cli/automation"
	"github.com/toby1991/chatgpt-cli/driver"
	"github.com/toby1991/chatgpt-cli/output"
)

// 全局 flags
var (
	flagModel     string
	flagWebSearch bool
	flagJSON      bool
	flagQuiet     bool
)

// rootCmd 是所有命令的父节点
var rootCmd = &cobra.Command{
	Use:   "gpt [query]",
	Short: "ChatGPT AI 命令行工具",
	Long: `gpt — 通过 ChatGPT Desktop App 进行 AI 对话

用法示例:
  gpt "量子计算是什么"                   一次性查询
  gpt --model "GPT-5.3" "解释X"          指定模型
  gpt --web-search "今日新闻"             启用网络搜索
  echo "query" | gpt                     从 stdin 读取
  gpt "query" --json | jq '.answer'      JSON 输出
  gpt                                    进入交互式对话

注意：首次运行需要在「系统设置 → 隐私与安全性 → 辅助功能」中授权终端应用。`,
	RunE:          runSearch,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute 是程序入口，由 main.go 调用
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "",
		"模型名称前缀, 如: GPT-5.3, 传统")
	rootCmd.PersistentFlags().BoolVar(&flagWebSearch, "web-search", false,
		"启用 ChatGPT 网络搜索")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false,
		"以 JSON 格式输出结果")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false,
		"只输出答案正文，不显示元数据")

	// 注册子命令
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(dumpCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(setupCaffeinateCmd)
	rootCmd.AddCommand(removeCaffeinateCmd)
}

// runSearch 处理查询逻辑：stdin / positional args / 交互式 REPL
func runSearch(cmd *cobra.Command, args []string) error {
	// 检查 Accessibility 权限
	if !automation.IsTrusted() {
		_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Run()
		return fmt.Errorf(
			"缺少 Accessibility 权限\n已尝试打开「系统设置 → 隐私与安全性 → 辅助功能」\n请在列表中勾选运行本工具的终端应用，然后重新运行")
	}

	query, isREPL, err := resolveQuery(args)
	if err != nil {
		return err
	}

	if isREPL {
		return runREPL()
	}

	return doSearch(query)
}

// resolveQuery 按优先级确定查询字符串：
//  1. 命令行 positional args
//  2. stdin pipe
//  3. 无输入 → 进入 REPL
func resolveQuery(args []string) (query string, isREPL bool, err error) {
	if len(args) > 0 {
		return strings.Join(args, " "), false, nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		q := strings.TrimSpace(sb.String())
		if q == "" {
			return "", false, fmt.Errorf("stdin 为空")
		}
		return q, false, nil
	}

	return "", true, nil
}

// doSearch 执行一次查询
func doSearch(query string) error {
	// Navigate to new chat first
	if err := driver.NavigateToHome(); err != nil {
		return fmt.Errorf("navigate to home failed: %w", err)
	}

	// Optionally set model
	if flagModel != "" {
		if err := driver.SetModel(flagModel); err != nil {
			return fmt.Errorf("set model failed: %w", err)
		}
	}

	// Optionally enable web search
	if flagWebSearch {
		if err := driver.SetWebSearch(true); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] web search not available: %v\n", err)
		}
	}

	spin := output.NewSpinner("正在询问 ChatGPT...")
	spin.Start()

	result, err := driver.Ask(query)
	spin.Stop()

	if err != nil {
		return fmt.Errorf("ask failed: %w", err)
	}

	output.PrintResult(result, flagJSON, flagQuiet)
	return nil
}

// runREPL 交互式多轮对话模式
func runREPL() error {
	if output.IsTerminal {
		fmt.Println("ChatGPT 交互模式  (输入 'exit' 或 Ctrl+C 退出)")
		fmt.Println()
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if output.IsTerminal {
			fmt.Print(output.Prompt())
		}

		if !scanner.Scan() {
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}
		if query == "exit" || query == "quit" || query == "q" {
			break
		}

		if err := doSearch(query); err != nil {
			output.PrintError(err)
		}

		if output.IsTerminal {
			fmt.Println()
		}
	}

	return nil
}
