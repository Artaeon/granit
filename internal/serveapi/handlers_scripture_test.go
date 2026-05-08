package serveapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/scripture"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// scriptureTestServer wires up just enough of Server to exercise the
// scripture handlers — no auth, no watcher, no file server. Vault root
// is a tempdir so Load() returns the bundled defaults.
func scriptureTestServer(t *testing.T) *Server {
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
	return &Server{
		cfg: Config{
			Vault:     v,
			TaskStore: store,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
}

func TestHandleListScriptures_IncludesTopics(t *testing.T) {
	s := scriptureTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scripture", nil)
	rr := httptest.NewRecorder()
	s.handleListScriptures(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Scriptures []scripture.Scripture   `json:"scriptures"`
		Total      int                     `json:"total"`
		Topics     []scripture.TopicCount  `json:"topics"`
		Topic      string                  `json:"topic"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if got.Total < 50 {
		t.Errorf("expected at least 50 verses in catalogue, got %d", got.Total)
	}
	if len(got.Topics) < 5 {
		t.Errorf("expected several topics, got %d", len(got.Topics))
	}
	if got.Topic != "" {
		t.Errorf("topic should be empty when no filter requested, got %q", got.Topic)
	}
}

func TestHandleListScriptures_TopicFilter(t *testing.T) {
	s := scriptureTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scripture?topic=love", nil)
	rr := httptest.NewRecorder()
	s.handleListScriptures(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Scriptures []scripture.Scripture `json:"scriptures"`
		Total      int                   `json:"total"`
		Topic      string                `json:"topic"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if got.Topic != "love" {
		t.Errorf("expected topic=love echoed back, got %q", got.Topic)
	}
	if got.Total == 0 {
		t.Fatal("expected verses tagged 'love'")
	}
	for _, sv := range got.Scriptures {
		ok := false
		for _, tag := range sv.Topics {
			if tag == "love" {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("scripture %q lacks 'love' topic; tags=%v", sv.Source, sv.Topics)
		}
	}
}

func TestHandleScriptureTopics(t *testing.T) {
	s := scriptureTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scripture/topics", nil)
	rr := httptest.NewRecorder()
	s.handleScriptureTopics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Topics []scripture.TopicCount `json:"topics"`
		Total  int                    `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if got.Total != len(got.Topics) {
		t.Errorf("total %d != len %d", got.Total, len(got.Topics))
	}
	if got.Total < 5 {
		t.Errorf("expected several topics, got %d", got.Total)
	}
}
