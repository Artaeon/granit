// Package email is the canonical schema + IO for the personal
// email tracker. Granit doesn't send or fetch real email — this
// is a CRM-grade record of inbound + outbound correspondence the
// user manually logs (or pastes into) so important threads don't
// fall through the cracks.
//
// The use case: a user gets an email at work that needs follow-up
// in 3 weeks. They log it here with a follow-up date; granit's
// daily / dashboard views surface it on that date so it doesn't
// sit forgotten in an inbox. Same for outgoing emails the user
// wants to remember they sent ("did I email Jane about the
// proposal? When?").
//
// Storage: <vault>/.granit/emails.json with 0o600 perms — emails
// often carry tone-sensitive context (pricing, agreements,
// personnel) so they're treated like state, not like notes.
//
// Pure data + IO. No HTTP, no rendering. Stdlib + atomicio + ulid.
package email

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

// Direction is whether the email came in or went out. Inferred
// from the user's perspective at log time; a reply they send
// flips direction relative to the original they received.
type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
)

// Status is the user's triage state for the entry. The flow is
// roughly: inbox (newly logged) → read (acknowledged) → replied
// (action taken) → archived (done, kept for reference). Custom
// statuses can be added later if needed; these four cover the
// common workflow.
type Status string

const (
	StatusInbox    Status = "inbox"
	StatusRead     Status = "read"
	StatusReplied  Status = "replied"
	StatusArchived Status = "archived"
)

// Email is a single tracked correspondence record. Only ID,
// Direction, Subject and From/To are required; everything else is
// optional so a user can scribble down a quick "Bob wrote about X"
// without filling out a full form, and refine the record later.
//
// CreatedAt + UpdatedAt are RFC3339 stamps for granit's audit
// pattern. ReceivedAt / SentAt are the actual email timestamps —
// distinct from when the user logged the record (CreatedAt).
type Email struct {
	ID        string    `json:"id"`
	Direction Direction `json:"direction"`
	Subject   string    `json:"subject"`

	// From is the sender's address (or display name if the user
	// only knows the name). To is the list of recipient addresses;
	// for outbound emails the first entry is the primary "to".
	From string   `json:"from"`
	To   []string `json:"to,omitempty"`
	Cc   []string `json:"cc,omitempty"`

	// Body is the plain-text or markdown body of the message.
	// Granit doesn't try to parse HTML email — if the user pastes
	// HTML, that's what gets stored. Markdown is preferred so
	// quick-replies the user drafts here can be copy-pasted into
	// their real email client.
	Body string `json:"body,omitempty"`

	// ReceivedAt / SentAt are the actual email timestamps. Both
	// are optional because the user might log a quick "Jane wrote
	// today" without remembering the exact time.
	ReceivedAt string `json:"received_at,omitempty"`
	SentAt     string `json:"sent_at,omitempty"`

	Status Status   `json:"status"`
	Tags   []string `json:"tags,omitempty"`

	// FollowUpDate is the YYYY-MM-DD the user wants this email
	// surfaced again. Empty = no follow-up; granit's dashboard
	// surfaces overdue follow-ups so they don't get lost.
	FollowUpDate string `json:"follow_up_date,omitempty"`

	// Loose foreign keys. PersonID matches internal/people; Project
	// is the project name (free-text). Both optional — a quick log
	// often hasn't been associated yet.
	PersonID string `json:"person_id,omitempty"`
	Project  string `json:"project,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Path returns the absolute path of the emails sidecar.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "emails.json")
}

// LoadAll reads the sidecar. Missing file → empty list (not an
// error) so a fresh vault doesn't have to be initialised.
func LoadAll(vaultRoot string) ([]Email, error) {
	data, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Email{}, nil
		}
		return nil, fmt.Errorf("email: read: %w", err)
	}
	var out []Email
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("email: parse: %w", err)
	}
	// Defensive normalization: any record missing Status gets
	// StatusInbox so the UI never has to defend against blanks.
	for i := range out {
		if out[i].Status == "" {
			out[i].Status = StatusInbox
		}
		if out[i].Direction == "" {
			out[i].Direction = DirectionIn
		}
	}
	// Stable order: most recent received/sent first; tie-break on
	// CreatedAt. Surfaces fresh items at the top — same pattern
	// the inbox the user is mirroring uses.
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := keyTimestamp(out[i]), keyTimestamp(out[j])
		if ki != kj {
			return ki > kj
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

// keyTimestamp returns the most recent of received_at / sent_at /
// created_at as the sort key. Empty values lose to populated ones
// because we compare strings (RFC3339 is lexicographically
// orderable, and "" < anything-non-empty).
func keyTimestamp(e Email) string {
	t := e.ReceivedAt
	if e.SentAt > t {
		t = e.SentAt
	}
	if e.CreatedAt > t {
		t = e.CreatedAt
	}
	return t
}

// SaveAll persists the list. Wraps atomicio so a crash mid-write
// can't truncate the file.
func SaveAll(vaultRoot string, items []Email) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("email: marshal: %w", err)
	}
	dir := filepath.Dir(Path(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("email: mkdir: %w", err)
	}
	if err := atomicio.WriteState(Path(vaultRoot), data); err != nil {
		return fmt.Errorf("email: write: %w", err)
	}
	return nil
}

// New constructs a fresh Email with a ULID + timestamps populated.
// Caller fills in the user-supplied fields before persisting via
// SaveAll. ULIDs sort by creation time, so they make a sensible
// secondary sort key when timestamps coincide.
func New() Email {
	now := time.Now().UTC().Format(time.RFC3339)
	return Email{
		ID:        ulid.Make().String(),
		Status:    StatusInbox,
		Direction: DirectionIn,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Find returns a pointer to the email with the given ID, or nil
// if not present. Linear scan — emails.json is bounded by user
// behaviour (a few hundred entries at most for an active power
// user), so an index would be premature.
func Find(items []Email, id string) *Email {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// Validate checks the basic invariants before persisting. Returns
// a user-readable error or nil. Subject + (From or To) are
// required because an email with no participants and no subject is
// just an empty record.
func (e *Email) Validate() error {
	if strings.TrimSpace(e.Subject) == "" {
		return errors.New("subject required")
	}
	if e.Direction != DirectionIn && e.Direction != DirectionOut {
		return fmt.Errorf("invalid direction %q (expected in or out)", e.Direction)
	}
	hasFrom := strings.TrimSpace(e.From) != ""
	hasTo := false
	for _, t := range e.To {
		if strings.TrimSpace(t) != "" {
			hasTo = true
			break
		}
	}
	if !hasFrom && !hasTo {
		return errors.New("at least one of from / to required")
	}
	switch e.Status {
	case StatusInbox, StatusRead, StatusReplied, StatusArchived, "":
		// "" gets normalised to inbox by LoadAll; accept it here
		// so a quick-log POST without an explicit status doesn't
		// fail validation.
	default:
		return fmt.Errorf("invalid status %q", e.Status)
	}
	return nil
}
