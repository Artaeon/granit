package serveapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/vault"
)

// streakTestServer plants daily-shaped notes in a tmp vault, wires up
// the Server with enough scaffolding for handleDailyStreak to run.
// Pure function (daily.ComputeStreak) is pinned in its own package;
// this fixture covers the handler-to-pure-function bridge.
func streakTestServer(t *testing.T, dates []string, folder string) http.HandlerFunc {
	t.Helper()
	root := t.TempDir()
	dailyDir := root
	if folder != "" {
		dailyDir = filepath.Join(root, folder)
		if err := os.MkdirAll(dailyDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range dates {
		body := "# " + d + "\n\nbody\n"
		if err := os.WriteFile(filepath.Join(dailyDir, d+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if folder != "" {
		cfg := fmt.Sprintf(`{"daily_notes_folder":%q}`, folder)
		if err := os.WriteFile(filepath.Join(root, ".granit.json"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Vault: v, Logger: slog.Default()}}
	return s.handleDailyStreak
}

func TestHandleDailyStreak_RoundTrip(t *testing.T) {
	// Anchor the test against TODAY so the "today logged" branch
	// is genuinely exercised. Build three consecutive days ending
	// today; expect Current=3, Longest=3, TodayLogged=true.
	today := time.Now().Local()
	d := func(offset int) string {
		return today.AddDate(0, 0, offset).Format("2006-01-02")
	}
	h := streakTestServer(t, []string{d(-2), d(-1), d(0)}, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily/streak", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var got daily.Streak
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Current != 3 {
		t.Errorf("Current = %d, want 3", got.Current)
	}
	if got.Longest != 3 {
		t.Errorf("Longest = %d, want 3", got.Longest)
	}
	if !got.TodayLogged {
		t.Errorf("TodayLogged should be true; got streak=%+v", got)
	}
}

func TestHandleDailyStreak_EmptyVault(t *testing.T) {
	// No dailies anywhere — handler must succeed (not 500) and
	// surface a zero-streak object so the UI can hide the badge.
	h := streakTestServer(t, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily/streak", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 on empty vault, got %d", rr.Code)
	}
	var got daily.Streak
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Current != 0 || got.Longest != 0 || got.TodayLogged {
		t.Errorf("expected zero-streak on empty vault, got %+v", got)
	}
}

func TestHandleDailyStreak_RespectsConfiguredFolder(t *testing.T) {
	// User has configured DailyNotesFolder="Journal"; the handler
	// must scan THAT folder and ignore vault-root dailies. Tests
	// the integration between config.LoadForVault + jotPathRegex
	// + ComputeStreak. Without the folder respect, a user with
	// a different layout would see "Streak: 0" forever.
	today := time.Now().Local()
	dateToday := today.Format("2006-01-02")
	dateYesterday := today.AddDate(0, 0, -1).Format("2006-01-02")

	root := t.TempDir()
	jdir := filepath.Join(root, "Journal")
	if err := os.MkdirAll(jdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// In-folder dailies — these SHOULD count.
	for _, d := range []string{dateYesterday, dateToday} {
		if err := os.WriteFile(filepath.Join(jdir, d+".md"), []byte("# "+d+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Vault-root daily-shaped — must NOT count.
	if err := os.WriteFile(filepath.Join(root, "2025-01-15.md"), []byte("decoy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".granit.json"), []byte(`{"daily_notes_folder":"Journal"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	v, _ := vault.NewVault(root)
	_ = v.Scan()
	s := &Server{cfg: Config{Vault: v, Logger: slog.Default()}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily/streak", nil)
	rr := httptest.NewRecorder()
	s.handleDailyStreak(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var got daily.Streak
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Current != 2 {
		t.Errorf("Current = %d, want 2 (today + yesterday in Journal/, decoy at root must not count)", got.Current)
	}
	if !got.TodayLogged {
		t.Errorf("TodayLogged should be true")
	}
}
