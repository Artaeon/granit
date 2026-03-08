package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const maxSearchHistory = 20

// SearchHistory holds persisted search queries for content search and find/replace.
type SearchHistory struct {
	ContentSearch []string `json:"content_search"`
	FindReplace   []string `json:"find_replace"`
}

// searchHistoryPath returns the path to the search history JSON file.
func searchHistoryPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "search-history.json")
}

// loadSearchHistory reads the search history from disk.
// Returns an empty SearchHistory if the file does not exist or is invalid.
func loadSearchHistory(vaultRoot string) SearchHistory {
	var h SearchHistory
	data, err := os.ReadFile(searchHistoryPath(vaultRoot))
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, &h)
	// Enforce caps on load
	if len(h.ContentSearch) > maxSearchHistory {
		h.ContentSearch = h.ContentSearch[len(h.ContentSearch)-maxSearchHistory:]
	}
	if len(h.FindReplace) > maxSearchHistory {
		h.FindReplace = h.FindReplace[len(h.FindReplace)-maxSearchHistory:]
	}
	return h
}

// saveSearchHistory writes the search history to disk.
func saveSearchHistory(vaultRoot string, h SearchHistory) {
	dir := filepath.Join(vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(searchHistoryPath(vaultRoot), data, 0600)
}

// appendToHistory adds a query to a history slice if it is non-empty and
// different from the last entry, capping at maxSearchHistory.
func appendToHistory(history []string, query string) []string {
	if query == "" {
		return history
	}
	if len(history) > 0 && history[len(history)-1] == query {
		return history
	}
	history = append(history, query)
	if len(history) > maxSearchHistory {
		history = history[len(history)-maxSearchHistory:]
	}
	return history
}
