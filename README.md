# gpt

> Drive your ChatGPT Desktop App as an MCP tool — your existing subscription, no extra fees.

[中文文档](README.zh.md)

`gpt` is a macOS CLI and MCP server that controls the **ChatGPT Desktop App** via Accessibility automation, turning your existing subscription into a programmable AI backend for Claude, OpenCode, and any other MCP client.

https://github.com/user-attachments/assets/bb1dcae3-e110-49c0-844e-bc4808a3ba78

---

## Quick Start

```bash
# 1. Build
git clone https://github.com/toby1991/chatgpt-cli
cd chatgpt-cli
make install          # builds → /usr/local/bin/gpt

# 2. Grant Accessibility permission (one-time)
#    System Settings → Privacy & Security → Accessibility → add your terminal app

# 3. Verify it works
gpt "what is MCP?"

# 4. Add to Claude Desktop or OpenCode (see MCP section below)
```

**Headless Mac (Mac mini via SSH)?** One extra step:

```bash
gpt setup-caffeinate   # prevents sleep + disables screen lock — run once, persists across reboots
```

---

## How It Works

| Backend | How | Cost | Speed |
|---------|-----|------|-------|
| **UI** (default) | Controls ChatGPT Desktop App via macOS AXUIElement API | Free — uses your subscription | 10–30 s |

Two operation modes:

| Mode | Command | Use case |
|------|---------|----------|
| **CLI** | `gpt "query"` | Direct terminal use, scripts, pipes |
| **MCP Server** | `gpt mcp` | Tool calls from Claude, OpenCode, etc. |

---

## CLI Usage

```bash
# Basic search
gpt "best practices for Go error handling"

# Specify model (name prefix — matches the UI model switcher)
gpt --model "GPT-5.3" "explain monads"

# Enable web search
gpt --web-search "latest news on AI"

# Pipe in, pipe out
echo "what is entropy?" | gpt
gpt "top Go CLI libraries" --json | jq '.answer'

# Quiet mode — answer only
gpt -q "capital of France"

# Interactive REPL
gpt
```

### Subcommands

```bash
gpt status               # check if ChatGPT Desktop is running
gpt models               # list available UI models
gpt dump                 # dump AX tree (diagnostic)
gpt version

# Headless Mac management
gpt setup-caffeinate     # install caffeinate LaunchAgent + disable screen lock
gpt remove-caffeinate    # uninstall
```

### Output Formats

**TTY** — colored output with spinner:
```
⠋ Waiting for response...

────────────────────────────────────────────────────
Go error handling best practices include wrapping errors...

Sources:
  [1] Effective Go — https://go.dev/doc/effective_go
  [2] Go Blog: Error handling — https://go.dev/blog/error-handling-and-go
────────────────────────────────────────────────────
```

**Pipe** — plain text, no color or spinner.

**JSON** (`--json`):
```json
{
  "answer": "Go error handling best practices...",
  "citations": [
    {"index": 1, "title": "Effective Go", "url": "https://go.dev/doc/effective_go"}
  ],
  "mode": "search",
  "model": "GPT-5.3"
}
```

---

## MCP Server

`gpt mcp` exposes ChatGPT as MCP tools over stdio.

### Tools

| Tool | Description |
|------|-------------|
| `search` | Ask ChatGPT — optional `model` and `web_search` override |
| `list_models` | List available models for the UI backend |

### Backend Configuration

```bash
gpt mcp                                    # UI only (default)
gpt mcp --model "GPT-5.3"                  # specify default model
gpt mcp --web-search                       # enable web search by default
```

### OpenClaw

[OpenClaw](https://openclaw.ai) manages MCP servers through [mcporter](https://github.com/steipete/mcporter). Add the following entry to `~/.mcporter/mcporter.json`:

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

### Claude Desktop

`~/Library/Application Support/Claude/claude_desktop_config.json`:

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

### OpenCode

`~/.config/opencode/opencode.json`:

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

### MCP Behaviors

- **Search prompt suffix**: In MCP mode, set `GPT_PROMPT_SUFFIX` environment variable to append text to each query.
- **Auto-launch**: If ChatGPT Desktop is not running, it is started automatically.
- **NavigateToHome**: Before each UI search, the app returns to the home page to start a clean thread.
- **Caffeinate check**: At startup, warns if `caffeinate` is not running and suggests `gpt setup-caffeinate`.

---

## Headless Mac Setup

Running on a Mac mini without a display (SSH / remote)?

```bash
# One command handles both sleep prevention and screen lock:
gpt setup-caffeinate
```

This does two things:
1. Installs a LaunchAgent that runs `caffeinate -dimsu` at login (auto-restarts, survives reboots)
2. Disables screen lock via `sysadminctl` (requires your login password)

**Why screen lock matters**: when a remote session (VNC/SSH) disconnects, macOS triggers the lock screen. This degrades WindowServer to "application" mode — all AX automation calls return empty/stale data silently. Disabling screen lock prevents this.

Additional belt-and-suspenders steps for headless:

```bash
sudo pmset -a displaysleep 0      # belt-and-suspenders: disable display sleep via pmset
```

### Display Settings

Two System Settings tweaks prevent the display from sleeping or locking while unattended:

| Setting | Path | Value |
|---------|------|-------|
| Turn off display when inactive | System Settings → Displays | **Never** |
| Require password after screensaver / display off | System Settings → Lock Screen | **Never** |

### Virtual Display

ChatGPT Desktop requires at least one display (physical or virtual). Options:

1. **Apple Remote Management** *(try this first)* — Enabling Remote Management in System Settings → Sharing automatically exposes a virtual framebuffer on Apple Silicon Mac mini. No extra hardware needed.
2. **HDMI Dummy Plug** — Plug a cheap HDMI dummy plug into the Mac mini's HDMI port. macOS sees it as a real display. Most reliable option if Remote Management doesn't work.
3. **BetterDisplay** — [BetterDisplay](https://github.com/waydabber/BetterDisplay) can create a software virtual display without any hardware. Useful when no physical HDMI port is available (e.g., Mac mini with only USB-C).

- **Accessibility permission**: Grant to the process running `gpt` in System Settings → Privacy & Security → Accessibility.

---

## Architecture

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
          │  chatgpt.go         │  ← UI backend (AX automation)
          └─────────┬───────────┘
                    │
          ┌─────────┴───────────┐
          │    automation/       │
          │  ax.go  (CGo)        │
          │  ax.h   (C header)   │
          │  ax.m   (Obj-C impl) │
          │  macOS AXUIElement   │
          └──────────────────────┘
```

### Ask Flow — UI Backend

```
NavigateToHome()       → click chevron-left if on results page
SetModel(model)        → open model popover, select by prefix
SetWebSearch(enable)   → toggle web search switch
SetTextAreaValue()     → type the query
Click("发送")          → submit the query
WaitForStopButton()    → wait for 停止生成 to appear
WaitForStopGone()      → wait for 停止生成 to disappear (generation done)
ReadResponseText()     → read response via AX API
extractLastResponse()  → parse response text
```

---

## Project Structure

```
chatgpt-cli/
├── main.go
├── go.mod / go.sum
├── Makefile
├── README.md / README.zh.md
├── cmd/
│   ├── root.go          # root command, flags, search dispatch
│   ├── mcp.go           # MCP server subcommand
│   ├── caffeinate.go    # setup-caffeinate / remove-caffeinate
│   ├── status.go        # check ChatGPT Desktop status
│   ├── models.go        # list available models
│   ├── dump.go          # AX tree dump (diagnostic)
│   └── version.go
├── driver/
│   ├── chatgpt.go       # UI backend: Ask, SetModel, SetWebSearch, NavigateToHome
│   └── search.go        # Dispatcher
├── automation/
│   ├── ax.go            # Go/CGo bindings
│   ├── ax.h             # C header
│   └── ax.m             # Objective-C: AXUIElement implementation
├── output/
│   └── format.go        # TTY detection, spinner, colors, JSON
└── docs/plans/          # Design documents
```

---

## Requirements

- macOS (Apple Silicon or Intel)
- Go 1.23+
- ChatGPT Desktop App (for UI backend)
- Accessibility permission granted to terminal / calling process (for UI backend)

---

## Known Limitations

| Limitation | Detail |
|------------|--------|
| macOS only | Uses AXUIElement API — no cross-platform support |
| Serial execution | One UI search at a time (ChatGPT App constraint) |
| Display required | UI backend needs at least one display (physical or virtual) |
| Accessibility permission required | Grant in System Settings → Privacy & Security → Accessibility |

---

## License

MIT
