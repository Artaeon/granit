package serveapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/textutil"
)

type searchHit struct {
	Path      string  `json:"path"`
	Title     string  `json:"title"`
	Line      int     `json:"line"`
	Column    int     `json:"column"`
	MatchLine string  `json:"matchLine"`
	Score     float64 `json:"score"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"results": []searchHit{}, "ready": s.search.IsReady()})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	raws := s.search.Search(q)
	if len(raws) > limit {
		raws = raws[:limit]
	}
	out := make([]searchHit, 0, len(raws))
	for _, h := range raws {
		title := h.Path
		if n := s.cfg.Vault.GetNote(h.Path); n != nil {
			title = n.Title
		}
		out = append(out, searchHit{
			Path:      h.Path,
			Title:     title,
			Line:      h.Line,
			Column:    h.Column,
			MatchLine: trimMatchLine(h.MatchLine, q),
			Score:     h.Score,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": out,
		"total":   len(out),
		"q":       q,
		"ready":   s.search.IsReady(),
	})
}

// trimMatchLine returns a snippet around the first match, capped to ~160 bytes.
// Both ends are rounded to UTF-8 rune boundaries so a German / Greek / CJK
// note never loses a char to a mid-codepoint cut (continuation bytes have
// the high bits `10`; everything else starts a rune).
func trimMatchLine(line, q string) string {
	if line == "" {
		return ""
	}
	idx := strings.Index(strings.ToLower(line), strings.ToLower(strings.Fields(q)[0]))
	if idx < 0 {
		return textutil.TruncateRunes(line, 160)
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	// Round start DOWN to the nearest rune-start byte. A byte is a
	// UTF-8 continuation iff its top two bits are 10 (i.e. `b & 0xC0
	// == 0x80`). Walking back while that's true lands us on the
	// previous rune's leading byte.
	for start > 0 && (line[start]&0xC0) == 0x80 {
		start--
	}
	end := start + 160
	if end > len(line) {
		end = len(line)
	}
	// Round end UP to the next rune-start byte by the same trick.
	for end < len(line) && (line[end]&0xC0) == 0x80 {
		end++
	}
	out := line[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(line) {
		out += "…"
	}
	return out
}
