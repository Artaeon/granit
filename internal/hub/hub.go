// Package hub is the canonical schema + IO for the personal hub —
// granit's "single login, find everything I need" page. A hub item
// is a quick-access entry: a link to a tool the user uses, a
// non-critical credential (think internal tools, dev consoles, the
// kind of things you might otherwise tape to a sticky note), or a
// reference URL.
//
// Storage lives at <vault>/.granit/hub.json with 0o600 permissions
// so the credentials portion isn't world-readable on a shared
// machine. Credentials are NOT cryptographically protected on disk
// — the user is expected to keep real secrets in a proper password
// manager (the UI carries this caveat). Granit's hub exists for
// the "what's the URL of my staging dashboard / what's the dev
// API key for service X" tier of access, where convenience matters
// more than vault-level security.
//
// Pure data + IO. No HTTP, no rendering. Stdlib + atomicio.
package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/oklog/ulid/v2"
)

// Item is a single hub entry. The fields are deliberately optional
// because the user's actual usage will mix shapes: pure links
// (just title + url), tool entries (title + url + category), and
// credentialed entries (title + url + username + password). One
// shape covers them all without forcing the user to decide which
// "kind" of entry they're creating.
//
// CreatedAt + UpdatedAt are RFC3339; ID is a Crockford-base32 ULID
// so the entries sort lexicographically by creation time.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`

	// URL is the primary link. When set, clicking the entry opens
	// it in a new tab. Optional — entries can also be plain notes
	// (a software licence key the user wants to keep handy, etc).
	URL string `json:"url,omitempty"`

	// Category is free-text. The page groups items by category so
	// the user gets clusters like "Dev tools", "Internal", "Music"
	// without a forced taxonomy. Empty = "Uncategorised".
	Category string `json:"category,omitempty"`

	// Icon is a short visual marker — typically an emoji like 🐙 or
	// a single character. Falls back to the first letter of the
	// title in the UI when empty.
	Icon string `json:"icon,omitempty"`

	// Notes carry free-form context (what the tool is for, login
	// gotchas, the user's reminder of why this is bookmarked).
	Notes string `json:"notes,omitempty"`

	// Username + Password are the optional non-critical credential
	// block. They're stored as plain strings on disk; the UI shows
	// a clear "use a real password manager for sensitive secrets"
	// notice. We don't pretend to encrypt — the value is
	// "convenience tier" not "security tier".
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Favorite items get pinned to the top of the page (and any
	// future dashboard widget). Boolean, defaults to false.
	Favorite bool `json:"favorite,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// hubFile is the on-disk shape. A single object with a versioned
// items list — versioning leaves room to evolve the format later
// (encrypted credential block, multi-vault sharing, etc) without
// breaking older clients reading the same file.
type hubFile struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

const fileVersion = 1

// path returns the absolute path to .granit/hub.json under the
// vault. The .granit directory is created on first save.
func path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "hub.json")
}

// LoadAll reads every hub item. A missing file is not an error —
// it just means the user hasn't added any items yet, so we return
// an empty slice. Any parse failure surfaces so the caller can
// distinguish "no items" from "bad file".
func LoadAll(vaultRoot string) ([]Item, error) {
	data, err := os.ReadFile(path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("hub: read: %w", err)
	}
	var f hubFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("hub: parse: %w", err)
	}
	// Sort favorites first, then alpha by title — stable order so
	// the API response is deterministic regardless of save order.
	sort.SliceStable(f.Items, func(i, j int) bool {
		if f.Items[i].Favorite != f.Items[j].Favorite {
			return f.Items[i].Favorite
		}
		return strings.ToLower(f.Items[i].Title) < strings.ToLower(f.Items[j].Title)
	})
	return f.Items, nil
}

// SaveAll writes the full set atomically. Caller is expected to
// have made any in-memory mutations before calling. We don't
// support per-item upsert at the IO layer because the file is
// small (typically < 100 entries) and a full rewrite is simpler
// + race-safer than a read-modify-write at the entry level.
func SaveAll(vaultRoot string, items []Item) error {
	if err := os.MkdirAll(filepath.Dir(path(vaultRoot)), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(hubFile{Version: fileVersion, Items: items}, "", "  ")
	if err != nil {
		return fmt.Errorf("hub: marshal: %w", err)
	}
	return atomicio.WriteState(path(vaultRoot), data)
}

// NewID mints a fresh ULID for a new hub item. Lowercased so the
// IDs are URL-safe and read consistently in JSON.
func NewID() string {
	return strings.ToLower(ulid.Make().String())
}

// Now returns an RFC3339 timestamp — used for CreatedAt /
// UpdatedAt fields. Centralised so test callers can swap it.
func Now() string {
	return time.Now().Format(time.RFC3339)
}
