package serveapi

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// urlParam reads a chi URL parameter and PathUnescape's it. chi
// doesn't decode percent-encoded path params on its own, so a name
// like "deep/work" or an ID with a "+" round-trips broken if a
// handler reads chi.URLParam directly.
//
// Forgiving on bad escapes: a malformed "%" returns the raw value
// rather than dropping the request, so handlers that compare
// against a clean ID don't 404 on a quirky encoder upstream.
func urlParam(r *http.Request, name string) string {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return ""
	}
	if dec, err := url.PathUnescape(raw); err == nil {
		return dec
	}
	return raw
}
