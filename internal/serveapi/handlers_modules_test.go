package serveapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// modulesTestServer constructs a Server pointed at a tempdir vault.
// Just enough wiring to drive the modules handlers — no auth, no
// watcher, no file server.
func modulesTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.Default()
	s := &Server{
		cfg: Config{
			Vault:     v,
			TaskStore: store,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
	return s, root
}

func TestHandleListModules_Shape(t *testing.T) {
	s, _ := modulesTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/modules", nil)
	rr := httptest.NewRecorder()
	s.handleListModules(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var got modulesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	// Baseline registers 10 module declarations (the 8 new IDs +
	// chat + habit_tracker). If that count changes deliberately,
	// update this expectation.
	if len(got.Modules) < 5 {
		t.Errorf("expected several baseline modules, got %d", len(got.Modules))
	}
	have := map[string]bool{}
	for _, m := range got.Modules {
		have[m.ID] = true
		if m.Name == "" {
			t.Errorf("module %q has empty name", m.ID)
		}
		// New (unsaved) modules default to enabled — that's the
		// migration-safety semantic in modules.Registry.Enabled.
		if !m.Enabled {
			t.Errorf("module %q should default to enabled", m.ID)
		}
	}
	for _, want := range []string{"goals", "projects", "habit_tracker", "deadlines", "scripture", "morning", "jots", "agents", "objects", "chat"} {
		if !have[want] {
			t.Errorf("missing baseline module %q", want)
		}
	}
	// CoreIDs should always include the four immutable surfaces.
	coreHave := map[string]bool{}
	for _, c := range got.CoreIDs {
		coreHave[c.ID] = true
	}
	for _, want := range []string{"notes", "tasks", "calendar", "settings"} {
		if !coreHave[want] {
			t.Errorf("missing core ID %q", want)
		}
	}
}

func TestHandlePutModules_PersistsAcrossReboot(t *testing.T) {
	s, root := modulesTestServer(t)

	body := putModulesRequest{Enabled: map[string]bool{"jots": false}}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/modules", bytes.NewReader(buf))
	rr := httptest.NewRecorder()
	s.handlePutModules(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status %d body %s", rr.Code, rr.Body.String())
	}
	// Echoed list should reflect the change.
	var after modulesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode put response: %v body=%s", err, rr.Body.String())
	}
	gotJots := lookupModule(after.Modules, "jots")
	if gotJots == nil {
		t.Fatal("jots missing from response")
	}
	if gotJots.Enabled {
		t.Error("jots should be disabled after PUT")
	}

	// Spin up a fresh Server pointed at the same vault — the on-disk
	// modules.json must propagate the disable.
	v2, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.Scan(); err != nil {
		t.Fatal(err)
	}
	store2, _ := tasks.Load(root, func() []tasks.NoteContent { return nil })
	logger := slog.Default()
	s2 := &Server{
		cfg: Config{
			Vault:     v2,
			TaskStore: store2,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/modules", nil)
	rr2 := httptest.NewRecorder()
	s2.handleListModules(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET-after-restart status %d body %s", rr2.Code, rr2.Body.String())
	}
	var rebooted modulesResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &rebooted); err != nil {
		t.Fatalf("decode reboot: %v", err)
	}
	rebJots := lookupModule(rebooted.Modules, "jots")
	if rebJots == nil {
		t.Fatal("jots missing after restart")
	}
	if rebJots.Enabled {
		t.Error("jots should remain disabled across server restart")
	}
}

func TestHandlePutModules_IgnoresCoreIDs(t *testing.T) {
	s, _ := modulesTestServer(t)
	// Try to disable a core ID — the handler should silently strip
	// it, persist nothing, and the GET should still report core
	// surfaces as toggle-free.
	body := putModulesRequest{Enabled: map[string]bool{"notes": false, "jots": false}}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/modules", bytes.NewReader(buf))
	rr := httptest.NewRecorder()
	s.handlePutModules(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status %d body %s", rr.Code, rr.Body.String())
	}
	// jots disabled, notes still absent from the registry's enabled
	// map (and absent from the modules list because it's a core ID).
	var got modulesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m := lookupModule(got.Modules, "notes"); m != nil {
		t.Errorf("notes should not appear in modules list (core), got %+v", m)
	}
	if m := lookupModule(got.Modules, "jots"); m == nil || m.Enabled {
		t.Errorf("jots should be present and disabled, got %+v", m)
	}
}

func lookupModule(list []moduleEntry, id string) *moduleEntry {
	for i, m := range list {
		if m.ID == id {
			return &list[i]
		}
	}
	return nil
}
