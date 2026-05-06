package serveapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/atomicio"
)

// printConfig captures the per-vault default header / footer / mode
// for the note print preview. Stored at .granit/print-config.json so
// the values survive across browsers and devices — localStorage
// alone meant a user who set "ACME — Internal" once on their
// desktop had to re-key it on their phone.
//
// The shape mirrors the localStorage keys the web overlay used
// before this endpoint existed (granit.print.header / footer /
// mode), so the migration is a one-time prefer-server-then-fallback
// read on the client side.
//
// Mode is validated to one of standard / certificate / report on
// write — anything else is silently coerced to "standard". Header
// and footer are free text; an empty string is the documented
// "no header / no footer" signal (the renderer falls back to a
// today-date footer when both are empty, but only if the user has
// never written a footer of their own).
type printConfig struct {
	Header string `json:"header"`
	Footer string `json:"footer"`
	Mode   string `json:"mode"`
}

// printConfigPath returns the absolute path to the per-vault config
// file. We don't pre-create the .granit directory — atomicio handles
// MkdirAll on write, and a missing file just means "no defaults yet"
// on read.
func printConfigPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "print-config.json")
}

// validMode returns m if it's a recognised print mode, otherwise
// "standard". Keeps the front-end's mode set honest — if a future
// build adds a new mode and an old client tries to write it, we
// silently downgrade rather than crashing the renderer. Adding a
// new mode requires both a client-side template AND adding it
// here; older clients that don't know about a new mode will see
// "standard" until they update.
func validMode(m string) string {
	switch m {
	case "standard", "certificate", "report", "letterhead", "memo":
		return m
	}
	return "standard"
}

func (s *Server) loadPrintConfig() printConfig {
	out := printConfig{Mode: "standard"}
	data, err := os.ReadFile(printConfigPath(s.cfg.Vault.Root))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	out.Mode = validMode(out.Mode)
	return out
}

func (s *Server) savePrintConfig(c printConfig) error {
	c.Mode = validMode(c.Mode)
	c.Header = strings.TrimRight(c.Header, "\n")
	c.Footer = strings.TrimRight(c.Footer, "\n")
	path := printConfigPath(s.cfg.Vault.Root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}

func (s *Server) handleGetPrintConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.loadPrintConfig())
}

func (s *Server) handlePutPrintConfig(w http.ResponseWriter, r *http.Request) {
	var body printConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.savePrintConfig(body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.loadPrintConfig())
}
