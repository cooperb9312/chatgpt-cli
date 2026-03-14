package driver

import (
	"fmt"
	"os"
)

// Dispatcher manages ChatGPT Desktop UI search/ask.
// Only the UI backend is supported (ChatGPT Desktop App).
type Dispatcher struct {
	PrimaryModel string // default model (empty = don't change)
	WebSearch    bool   // default web search setting
}

// Ask sends a query via ChatGPT Desktop UI.
// requestModel: per-request model override (empty = use Dispatcher.PrimaryModel)
// requestWebSearch: per-request web search override (-1=use default, 0=off, 1=on)
func (d *Dispatcher) Ask(query, requestModel string, requestWebSearch int) (*SearchResult, error) {
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
			fmt.Fprintf(os.Stderr, "[warn] SetWebSearch failed: %v\n", err)
		}
	}

	return Ask(query)
}
