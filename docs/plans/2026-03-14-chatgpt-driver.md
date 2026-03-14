# ChatGPT Desktop Driver Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform this repo from a Perplexity CLI tool into a full `gpt` CLI + MCP tool that drives the **ChatGPT Desktop App** (`com.openai.chat`) via macOS Accessibility API automation.

**Architecture:** Single-backend (UI only — no REST API), same AX automation layer (`automation/`), replace `driver/perplexity.go` with `driver/chatgpt.go`, keep `driver/search.go` as a thin dispatcher wrapper. The `automation/` package stays 100% app-agnostic; the `driver/` package holds all ChatGPT-specific logic.

**Tech Stack:** Go 1.23, cobra CLI, mcp-go MCP server, CGo + Objective-C AX API (ApplicationServices framework), macOS 14+

**AX Element Map (confirmed via live dump of com.openai.chat):**

| UI Element | AX Description | Type |
|---|---|---|
| Model selector button | `ChatGPT` | `AXButton` in `AXToolbar` |
| Model: GPT-5.3 flagship | `GPT-5.3、旗舰模型` | `AXButton` in popover |
| Traditional model | `传统模型` | `AXButton` in popover |
| Temporary chat toggle | `临时聊天` | `AXCheckBox` in popover |
| Web search toggle | `搜索` | `AXButton` in input area |
| Text input | first `AXTextArea` in window | `AXTextArea`, set via `AXUIElementSetAttributeValue` |
| Send button | `发送` | `AXButton` |
| New chat | `新聊天` | `AXButton` in toolbar |
| Generation indicator | `停止生成` | `AXButton` (present=generating, absent=done) |
| Response text | text in `kAXDescriptionAttribute` | `AXStaticText` elements |

---

## Task 1: Rename go module + Makefile

**Files:**
- Modify: `go.mod`
- Modify: `Makefile`

**Step 1: Update go.mod module path**

Change:
```
module github.com/toby1991/pplx-cli
```
To:
```
module github.com/toby1991/chatgpt-cli
```

**Step 2: Update Makefile binary name**

Change `BINARY := pplx` → `BINARY := gpt`
Change `install` target paths from `/usr/local/bin/pplx` → `/usr/local/bin/gpt`
Remove `test-search` and `test-json` targets (Perplexity-specific).

**Step 3: Bulk-replace all import paths in Go source files**

Run:
```bash
find . -name "*.go" | xargs sed -i '' 's|github.com/toby1991/pplx-cli|github.com/toby1991/chatgpt-cli|g'
```

**Step 4: Verify no old import path remains**

Run:
```bash
grep -r "pplx-cli" --include="*.go" .
```
Expected: no output.

---

## Task 2: Add ax_set_textarea_value + ax_read_response_text to automation/ax.h

**Files:**
- Modify: `automation/ax.h`

**Step 1: Add declarations at end of ax.h (before `#endif`)**

```c
// Set the value of the first AXTextArea in the app window.
// Returns 0 on success, -1 on failure.
int ax_set_textarea_value(const char *bundle_id, const char *text);

// Read the last AI response text from AXStaticText elements via kAXDescriptionAttribute.
// Returns malloc'd C string (caller must free), NULL if nothing found.
// For Electron-based apps (ChatGPT), response text is in description, not value.
char* ax_read_response_text(const char *bundle_id);
```

---

## Task 3: Implement ax_set_textarea_value + ax_read_response_text in automation/ax.m

**Files:**
- Modify: `automation/ax.m`

**Step 1: Add ax_set_textarea_value**

Add this function before the final closing comment in ax.m. It finds the first AXTextArea in the window and sets its value attribute:

```objc
int ax_set_textarea_value(const char *bundle_id, const char *text) {
    pid_t pid = find_pid_by_bundle_id(bundle_id);
    if (pid < 0) return -1;

    AXUIElementRef app = AXUIElementCreateApplication(pid);
    if (!app) return -1;

    // Get the focused window or first window
    CFArrayRef windows = NULL;
    AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, (CFTypeRef *)&windows);
    if (!windows || CFArrayGetCount(windows) == 0) {
        CFRelease(app);
        return -1;
    }
    AXUIElementRef win = (AXUIElementRef)CFArrayGetValueAtIndex(windows, 0);

    // Recursive DFS to find first AXTextArea
    __block AXUIElementRef found = NULL;
    // Use a helper stack-based search
    NSMutableArray *stack = [NSMutableArray arrayWithObject:(__bridge id)win];
    while (stack.count > 0 && !found) {
        AXUIElementRef elem = (__bridge AXUIElementRef)[stack lastObject];
        [stack removeLastObject];

        CFStringRef role = NULL;
        AXUIElementCopyAttributeValue(elem, kAXRoleAttribute, (CFTypeRef *)&role);
        if (role) {
            if (CFStringCompare(role, kAXTextAreaRole, 0) == kCFCompareEqualTo) {
                found = elem;
                CFRelease(role);
                break;
            }
            CFRelease(role);
        }

        CFArrayRef children = NULL;
        AXUIElementCopyAttributeValue(elem, kAXChildrenAttribute, (CFTypeRef *)&children);
        if (children) {
            CFIndex count = CFArrayGetCount(children);
            for (CFIndex i = 0; i < count; i++) {
                AXUIElementRef child = (AXUIElementRef)CFArrayGetValueAtIndex(children, i);
                [stack addObject:(__bridge id)child];
            }
            CFRelease(children);
        }
    }

    if (!found) {
        CFRelease(windows);
        CFRelease(app);
        return -1;
    }

    NSString *nsText = [NSString stringWithUTF8String:text];
    AXError err = AXUIElementSetAttributeValue(found, kAXValueAttribute, (__bridge CFTypeRef)nsText);

    CFRelease(windows);
    CFRelease(app);
    return (err == kAXErrorSuccess) ? 0 : -1;
}
```

**Step 2: Add ax_read_response_text**

This function collects all `AXStaticText` elements whose `kAXDescriptionAttribute` is non-empty and non-trivial (len > 10), and returns the last one (which is the AI's latest response):

```objc
char* ax_read_response_text(const char *bundle_id) {
    pid_t pid = find_pid_by_bundle_id(bundle_id);
    if (pid < 0) return NULL;

    AXUIElementRef app = AXUIElementCreateApplication(pid);
    if (!app) return NULL;

    CFArrayRef windows = NULL;
    AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, (CFTypeRef *)&windows);
    if (!windows || CFArrayGetCount(windows) == 0) {
        CFRelease(app);
        return NULL;
    }
    AXUIElementRef win = (AXUIElementRef)CFArrayGetValueAtIndex(windows, 0);

    NSMutableArray<NSString *> *texts = [NSMutableArray array];
    NSMutableArray *stack = [NSMutableArray arrayWithObject:(__bridge id)win];

    while (stack.count > 0) {
        AXUIElementRef elem = (__bridge AXUIElementRef)[stack lastObject];
        [stack removeLastObject];

        CFStringRef role = NULL;
        AXUIElementCopyAttributeValue(elem, kAXRoleAttribute, (CFTypeRef *)&role);
        if (role) {
            BOOL isStaticText = (CFStringCompare(role, kAXStaticTextRole, 0) == kCFCompareEqualTo);
            CFRelease(role);

            if (isStaticText) {
                // ChatGPT (Electron) stores text in kAXDescriptionAttribute
                CFStringRef desc = NULL;
                AXUIElementCopyAttributeValue(elem, kAXDescriptionAttribute, (CFTypeRef *)&desc);
                if (desc) {
                    NSString *s = (__bridge_transfer NSString *)desc;
                    // Filter: must be substantial text (not UI labels like "发送", "新聊天")
                    if (s.length > 20) {
                        [texts addObject:s];
                    }
                }
                // Don't recurse into static text children
                continue;
            }
        }

        CFArrayRef children = NULL;
        AXUIElementCopyAttributeValue(elem, kAXChildrenAttribute, (CFTypeRef *)&children);
        if (children) {
            CFIndex count = CFArrayGetCount(children);
            for (CFIndex i = 0; i < count; i++) {
                AXUIElementRef child = (AXUIElementRef)CFArrayGetValueAtIndex(children, i);
                [stack addObject:(__bridge id)child];
            }
            CFRelease(children);
        }
    }

    CFRelease(windows);
    CFRelease(app);

    if (texts.count == 0) return NULL;

    // Return all texts joined by double newline (full conversation visible)
    // Caller likely wants only last response — joining gives them everything to parse
    NSString *joined = [texts componentsJoinedByString:@"\n\n"];
    return strdup([joined UTF8String]);
}
```

> **Note on filtering:** The threshold of 20 chars filters out toolbar labels. The last `AXStaticText` with substantial content is typically the latest AI response. If we want only the last response, the driver layer (Go) slices the last element from the array.

---

## Task 4: Add Go wrappers in automation/ax.go

**Files:**
- Modify: `automation/ax.go`

**Step 1: Add SetTextAreaValue wrapper**

```go
// SetTextAreaValue sets the text content of the first AXTextArea in the App window.
// This works for Electron-based apps (ChatGPT) that expose their input via AX.
func SetTextAreaValue(bundleID, text string) error {
    cBundleID := C.CString(bundleID)
    defer C.free(unsafe.Pointer(cBundleID))
    cText := C.CString(text)
    defer C.free(unsafe.Pointer(cText))
    if C.ax_set_textarea_value(cBundleID, cText) != 0 {
        return fmt.Errorf("failed to set textarea value")
    }
    return nil
}
```

**Step 2: Add ReadResponseText wrapper**

```go
// ReadResponseText reads AI response text from AXStaticText elements via kAXDescriptionAttribute.
// ChatGPT Desktop (Electron) stores response text in description, not value.
// Returns the collected text or an error if nothing found.
func ReadResponseText(bundleID string) (string, error) {
    cBundleID := C.CString(bundleID)
    defer C.free(unsafe.Pointer(cBundleID))
    cStr := C.ax_read_response_text(cBundleID)
    if cStr == nil {
        return "", fmt.Errorf("no response text found in AX tree")
    }
    defer C.free(unsafe.Pointer(cStr))
    return C.GoString(cStr), nil
}
```

---

## Task 5: Create driver/chatgpt.go

**Files:**
- Create: `driver/chatgpt.go`

This is the core of the rewrite. It replaces `driver/perplexity.go`.

```go
package driver

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
    "time"

    "github.com/toby1991/chatgpt-cli/automation"
)

const (
    BundleID = "com.openai.chat"

    // Timeouts
    askTimeout  = 300 * time.Second
    askPoll     = 500 * time.Millisecond
    windowTimeout = 10 * time.Second
    windowPoll    = 500 * time.Millisecond

    // UI delays
    uiDelay      = 300 * time.Millisecond
    popoverDelay = 400 * time.Millisecond

    // Generation detection
    generatingIndicator = "停止生成"

    // Stability detection: N consecutive same-length readings = done
    stableRequired = 3
    minContentLen  = 1
    noActivityTimeout = 30 * time.Second
)

// AskResult holds the ChatGPT response
type AskResult struct {
    Answer string `json:"answer"`
    Model  string `json:"model"`
}

// EnsureAppRunning ensures ChatGPT Desktop App is running.
func EnsureAppRunning() error {
    if err := automation.ActivateApp(BundleID); err == nil {
        return nil
    }
    if err := exec.Command("open", "-b", BundleID).Run(); err != nil {
        return fmt.Errorf("failed to launch ChatGPT: %w", err)
    }
    deadline := time.Now().Add(windowTimeout)
    for time.Now().Before(deadline) {
        if err := automation.ActivateApp(BundleID); err == nil {
            break
        }
        time.Sleep(windowPoll)
    }
    return nil
}

// ensureWindow activates app and waits for window
func ensureWindow() error {
    if err := EnsureAppRunning(); err != nil {
        return err
    }
    if err := automation.WaitForWindow(BundleID, windowTimeout, windowPoll); err != nil {
        return fmt.Errorf("ChatGPT window not available: %w", err)
    }
    return nil
}

// NavigateToHome clicks "新聊天" to start fresh.
// For ChatGPT, we always start a new chat to avoid appending to old conversations.
func NavigateToHome() error {
    if err := ensureWindow(); err != nil {
        return err
    }
    // Dismiss any open popover first
    if automation.HasPopover(BundleID) {
        automation.PressEscape()
        time.Sleep(uiDelay)
    }
    btn, err := automation.FindButton(BundleID, "新聊天")
    if err != nil {
        return fmt.Errorf("新聊天 button not found: %w", err)
    }
    defer btn.Release()
    if err := automation.Click(btn); err != nil {
        return fmt.Errorf("failed to click 新聊天: %w", err)
    }
    time.Sleep(uiDelay)
    return nil
}

// SetModel opens the model selector popover and clicks the model by name prefix.
// modelName: prefix of the model button's AXDescription, e.g. "GPT-5.3" or "传统"
func SetModel(modelName string) error {
    if err := ensureWindow(); err != nil {
        return err
    }
    // The model selector button has AXDescription "ChatGPT" in the toolbar
    modelBtn, err := automation.FindButton(BundleID, "ChatGPT")
    if err != nil {
        return fmt.Errorf("model button (ChatGPT) not found: %w", err)
    }
    defer modelBtn.Release()
    if err := automation.Click(modelBtn); err != nil {
        return fmt.Errorf("failed to open model popover: %w", err)
    }
    time.Sleep(popoverDelay)

    targetBtn, err := automation.FindButtonPrefix(BundleID, modelName)
    if err != nil {
        return fmt.Errorf("model %q not found in popover: %w", modelName, err)
    }
    defer targetBtn.Release()
    if err := automation.Click(targetBtn); err != nil {
        return fmt.Errorf("failed to select model: %w", err)
    }
    time.Sleep(uiDelay)
    return nil
}

// SetWebSearch enables or disables web search by toggling the 搜索 button.
// The 搜索 button is a toggle in the input area — click once to enable, click again to disable.
// We check current state by looking for whether it is already in the right state.
// Note: ChatGPT's web search button doesn't have a clear "active" AX attribute we can read,
// so we use a simple: always disable first (if enabled), then enable if requested.
func SetWebSearch(enable bool) error {
    if err := ensureWindow(); err != nil {
        return err
    }
    // The 搜索 button presence indicates web search is available; its state is harder to read
    // via AX. For simplicity: we check if button exists (it always does in standard mode),
    // and rely on the caller to only call SetWebSearch(true) when needed.
    // This is a best-effort toggle — if already in the desired state, clicking is idempotent
    // only if the user hasn't changed it manually.
    searchBtn, err := automation.FindButton(BundleID, "搜索")
    if err != nil {
        // Web search button not found — model may not support it
        if enable {
            return fmt.Errorf("web search button not found (model may not support web search)")
        }
        return nil // not found and we don't need it
    }
    defer searchBtn.Release()

    if enable {
        if err := automation.Click(searchBtn); err != nil {
            return fmt.Errorf("failed to enable web search: %w", err)
        }
        time.Sleep(uiDelay)
    }
    return nil
}

// Ask sends a query to ChatGPT Desktop App and waits for the response.
//
// Flow:
//  1. Set text in AXTextArea via AXUIElementSetAttributeValue
//  2. Click 发送 button
//  3. Wait for 停止生成 to appear then disappear (generation complete)
//  4. Read response text via ax_read_response_text (kAXDescriptionAttribute)
func Ask(query string) (*AskResult, error) {
    // Set text in input area
    if err := automation.SetTextAreaValue(BundleID, query); err != nil {
        return nil, fmt.Errorf("failed to set query text: %w", err)
    }
    time.Sleep(200 * time.Millisecond)

    // Click 发送
    sendBtn, err := automation.FindButton(BundleID, "发送")
    if err != nil {
        return nil, fmt.Errorf("发送 button not found: %w", err)
    }
    defer sendBtn.Release()
    if err := automation.Click(sendBtn); err != nil {
        return nil, fmt.Errorf("failed to click 发送: %w", err)
    }

    // Wait for generation to complete
    if err := waitForGenerationComplete(BundleID, askTimeout, askPoll); err != nil {
        return nil, fmt.Errorf("ask timed out: %w", err)
    }

    // Read response
    text, err := automation.ReadResponseText(BundleID)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    // Extract only the last response (the one after the last human message)
    answer := extractLastResponse(text, query)

    return &AskResult{
        Answer: answer,
        Model:  "", // ChatGPT Desktop doesn't expose selected model via AX
    }, nil
}

// waitForGenerationComplete polls until 停止生成 disappears (generation done).
//
// Strategy:
//  1. Wait for 停止生成 to appear (confirms generation started)
//  2. Then wait for it to disappear (confirms generation complete)
//  3. Fallback: if 停止生成 never appears within 5s, assume generation already done
func waitForGenerationComplete(bundleID string, timeout, poll time.Duration) error {
    deadline := time.Now().Add(timeout)

    // Phase 1: wait for generation to START (停止生成 appears)
    startDeadline := time.Now().Add(5 * time.Second)
    generationStarted := false
    for time.Now().Before(startDeadline) {
        if automation.HasButton(bundleID, generatingIndicator) {
            generationStarted = true
            fmt.Fprintf(os.Stderr, "[wait] generation started (停止生成 appeared)\n")
            break
        }
        time.Sleep(100 * time.Millisecond)
    }
    if !generationStarted {
        fmt.Fprintf(os.Stderr, "[wait] 停止生成 never appeared, assuming immediate response\n")
    }

    // Phase 2: wait for generation to COMPLETE (停止生成 disappears)
    for time.Now().Before(deadline) {
        if !automation.HasButton(bundleID, generatingIndicator) {
            fmt.Fprintf(os.Stderr, "[wait] generation complete (停止生成 gone)\n")
            // Extra settle time for AX tree to fully update
            time.Sleep(500 * time.Millisecond)
            return nil
        }
        time.Sleep(poll)
    }

    return fmt.Errorf("timed out waiting for generation to complete (waited %v)", timeout)
}

// extractLastResponse attempts to isolate the AI's latest reply from the full response text.
// Since ax_read_response_text returns all visible AXStaticText joined by "\n\n",
// we try to find the last substantial block that comes after the user query echo.
func extractLastResponse(fullText, query string) string {
    // If the full text contains the query, take everything after it
    // (the AI response follows the user message in the AX tree)
    if idx := strings.LastIndex(fullText, query); idx != -1 {
        after := strings.TrimSpace(fullText[idx+len(query):])
        if after != "" {
            return after
        }
    }
    // Fallback: return all text
    return strings.TrimSpace(fullText)
}

// GetStatus returns a short status string for ChatGPT Desktop.
// Since ChatGPT Desktop doesn't expose model/mode via UserDefaults in a useful way,
// we return a simple "running" or "not running" status.
func GetStatus() (status, model string, err error) {
    if automation.HasButton(BundleID, "新聊天") {
        return "running", "", nil
    }
    return "not running", "", nil
}
```

---

## Task 6: Simplify driver/search.go

**Files:**
- Modify: `driver/search.go`

Remove API backend support entirely. ChatGPT has no Sonar-equivalent REST API we need. The dispatcher becomes a thin wrapper that just calls the UI backend.

```go
package driver

import "fmt"

// Dispatcher manages the search/ask backend.
// For ChatGPT CLI, only the UI backend is supported.
type Dispatcher struct {
    PrimaryModel string // default model (empty = don't change)
    WebSearch    bool   // default web search setting
}

// Ask sends a query via the ChatGPT Desktop UI.
// requestModel: per-request model override (empty = use dispatcher default)
// requestWebSearch: per-request web search override (-1 = use dispatcher default, 0 = off, 1 = on)
func (d *Dispatcher) Ask(query, requestModel string, requestWebSearch int) (*AskResult, error) {
    model := d.PrimaryModel
    if requestModel != "" {
        model = requestModel
    }

    if err := NavigateToHome(); err != nil {
        return nil, fmt.Errorf("navigate to home: %w", err)
    }
    if model != "" {
        if err := SetModel(model); err != nil {
            return nil, fmt.Errorf("set model: %w", err)
        }
    }

    webSearch := d.WebSearch
    if requestWebSearch == 1 {
        webSearch = true
    } else if requestWebSearch == 0 {
        webSearch = false
    }
    if webSearch {
        if err := SetWebSearch(true); err != nil {
            // Non-fatal: log and continue
            fmt.Fprintf(os.Stderr, "[warn] SetWebSearch failed: %v\n", err)
        }
    }

    return Ask(query)
}
```

> **Note:** `os` import is needed in search.go — add it to imports.

---

## Task 7: Delete driver/perplexity.go and driver/api.go

**Files:**
- Delete: `driver/perplexity.go`
- Delete: `driver/api.go`

Run:
```bash
rm driver/perplexity.go driver/api.go
```

---

## Task 8: Update cmd/root.go

**Files:**
- Modify: `cmd/root.go`

**Changes:**
1. Change `Use: "pplx [query]"` → `Use: "gpt [query]"`
2. Change `Short` and `Long` descriptions to reference ChatGPT
3. Remove `--sources` flag (not applicable to ChatGPT)
4. Add `--web-search` bool flag (replaces sources)
5. Update `doSearch` to call `driver.Ask` instead of `driver.Search`
6. Remove `parseSources` function
7. Update `runREPL` prompt string
8. Remove `sourcesCmd` and `apiCmd` from `rootCmd.AddCommand`

New flag set:
```go
var (
    flagModel     string
    flagWebSearch bool
    flagJSON      bool
    flagQuiet     bool
)

func init() {
    rootCmd.PersistentFlags().StringVar(&flagModel, "model", "",
        "模型名称前缀, 如: GPT-5.3, 传统")
    rootCmd.PersistentFlags().BoolVar(&flagWebSearch, "web-search", false,
        "启用 ChatGPT 网络搜索")
    rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false,
        "以 JSON 格式输出结果")
    rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false,
        "只输出答案正文")

    rootCmd.AddCommand(statusCmd)
    rootCmd.AddCommand(modelsCmd)
    rootCmd.AddCommand(versionCmd)
    rootCmd.AddCommand(dumpCmd)
    rootCmd.AddCommand(mcpCmd)
    rootCmd.AddCommand(setupCaffeinateCmd)
    rootCmd.AddCommand(removeCaffeinateCmd)
}
```

New `doSearch`:
```go
func doSearch(query string) error {
    webSearchInt := -1 // use dispatcher default
    if flagWebSearch {
        webSearchInt = 1
    }

    spin := output.NewSpinner("正在询问 ChatGPT...")
    spin.Start()

    result, err := driver.Ask(query)  // direct call, no dispatcher needed for single-shot CLI
    spin.Stop()

    if err != nil {
        return fmt.Errorf("ask failed: %w", err)
    }

    // For CLI single-shot: handle model and web search before Ask
    // Actually restructure: set model/web-search before calling Ask
    output.PrintResult(&driver.SearchResult{Answer: result.Answer, Model: result.Model}, flagJSON, flagQuiet)
    return nil
}
```

**Better approach for doSearch:**
```go
func doSearch(query string) error {
    if flagModel != "" {
        if err := driver.SetModel(flagModel); err != nil {
            return fmt.Errorf("set model failed: %w", err)
        }
    }
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

    printAskResult(result, flagJSON, flagQuiet)
    return nil
}
```

---

## Task 9: Update output/format.go for AskResult

**Files:**
- Modify: `output/format.go`

The existing `PrintResult` takes `*driver.SearchResult` (has citations). ChatGPT responses don't have citations. Options:
1. Keep `SearchResult` struct and just leave `Citations` empty
2. Add `PrintAskResult` that takes `*driver.AskResult`

**Decision: Keep `SearchResult` but add adapter in cmd/root.go that converts `AskResult` → `SearchResult`:**

```go
// In cmd/root.go doSearch:
sr := &driver.SearchResult{
    Answer: result.Answer,
    Model:  result.Model,
}
output.PrintResult(sr, flagJSON, flagQuiet)
```

This is zero-friction since `SearchResult` already exists and `output.PrintResult` is already wired up.

---

## Task 10: Update cmd/models.go

**Files:**
- Modify: `cmd/models.go`

Replace Perplexity model list with ChatGPT models (from confirmed AX exploration):

```go
var availableModels = []struct {
    Name         string
    Description  string
    ButtonPrefix string
}{
    {"GPT-5.3 旗舰", "最新旗舰模型", "GPT-5.3"},
    {"传统模型", "传统 ChatGPT 模型", "传统"},
}
```

Update `modelsCmd.Short`, help text, and example command from `pplx` → `gpt`.

---

## Task 11: Delete cmd/sources.go and cmd/api.go

**Files:**
- Delete: `cmd/sources.go`
- Delete: `cmd/api.go`

Run:
```bash
rm cmd/sources.go cmd/api.go
```

---

## Task 12: Update cmd/status.go

**Files:**
- Modify: `cmd/status.go`

Change `Short` from "显示当前 Perplexity 搜索模式和模型" → "显示 ChatGPT Desktop App 状态".

---

## Task 13: Update cmd/mcp.go

**Files:**
- Modify: `cmd/mcp.go`

**Major changes:**
1. Remove `--primary`/`--fallback`/`--sources` flags (no API backend, no sources)
2. Add `--model` and `--web-search` flags
3. Remove `PERPLEXITY_API_KEY` check
4. Update server name from `"Perplexity Research"` → `"ChatGPT"`
5. Update tool descriptions to reference ChatGPT
6. Remove `handleListSources` (no sources concept)
7. Update `handleSearch` to use `driver.Dispatcher.Ask`
8. Update `handleListModels` to show ChatGPT models
9. Remove `list_sources` tool registration
10. Rename MCP config example from `pplx` → `gpt`, remove `PERPLEXITY_API_KEY`

New flags:
```go
var (
    flagMCPModel     string
    flagMCPWebSearch bool
)
```

New `runMCP`:
```go
func runMCP(cmd *cobra.Command, args []string) error {
    if err := driver.EnsureAppRunning(); err != nil {
        return fmt.Errorf("ChatGPT Desktop App: %w", err)
    }
    // caffeinate check
    if err := exec.Command("pgrep", "-x", "caffeinate").Run(); err != nil {
        fmt.Fprintf(cmd.ErrOrStderr(), "[mcp] warning: caffeinate not running\n")
        fmt.Fprintf(cmd.ErrOrStderr(), "[mcp] hint: run `gpt setup-caffeinate`\n")
    }

    mcpDispatcher = &driver.Dispatcher{
        PrimaryModel: flagMCPModel,
        WebSearch:    flagMCPWebSearch,
    }

    s := server.NewMCPServer("ChatGPT", Version, server.WithToolCapabilities(true))
    s.AddTool(searchTool, handleSearch)
    s.AddTool(listModelsTool, handleListModels)
    return server.ServeStdio(s)
}
```

---

## Task 14: Update cmd/dump.go

**Files:**
- Modify: `cmd/dump.go`

Change default bundle ID from `driver.BundleID` (already will be `com.openai.chat` after Task 5). No other changes needed — it already accepts an optional bundle ID argument.

---

## Task 15: Build and verify

**Step 1: Run build**

```bash
make build
```

Expected: `build/gpt` binary created, no compile errors.

**Step 2: Check for remaining pplx references**

```bash
grep -r "pplx\|Perplexity\|perplexity" --include="*.go" . | grep -v "_test\|vendor\|\.git"
```

Fix any remaining references.

**Step 3: Test accessibility**

```bash
build/gpt status
```

Expected: "running" or "not running".

---

## Task 16: End-to-end test

**Step 1: Basic query**

```bash
build/gpt "what is 2+2"
```

Expected: ChatGPT responds with "4" or similar.

**Step 2: JSON output**

```bash
build/gpt "what is 2+2" --json | jq '.answer'
```

Expected: JSON with answer field.

**Step 3: Web search**

```bash
build/gpt --web-search "today's date"
```

Expected: ChatGPT uses web search and responds with current date context.

**Step 4: MCP server**

```bash
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | build/gpt mcp
```

Expected: JSON response listing `search` and `list_models` tools.

---

## Summary of file changes

| File | Action |
|------|--------|
| `go.mod` | Module rename |
| `Makefile` | Binary rename |
| `automation/ax.h` | Add 2 declarations |
| `automation/ax.m` | Add 2 implementations |
| `automation/ax.go` | Add 2 Go wrappers |
| `driver/chatgpt.go` | CREATE (main driver) |
| `driver/search.go` | Simplify (UI only) |
| `driver/perplexity.go` | DELETE |
| `driver/api.go` | DELETE |
| `cmd/root.go` | Update for ChatGPT |
| `cmd/models.go` | Update model list |
| `cmd/sources.go` | DELETE |
| `cmd/api.go` | DELETE |
| `cmd/status.go` | Update description |
| `cmd/mcp.go` | Major rewrite |
| `cmd/dump.go` | Minor update |
