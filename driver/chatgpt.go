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
	askTimeout    = 300 * time.Second
	askPoll       = 500 * time.Millisecond
	windowTimeout = 10 * time.Second
	windowPoll    = 500 * time.Millisecond

	// UI delays
	uiDelay      = 300 * time.Millisecond
	popoverDelay = 400 * time.Millisecond

	// Generation detection button label (present=generating, absent=done)
	generatingIndicator = "停止生成"
)

// AskResult holds the ChatGPT response
type AskResult struct {
	Answer string `json:"answer"`
	Model  string `json:"model"`
}

// SearchResult is an alias for backward compatibility with cmd/ package.
// ChatGPT responses don't have citations, so Citations will always be empty.
type SearchResult struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Mode      string     `json:"mode"`
	Model     string     `json:"model"`
}

// Citation kept for struct compatibility (ChatGPT Desktop doesn't return citations)
type Citation struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// EnsureAppRunning ensures ChatGPT Desktop App is running.
// If not running, launches it with `open -b` and waits for AX accessibility.
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

// ensureWindow activates app and waits for its AXWindow to appear.
func ensureWindow() error {
	if err := EnsureAppRunning(); err != nil {
		return err
	}
	if err := automation.WaitForWindow(BundleID, windowTimeout, windowPoll); err != nil {
		return fmt.Errorf("ChatGPT window not available: %w (is a display connected?)", err)
	}
	return nil
}

// NavigateToHome clicks "新聊天" to start a fresh conversation.
// ChatGPT always starts a new chat per query to avoid appending to old conversations.
func NavigateToHome() error {
	if err := ensureWindow(); err != nil {
		return err
	}
	// Dismiss any open popover first
	if automation.HasPopover(BundleID) {
		fmt.Fprintf(os.Stderr, "[nav] popover open, pressing Escape\n")
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

// SetModel opens the model selector popover and clicks the model matching the prefix.
// modelName is a prefix of the button's AXDescription, e.g. "GPT-5.3" or "传统".
// The model selector button itself has AXDescription "ChatGPT" in the toolbar.
func SetModel(modelName string) error {
	if err := ensureWindow(); err != nil {
		return err
	}
	modelBtn, err := automation.FindButton(BundleID, "ChatGPT")
	if err != nil {
		return fmt.Errorf("model button not found (ChatGPT toolbar button): %w", err)
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

// SetWebSearch clicks the 搜索 toggle button to enable web search.
// This is a best-effort operation — if the button is not found, it logs a warning
// (some models may not support web search).
func SetWebSearch(enable bool) error {
	if !enable {
		return nil
	}
	if err := ensureWindow(); err != nil {
		return err
	}
	searchBtn, err := automation.FindButton(BundleID, "搜索")
	if err != nil {
		return fmt.Errorf("web search button not found (model may not support it): %w", err)
	}
	defer searchBtn.Release()
	if err := automation.Click(searchBtn); err != nil {
		return fmt.Errorf("failed to click web search: %w", err)
	}
	time.Sleep(uiDelay)
	return nil
}

// Ask sends query to ChatGPT Desktop and waits for the full response.
//
// Flow:
//  1. Set text in AXTextArea via SetTextAreaValue
//  2. Click 发送 button
//  3. Wait for 停止生成 to appear (generation started), then disappear (generation done)
//  4. Read response text via ReadResponseText (kAXDescriptionAttribute of AXStaticText)
func Ask(query string) (*SearchResult, error) {
	if err := automation.SetTextAreaValue(BundleID, query); err != nil {
		return nil, fmt.Errorf("failed to set query text: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	sendBtn, err := automation.FindButton(BundleID, "发送")
	if err != nil {
		return nil, fmt.Errorf("发送 button not found: %w", err)
	}
	defer sendBtn.Release()
	if err := automation.Click(sendBtn); err != nil {
		return nil, fmt.Errorf("failed to click 发送: %w", err)
	}

	if err := waitForGenerationComplete(BundleID, askTimeout, askPoll); err != nil {
		return nil, fmt.Errorf("ask timed out: %w", err)
	}

	text, err := automation.ReadResponseText(BundleID)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	answer := extractLastResponse(text, query)

	return &SearchResult{
		Answer: answer,
		Model:  "",
		Mode:   "ui",
	}, nil
}

// waitForGenerationComplete polls until 停止生成 button disappears (generation done).
//
// Phase 1: wait up to 5s for 停止生成 to APPEAR (confirms generation started).
// Phase 2: wait up to timeout for 停止生成 to DISAPPEAR (confirms generation done).
// If phase 1 times out (button never appeared), we assume immediate/instant response.
func waitForGenerationComplete(bundleID string, timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Phase 1: wait for generation to START
	startDeadline := time.Now().Add(5 * time.Second)
	generationStarted := false
	for time.Now().Before(startDeadline) {
		if automation.HasButton(bundleID, generatingIndicator) {
			generationStarted = true
			fmt.Fprintf(os.Stderr, "[wait] generation started\n")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !generationStarted {
		fmt.Fprintf(os.Stderr, "[wait] 停止生成 never appeared, assuming immediate response\n")
	}

	// Phase 2: wait for generation to COMPLETE
	for time.Now().Before(deadline) {
		if !automation.HasButton(bundleID, generatingIndicator) {
			fmt.Fprintf(os.Stderr, "[wait] generation complete\n")
			time.Sleep(500 * time.Millisecond) // settle time for AX tree
			return nil
		}
		time.Sleep(poll)
	}

	return fmt.Errorf("timed out waiting for generation to complete (waited %v)", timeout)
}

// extractLastResponse extracts the AI's reply from the full AX text dump.
// ax_read_response_text returns all AXStaticText blocks joined by "\n\n".
// We attempt to isolate the content after the user's query echo.
func extractLastResponse(fullText, query string) string {
	if idx := strings.LastIndex(fullText, query); idx != -1 {
		after := strings.TrimSpace(fullText[idx+len(query):])
		if after != "" {
			return after
		}
	}
	return strings.TrimSpace(fullText)
}

// GetStatus returns a brief status string for ChatGPT Desktop App.
func GetStatus() (status, model string, err error) {
	if err := automation.ActivateApp(BundleID); err == nil {
		return "running", "", nil
	}
	return "not running", "", nil
}
