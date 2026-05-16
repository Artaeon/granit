// Package hub — tools catalogue.
//
// The "tools" half of the hub is a separate concern from the link
// half: where Items are URLs the user opens, Tools are curated
// setup-command snippets the user copies into a terminal — "how do
// I install / configure / use this program on a fresh machine". A
// card for "neovim" carries an ordered list of commands ("install
// via brew", "clone dotfiles", "open config"); a card for "kubectl"
// carries context switches + common one-liners.
//
// Storage lives at <vault>/.granit/hub-tools.json — separate file
// from hub.json because the shapes are different (a tool is a
// header + a list of commands, an item is a single record) and
// keeping them apart means a corrupted commands list can't take
// down the link launcher and vice-versa. Atomic writes only.
package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artaeon/granit/internal/atomicio"
)

// Command is one entry in a Tool's ordered list. Label is what the
// user reads in the UI ("install via brew"); Command is the actual
// shell line that lands in their clipboard ("brew install neovim").
// Notes is optional context — pre-conditions, expected output, etc.
type Command struct {
	Label   string `json:"label"`
	Command string `json:"command"`
	Notes   string `json:"notes,omitempty"`
}

// Tool is a single catalogue card. Name is the program/tool the
// commands belong to ("neovim", "kubectl", "1password CLI"). Icon
// is an emoji or single-char glyph; Color is a Tailwind colour
// name the UI uses to tint the card border. Tags are free-form
// labels the search/filter input matches against.
//
// SortOrder is rewritten by Reorder; 1-based so a freshly-saved
// tool with the default 0 sorts last (gives the user a visual
// "newest at the bottom" cue until they drag).
type Tool struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Color       string    `json:"color,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Commands    []Command `json:"commands"`
	SortOrder   int       `json:"sort_order,omitempty"`
	CreatedAt   string    `json:"created_at,omitempty"`
	UpdatedAt   string    `json:"updated_at,omitempty"`
}

// toolsFile mirrors hubFile — a versioned envelope so we can evolve
// the shape later without breaking older clients reading the same
// file (e.g. add per-tool environment overrides, command-output
// expectations, etc).
type toolsFile struct {
	Version int    `json:"version"`
	Tools   []Tool `json:"tools"`
}

const toolsFileVersion = 1

func toolsPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "hub-tools.json")
}

// LoadAllTools reads every tool card. A missing file is not an
// error — the user simply hasn't added any tools yet. Sort order:
// the user's SortOrder (manual drag) wins, with alpha by name as
// the tiebreaker so freshly-created tools (SortOrder == 0) cluster
// together at the top.
func LoadAllTools(vaultRoot string) ([]Tool, error) {
	data, err := os.ReadFile(toolsPath(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("hub: read tools: %w", err)
	}
	var f toolsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("hub: parse tools: %w", err)
	}
	sort.SliceStable(f.Tools, func(i, j int) bool {
		a, b := f.Tools[i], f.Tools[j]
		if a.SortOrder != b.SortOrder {
			// Zero (default / never-reordered) sorts AFTER any
			// explicit ordering so dragged tools surface first.
			if a.SortOrder == 0 {
				return false
			}
			if b.SortOrder == 0 {
				return true
			}
			return a.SortOrder < b.SortOrder
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return f.Tools, nil
}

// SaveAllTools writes the catalogue atomically. Mirrors SaveAll —
// the file is small (typically < 50 tools), so a full rewrite is
// simpler than per-tool merging and side-steps read-modify-write
// races.
func SaveAllTools(vaultRoot string, tools []Tool) error {
	if err := os.MkdirAll(filepath.Dir(toolsPath(vaultRoot)), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(toolsFile{Version: toolsFileVersion, Tools: tools}, "", "  ")
	if err != nil {
		return fmt.Errorf("hub: marshal tools: %w", err)
	}
	return atomicio.WriteState(toolsPath(vaultRoot), data)
}

// ReorderTools rewrites SortOrder on the given IDs in the supplied
// order. Tools not in the list keep their existing SortOrder.
// Matches Reorder's contract: 1-based positions so unset (0) sorts
// after anything explicit.
func ReorderTools(vaultRoot string, orderedIDs []string) error {
	tools, err := LoadAllTools(vaultRoot)
	if err != nil {
		return err
	}
	idx := make(map[string]int, len(tools))
	for i := range tools {
		idx[tools[i].ID] = i
	}
	now := Now()
	for newPos, id := range orderedIDs {
		i, ok := idx[id]
		if !ok {
			continue
		}
		tools[i].SortOrder = newPos + 1
		tools[i].UpdatedAt = now
	}
	return SaveAllTools(vaultRoot, tools)
}

// SanitizeCommands trims label / command whitespace and drops rows
// with neither set. Used at create + patch time so the on-disk
// shape stays clean (no empty-command ghost rows from a form that
// rendered an extra blank line the user never filled in).
func SanitizeCommands(in []Command) []Command {
	out := make([]Command, 0, len(in))
	for _, c := range in {
		c.Label = strings.TrimSpace(c.Label)
		c.Command = strings.TrimSpace(c.Command)
		c.Notes = strings.TrimSpace(c.Notes)
		if c.Label == "" && c.Command == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// SanitizeTags trims + drops empties + lower-cases tags so the
// search input matches consistently regardless of how the user
// typed them. Order is preserved so a tool's "primary" tag stays
// the leading one in the UI.
func SanitizeTags(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

