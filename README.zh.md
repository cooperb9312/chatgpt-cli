# gpt

> 把你的 ChatGPT Desktop App 变成 MCP 工具——使用你现有的订阅，无需额外费用。

[English](README.md)

`gpt` 是一个 macOS CLI 工具兼 MCP 服务，通过 Accessibility 自动化直接驱动 **ChatGPT Desktop App**，将你现有的订阅变成 Claude、OpenCode 等任意 MCP 客户端可调用的 AI 后端。

[https://github.com/user-attachments/assets/b4b4fc53-a6bd-4778-8f25-2946c61f7860](https://github.com/user-attachments/assets/bb1dcae3-e110-49c0-844e-bc4808a3ba78)

---

## 快速上手

```bash
# 1. 编译安装
git clone https://github.com/toby1991/chatgpt-cli
cd chatgpt-cli
make install          # 编译 → /usr/local/bin/gpt

# 2. 授权 Accessibility（一次性）
#    系统设置 → 隐私与安全性 → 辅助功能 → 添加你的终端 App

# 3. 验证是否正常工作
gpt "什么是 MCP？"

# 4. 配置到 Claude Desktop 或 OpenCode（见下方 MCP 章节）
```

**Headless Mac（Mac mini 通过 SSH 访问）？** 额外一步：

```bash
gpt setup-caffeinate   # 防休眠 + 关闭锁屏——执行一次，重启后自动生效
```

---

## 工作原理

| 后端 | 方式 | 费用 | 速度 |
|------|------|------|------|
| **UI**（默认） | 通过 macOS AXUIElement API 控制 ChatGPT Desktop App | 免费——使用你的订阅 | 10–30 秒 |

两种运行模式：

| 模式 | 命令 | 场景 |
|------|------|------|
| **CLI** | `gpt "query"` | 终端直接使用、脚本、管道 |
| **MCP Server** | `gpt mcp` | Claude、OpenCode 等发起的工具调用 |

---

## CLI 使用

```bash
# 基础搜索
gpt "Go 错误处理最佳实践"

# 指定模型（名称前缀匹配 UI 模型切换器）
gpt --model "GPT-5.3" "解释 Monad"

# 启用联网搜索
gpt --web-search "AI 最新新闻"

# 管道输入/输出
echo "什么是熵？" | gpt
gpt "最好的 Go CLI 框架" --json | jq '.answer'

# 静默模式——只输出答案
gpt -q "法国首都"

# 交互式 REPL
gpt
```

### 子命令

```bash
gpt status               # 检查 ChatGPT Desktop 是否运行中
gpt models               # 列出可用的 UI 模型
gpt dump                 # 导出 AX 树（诊断用）
gpt version

# Headless Mac 管理
gpt setup-caffeinate     # 安装 caffeinate LaunchAgent + 关闭锁屏
gpt remove-caffeinate    # 卸载
```

### 输出格式

**TTY 模式**——带颜色和 spinner：
```
⠋ 等待响应中...

────────────────────────────────────────────────────
Go 错误处理最佳实践包括对错误进行包装...

来源：
  [1] Effective Go — https://go.dev/doc/effective_go
  [2] Go Blog: 错误处理 — https://go.dev/blog/error-handling-and-go
────────────────────────────────────────────────────
```

**Pipe 模式**——纯文本，无颜色，无 spinner。

**JSON 模式**（`--json`）：
```json
{
  "answer": "Go 错误处理最佳实践...",
  "citations": [
    {"index": 1, "title": "Effective Go", "url": "https://go.dev/doc/effective_go"}
  ],
  "mode": "search",
  "model": "GPT-5.3"
}
```

---

## MCP Server

`gpt mcp` 通过 stdio 将 ChatGPT 暴露为 MCP 工具。

### 工具列表

| 工具 | 说明 |
|------|------|
| `search` | 询问 ChatGPT——可覆盖 `model` 和 `web_search` |
| `list_models` | 列出 UI 后端的可用模型 |

### 后端配置

```bash
gpt mcp                                    # 仅 UI（默认）
gpt mcp --model "GPT-5.3"                  # 指定默认模型
gpt mcp --web-search                       # 默认启用联网搜索
```

### 配置 OpenClaw

[OpenClaw](https://openclaw.ai) 通过 [mcporter](https://github.com/steipete/mcporter) 管理 MCP 服务器。将以下配置添加到 `~/.mcporter/mcporter.json`：

```json
{
  "mcpServers": {
    "gpt": {
      "command": "/usr/local/bin/gpt",
      "args": ["mcp", "--model", "GPT-5.3"],
      "env": {
        "GPT_PROMPT_SUFFIX": ""
      }
    }
  }
}
```

### 配置 Claude Desktop

`~/Library/Application Support/Claude/claude_desktop_config.json`：

```json
{
  "mcpServers": {
    "chatgpt": {
      "type": "stdio",
      "command": "/usr/local/bin/gpt",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

### 配置 OpenCode

`~/.config/opencode/opencode.json`：

```json
{
  "mcp": {
    "chatgpt": {
      "type": "stdio",
      "command": "/usr/local/bin/gpt",
      "args": ["mcp", "--model", "GPT-5.3"],
      "env": {}
    }
  }
}
```

### MCP 特有行为

- **搜索提示后缀**：在 MCP 模式下，设置 `GPT_PROMPT_SUFFIX` 环境变量可在每次查询时附加文本。
- **自动启动**：如果 ChatGPT Desktop 未运行，会自动启动。
- **返回首页**：每次 UI 搜索前，App 会自动返回首页，确保新线程、干净状态。
- **Caffeinate 检查**：使用 UI 后端时，启动时检查 `caffeinate` 是否运行，未运行则提示执行 `gpt setup-caffeinate`。

---

## Headless Mac 配置

在无显示器的 Mac mini 上通过 SSH 运行？

```bash
# 一条命令搞定防休眠 + 关闭锁屏：
gpt setup-caffeinate
```

这条命令做了两件事：
1. 安装 LaunchAgent，登录时自动运行 `caffeinate -dimsu`（自动重启，重启后持续生效）
2. 通过 `sysadminctl` 关闭锁屏（需要输入你的登录密码）

**为什么锁屏很关键**：远程会话（VNC/SSH）断开时，macOS 会触发锁屏，将 WindowServer 降级为「应用程序」模式——所有 AX 自动化调用都会静默返回空数据或过期数据。关闭锁屏可以防止这种情况。

额外的加固步骤：

```bash
sudo pmset -a displaysleep 0      # 通过 pmset 关闭显示器休眠（双重保险）
```

### 显示器设置

在「系统设置」中调整以下两项，防止显示器在无人值守时休眠或锁屏：

| 设置项 | 路径 | 值 |
|--------|------|----|
| 非活跃时关闭显示器 | 系统设置 → 显示器 | **永不** |
| 屏幕保护程序/显示器关闭后需要密码 | 系统设置 → 锁定屏幕 | **永不** |

### 虚拟显示器

ChatGPT Desktop 需要至少一个显示器（物理或虚拟）。可选方案：

1. **Apple 远程管理** *(优先尝试)* — 在「系统设置 → 共享」中启用「远程管理」后，Apple Silicon Mac mini 会自动创建虚拟帧缓冲，无需额外硬件。
2. **HDMI 假负载插头** — 将廉价的 HDMI 假负载插头接入 Mac mini 的 HDMI 口，macOS 会将其识别为真实显示器。在远程管理方案不奏效时，这是最可靠的选择。
3. **BetterDisplay** — [BetterDisplay](https://github.com/waydabber/BetterDisplay) 可以纯软件创建虚拟显示器，无需任何硬件，适用于没有 HDMI 口的 Mac mini（如仅有 USB-C 接口的型号）。

- **Accessibility 权限**：在「系统设置 → 隐私与安全性 → 辅助功能」中授权运行 `gpt` 的进程。

---

## 架构

```
┌─────────────────────────────────────────────────┐
│                    cmd/                          │
│  root.go  mcp.go  caffeinate.go  ...            │
│            Cobra CLI + MCP Server                │
└───────────────────┬─────────────────────────────┘
                    │
          ┌─────────┴──────────┐
          │      driver/        │
          │  search.go          │  ← Dispatcher
          │  chatgpt.go         │  ← UI 后端（AX 自动化）
          └─────────┬───────────┘
                    │
          ┌─────────┴───────────┐
          │    automation/       │
          │  ax.go  （CGo）      │
          │  ax.h   （C 头文件） │
          │  ax.m   （ObjC 实现）│
          │  macOS AXUIElement   │
          └──────────────────────┘
```

### 询问流程——UI 后端

```
NavigateToHome()       → 如果在结果页则点击 chevron-left 返回首页
SetModel(model)        → 打开模型弹窗，按前缀选中
SetWebSearch(enable)   → 切换联网搜索开关
SetTextAreaValue()     → 输入查询内容
Click("发送")          → 提交查询
WaitForStopButton()    → 等待 停止生成 按钮出现
WaitForStopGone()      → 等待 停止生成 按钮消失（生成完成）
ReadResponseText()     → 通过 AX API 读取回复
extractLastResponse()  → 解析回复文本
```

---

## 项目结构

```
chatgpt-cli/
├── main.go
├── go.mod / go.sum
├── Makefile
├── README.md / README.zh.md
├── cmd/
│   ├── root.go          # 根命令、flags、搜索分发
│   ├── mcp.go           # MCP Server 子命令
│   ├── caffeinate.go    # setup-caffeinate / remove-caffeinate
│   ├── status.go        # 检查 ChatGPT Desktop 状态
│   ├── models.go        # 列出可用模型
│   ├── dump.go          # AX 树导出（诊断）
│   └── version.go
├── driver/
│   ├── chatgpt.go       # UI 后端：Ask、SetModel、SetWebSearch、NavigateToHome
│   └── search.go        # Dispatcher
├── automation/
│   ├── ax.go            # Go/CGo 绑定
│   ├── ax.h             # C 头文件
│   └── ax.m             # Objective-C：AXUIElement 实现
├── output/
│   └── format.go        # TTY 检测、spinner、颜色、JSON
└── docs/plans/          # 设计文档
```

---

## 环境要求

- macOS（Apple Silicon 或 Intel）
- Go 1.23+
- ChatGPT Desktop App（UI 后端必需）
- 已授权 Accessibility 权限（UI 后端必需）

---

## 已知限制

| 限制 | 说明 |
|------|------|
| 仅 macOS | 使用 AXUIElement API，不支持跨平台 |
| UI 后端串行 | 每次只能执行一个搜索（ChatGPT App 本身的限制） |
| 需要显示器 | UI 后端至少需要一个物理或虚拟显示器 |
| 需要 Accessibility 权限 | 在「系统设置 → 隐私与安全性 → 辅助功能」中授权 |

---

## License

MIT
