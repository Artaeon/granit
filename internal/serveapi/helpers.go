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

// urlParam reads a chi path parameter and URL-decodes it. Chi extracts
// the raw segment ("foo%2Fbar") without decoding, which silently
// breaks lookups when the underlying entity name contains characters
// that the client had to percent-encode (most commonly "/"). Always
// use this helper instead of chi.URLParam for entity keys that come
// from user content.
func urlParam(r *http.Request, name string) string {
	raw := chi.URLParam(r, name)
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}
