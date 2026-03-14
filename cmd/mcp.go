package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/toby1991/chatgpt-cli/driver"
)

// searchPromptSuffix appended to every MCP query. Leave empty to disable.
const searchPromptSuffix = ``

var (
	flagMCPModel     string
	flagMCPWebSearch bool
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "启动 MCP Server (stdio transport)",
	Long: `以 MCP (Model Context Protocol) 服务器模式运行，通过 stdin/stdout 通信。

需要 ChatGPT Desktop App 正在运行，并已授权 Accessibility 权限。
UI 后端建议配合 caffeinate 防止 headless Mac 睡眠。

选项:
  --model MODEL       默认模型前缀（如: GPT-5.3, 传统）
  --web-search        默认启用网络搜索

配置示例 (OpenCode ~/.config/opencode/config.json):
  {
    "mcpServers": {
      "gpt": {
        "type": "stdio",
        "command": "gpt",
        "args": ["mcp", "--model", "GPT-5.3"]
      }
    }
  }`,
	RunE:          runMCP,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// mcpDispatcher 是 MCP server 运行期间共享的调度器
var mcpDispatcher *driver.Dispatcher

func init() {
	mcpCmd.Flags().StringVar(&flagMCPModel, "model", "",
		"默认模型前缀（如: GPT-5.3, 传统）")
	mcpCmd.Flags().BoolVar(&flagMCPWebSearch, "web-search", false,
		"默认启用网络搜索")
}

func runMCP(cmd *cobra.Command, args []string) error {
	if err := driver.EnsureAppRunning(); err != nil {
		return fmt.Errorf("ChatGPT Desktop App: %w", err)
	}

	if err := exec.Command("pgrep", "-x", "caffeinate").Run(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "[mcp] warning: caffeinate is not running — headless Mac may sleep\n")
		fmt.Fprintf(cmd.ErrOrStderr(), "[mcp] hint: run `gpt setup-caffeinate` to install a persistent LaunchAgent\n")
	}

	mcpDispatcher = &driver.Dispatcher{
		PrimaryModel: flagMCPModel,
		WebSearch:    flagMCPWebSearch,
	}

	s := server.NewMCPServer(
		"ChatGPT",
		Version,
		server.WithToolCapabilities(true),
	)

	searchTool := mcp.NewTool("search",
		mcp.WithDescription("Ask ChatGPT a question via the ChatGPT Desktop App. Returns the AI response."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The question or prompt to send to ChatGPT"),
		),
		mcp.WithString("model",
			mcp.Description("Model name prefix (e.g. GPT-5.3, 传统). Call list_models to see available models."),
		),
		mcp.WithBoolean("web_search",
			mcp.Description("Enable web search for this query (true/false)"),
		),
	)
	s.AddTool(searchTool, handleSearch)

	listModelsTool := mcp.NewTool("list_models",
		mcp.WithDescription("List all available ChatGPT models."),
	)
	s.AddTool(listModelsTool, handleListModels)

	return server.ServeStdio(s)
}

func handleListModels(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	sb.WriteString("Available ChatGPT Desktop models:\n\n")
	for _, m := range availableModels {
		sb.WriteString(fmt.Sprintf("- **%s** — %s\n  Use `model: \"%s\"` in search\n", m.Name, m.Description, m.ButtonPrefix))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	model := request.GetString("model", "")

	// web_search is a boolean parameter; -1 means use dispatcher default
	webSearchInt := -1
	// Only override if the parameter was explicitly provided
	// GetBool returns false as default — we check Params.Arguments to distinguish
	// "not provided" from "explicitly false"
	if args, ok := request.Params.Arguments.(map[string]any); ok {
		if _, exists := args["web_search"]; exists {
			if request.GetBool("web_search", false) {
				webSearchInt = 1
			} else {
				webSearchInt = 0
			}
		}
	}

	// Append prompt suffix if configured
	suffix := os.Getenv("GPT_PROMPT_SUFFIX")
	if suffix == "" {
		suffix = searchPromptSuffix
	}
	fullQuery := query
	if suffix != "" {
		fullQuery = query + "\n\n" + suffix
	}

	result, err := mcpDispatcher.Ask(fullQuery, model, webSearchInt)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("ask failed: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(result.Answer)

	return mcp.NewToolResultText(sb.String()), nil
}
