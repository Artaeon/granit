package sabbath

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// LogEntry is one observance event. Event is either "begin" or
// "end". The log is append-only — entries are never deleted, only
// added. The intent is *witness*, not measurement: a quiet record
// that the day was honored.
//
// We deliberately avoid recording anything beyond "begin"/"end" and
// timestamp. No "intent", no "duration", no "kept-the-fast" boolean
// — adding those columns is exactly the slope that turns this into
// a streak counter, which is what [[feedback-life-tree-not-gamified]]
// asks us to resist.
type LogEntry struct {
	At    string `json:"at"`    // RFC3339
	Event string `json:"event"` // "begin" | "end"
}

func logPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "sabbath-log.json")
}

// LoadLog returns every entry in chronological (oldest-first) order.
// Missing file → empty slice, not an error.
func LoadLog(vaultRoot string) ([]LogEntry, error) {
	data, err := os.ReadFile(logPath(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var entries []LogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// AppendLog adds one entry and rewrites the file atomically. We
// rewrite-the-whole-file rather than streaming-append because the
// file stays tiny (one user, a handful of sabbaths per month) and
// atomicio gives us safe writes for free.
//
// Coalesces duplicates: if the most recent entry has the same Event
// AND its timestamp is within 60s of the new one, the append is
// dropped. Stops UI re-toggles from spamming the log when a device
// re-PUTs state on every focus.
func AppendLog(vaultRoot string, entry LogEntry) error {
	entries, err := LoadLog(vaultRoot)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		if last.Event == entry.Event {
			if lastT, err1 := time.Parse(time.RFC3339, last.At); err1 == nil {
				if newT, err2 := time.Parse(time.RFC3339, entry.At); err2 == nil {
					if newT.Sub(lastT).Abs() < time.Minute {
						return nil
					}
				}
			}
		}
	}
	entries = append(entries, entry)
	dir := filepath.Dir(logPath(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(logPath(vaultRoot), data)
}
