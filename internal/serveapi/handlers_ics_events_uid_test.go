package serveapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/icswriter"
)

// TestICS_PatchEventWithSpecialUIDs covers UIDs from common third-
// party calendars to make sure the URL round-trip + chi decode lands
// in the handler with the right value. Pre-fix, all four passed
// already; we keep the cases as a regression guard against a future
// chi upgrade that changes decoding semantics.
func TestICS_PatchEventWithSpecialUIDs(t *testing.T) {
	cases := []struct {
		name string
		uid  string
	}{
		{"hex-dash", "0E5C20E8-9A2C-4F7A-A412-C7DD8C2A5B3D"},
		{"at-sign", "_8d2j8phc6krj4ba26t0jepbpdtg32@google.com"},
		{"long-fold", "040000008200E00074C5B7101A82E00800000000F0E5C20E89A2C4F7AA412C7DD8C2A5B3D000000000010000000C6BAEA2A33E73345A87E96EF7B07DB91"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, h, root := icsTestServer(t)
			if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "calendars", "test.ics")
			ev := icswriter.Event{
				UID:     tc.uid,
				Summary: "test",
				Start:   mustTime(t, "2026-05-15T09:00:00Z"),
				End:     mustTime(t, "2026-05-15T10:00:00Z"),
			}
			if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "test"}, []icswriter.Event{ev}); err != nil {
				t.Fatal(err)
			}
			encoded := url.PathEscape(tc.uid)
			code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/test.ics/events/"+encoded, map[string]interface{}{
				"summary": "renamed",
			})
			if code != http.StatusOK {
				t.Fatalf("PATCH with UID %q (encoded %q): got %d, body=%s", tc.uid, encoded, code, body)
			}
		})
	}
}

// TestICS_PatchTolerantOfWhitespaceInStoredUID is the actual repro
// for the "ics event not found" reports: some inbound .ics files
// (Apple Calendar drag-export, certain sync apps) emit
// `UID: foo@bar` with a stray leading space the original parser
// stored verbatim. The frontend's JSON round-trip preserved the
// whitespace too, but the handler's strict == match silently failed
// on the trimmed-vs-untrimmed compare in some path. Tolerant trim
// on both sides makes the match resilient regardless of which side
// drifts.
func TestICS_PatchTolerantOfWhitespaceInStoredUID(t *testing.T) {
	_, h, root := icsTestServer(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "leaks.ics")
	// Plant a VEVENT manually whose UID has a leading space — only
	// inbound files from other apps produce this shape; icswriter
	// never would.
	body := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID: my-event@apple\r\n" +
		"SUMMARY:test\r\n" +
		"DTSTART:20260512T100000Z\r\n" +
		"DTEND:20260512T110000Z\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"SEQUENCE:0\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Read side trims whitespace, so the canonical UID emitted to
	// the wire is "my-event@apple" (no leading space).
	parsed, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("want 1 event, got %d", len(parsed))
	}
	if parsed[0].UID != "my-event@apple" {
		t.Fatalf("parser should trim UID whitespace; got %q", parsed[0].UID)
	}

	// PATCH the canonical form — must succeed even though the
	// on-disk UID still has the leading space.
	encoded := url.PathEscape("my-event@apple")
	code, b := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/leaks.ics/events/"+encoded, map[string]interface{}{
		"summary": "renamed",
	})
	if code != http.StatusOK {
		t.Fatalf("PATCH with canonical UID: got %d, body=%s", code, b)
	}
}

// TestICS_PatchEventNotFoundErrorIsDiagnostic ensures the 404 body
// carries enough info for the user to act ("which UID, which
// source, what to do next"). Pre-fix the message was just "event
// not found" which gave the user no way to tell apart a stale
// cache, a typo, or a real bug.
func TestICS_PatchEventNotFoundErrorIsDiagnostic(t *testing.T) {
	_, h, root := icsTestServer(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "test.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "test"}, nil); err != nil {
		t.Fatal(err)
	}
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/test.ics/events/ghost-uid", map[string]interface{}{
		"summary": "nope",
	})
	if code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", code)
	}
	bodyStr := string(body)
	for _, want := range []string{"ghost-uid", "test.ics", "refresh"} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("error body should mention %q for diagnostics, got: %s", want, bodyStr)
		}
	}
}
