// Package emailsignatures stores HTML email signatures the user
// reuses across mail clients. Storage lives at
// <vault>/.granit/email-signatures.json (0o600). Pure data + IO,
// stdlib + atomicio — no HTTP, no rendering.
//
// Why a first-class object: the user has many signatures (work,
// venture A, venture B, formal, casual…) and managing them as
// a list of HTML blobs in a regular note loses the structure.
// Native CRUD + a render-preview surface lets the user maintain,
// preview, and copy each signature in one place.
//
// Security note: the HTML body is the user's own — signatures
// are content the user authors. The web preview MUST render in
// an iframe sandbox (no scripts) because user-authored HTML can
// still contain `<script>` blocks that would fire on the preview
// page. The iframe boundary is the trust line; this package only
// stores bytes.
package emailsignatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/oklog/ulid/v2"
)

// Signature is one stored signature record. Fields kept minimal —
// HTML is the payload, everything else is metadata the user wants
// to filter or organise by.
type Signature struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	HTML  string `json:"html"`
	// Optional plain-text fallback. Some mail clients render the
	// text/plain part instead of text/html (especially on mobile
	// quick-replies). Letting the user store both keeps signatures
	// portable across clients.
	PlainText string `json:"plain_text,omitempty"`
	// Optional grouping — "Work", "Personal", "Venture: Stoicera"
	// etc. Free-text so the user can shape the namespace.
	Category string `json:"category,omitempty"`
	// IsDefault marks the user's "use this unless I pick another"
	// signature. At most one signature per vault should carry the
	// flag; SaveAll enforces that by clearing the flag on every
	// other entry when one is set.
	IsDefault bool   `json:"is_default,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "email-signatures.json")
}

// LoadAll reads every signature from disk, sorted by name (case-
// insensitive). Missing file → empty slice. Malformed JSON →
// error so the user sees the corruption rather than silently
// losing entries.
func LoadAll(vaultRoot string) ([]Signature, error) {
	data, err := os.ReadFile(path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Signature{}, nil
		}
		return nil, fmt.Errorf("emailsignatures: read: %w", err)
	}
	var list []Signature
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("emailsignatures: parse: %w", err)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return foldCmp(list[i].Name, list[j].Name) < 0
	})
	return list, nil
}

// Find returns the signature with the given ID or nil.
func Find(vaultRoot, id string) (*Signature, error) {
	list, err := LoadAll(vaultRoot)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, nil
}

// Upsert replaces an existing signature by ID, or appends when ID
// is empty / not found. Setting IsDefault clears the flag on every
// other entry (only one default per vault).
func Upsert(vaultRoot string, s Signature) (Signature, error) {
	list, err := LoadAll(vaultRoot)
	if err != nil {
		return s, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if s.ID == "" {
		s.ID = NewID()
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.IsDefault {
		for i := range list {
			if list[i].ID != s.ID {
				list[i].IsDefault = false
			}
		}
	}
	replaced := false
	for i := range list {
		if list[i].ID == s.ID {
			if s.CreatedAt == "" {
				s.CreatedAt = list[i].CreatedAt
			}
			list[i] = s
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, s)
	}
	if err := SaveAll(vaultRoot, list); err != nil {
		return s, err
	}
	return s, nil
}

// Delete removes the signature with the given ID. Missing ID → no-op
// (idempotent so a double-click on the trash icon doesn't 404).
func Delete(vaultRoot, id string) error {
	list, err := LoadAll(vaultRoot)
	if err != nil {
		return err
	}
	out := list[:0]
	for _, s := range list {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return SaveAll(vaultRoot, out)
}

// SaveAll writes the canonical list back to disk atomically. JSON
// is indented for git-friendly diffs.
func SaveAll(vaultRoot string, list []Signature) error {
	if list == nil {
		list = []Signature{}
	}
	dir := filepath.Dir(path(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("emailsignatures: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("emailsignatures: marshal: %w", err)
	}
	return atomicio.WriteState(path(vaultRoot), data)
}

// NewID returns a Crockford-base32 ULID — sortable by creation time
// so a future "newest first" view doesn't need a separate index.
func NewID() string {
	return ulid.Make().String()
}

// foldCmp compares two strings case-insensitively. Tiny helper so
// LoadAll's Sort doesn't pull in strings.EqualFold inside the
// closure.
func foldCmp(a, b string) int {
	la := len(a)
	lb := len(b)
	n := la
	if lb < n {
		n = lb
	}
	for i := 0; i < n; i++ {
		ca := lower(a[i])
		cb := lower(b[i])
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
	}
	switch {
	case la < lb:
		return -1
	case la > lb:
		return 1
	}
	return 0
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
