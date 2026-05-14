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

// TestICS_KindRoundTrip verifies the X-GRANIT-KIND extension
// survives the read-write-read cycle: write an event with Kind,
// re-parse it, see the field; PATCH it with a new Kind via the
// HTTP layer, re-parse, see the new value.
func TestICS_KindRoundTrip(t *testing.T) {
	_, h, root := icsTestServer(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "work.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "work"}, []icswriter.Event{{
		UID:     "k1@granit",
		Summary: "1:1 with Sam",
		Start:   mustTime(t, "2026-05-15T09:00:00Z"),
		End:     mustTime(t, "2026-05-15T09:30:00Z"),
		Kind:    "meeting",
	}}); err != nil {
		t.Fatal(err)
	}

	on := readFileT(t, path)
	if !strings.Contains(on, "X-GRANIT-KIND:meeting") {
		t.Errorf("X-GRANIT-KIND line missing or wrong:\n%s", on)
	}

	// Read back through the calendar feed reader.
	parsed, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed[0].Kind != "meeting" {
		t.Fatalf("parsed Kind: got %+v", parsed)
	}

	// PATCH to a different Kind via HTTP.
	kind := "focus"
	enc := url.PathEscape("k1@granit")
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/work.ics/events/"+enc, map[string]interface{}{
		"kind": kind,
	})
	if code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", code, body)
	}
	on = readFileT(t, path)
	if !strings.Contains(on, "X-GRANIT-KIND:focus") {
		t.Errorf("Kind not updated to focus:\n%s", on)
	}
	if strings.Contains(on, "X-GRANIT-KIND:meeting") {
		t.Errorf("old meeting kind still on disk:\n%s", on)
	}

	// PATCH with kind="" — clears the line.
	code, body = icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/work.ics/events/"+enc, map[string]interface{}{
		"kind": "",
	})
	if code != http.StatusOK {
		t.Fatalf("PATCH clear: %d %s", code, body)
	}
	on = readFileT(t, path)
	if strings.Contains(on, "X-GRANIT-KIND:") {
		t.Errorf("Kind=\"\" should clear the line:\n%s", on)
	}

	// Case-normalisation: hand-edited "Meeting" should read as
	// "meeting" through the parser.
	withCap := strings.Replace(on, "BEGIN:VEVENT", "BEGIN:VEVENT\r\nX-GRANIT-KIND:Meeting", 1)
	if err := os.WriteFile(path, []byte(withCap), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err = parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed[0].Kind != "meeting" {
		t.Errorf("case-normalised Kind: got %q", parsed[0].Kind)
	}
}
