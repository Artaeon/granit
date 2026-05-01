package serveapi

import (
	"net/http"
	"sort"
	"strings"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	counts := map[string]int{}
	for _, n := range s.cfg.Vault.SnapshotNotes() {
		for _, t := range tagsFor(n) {
			counts[t]++
		}
	}
	type tagRow struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	rows := make([]tagRow, 0, len(counts))
	for t, c := range counts {
		rows = append(rows, tagRow{Tag: t, Count: c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Tag < rows[j].Tag
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tags":  rows,
		"total": len(rows),
	})
}

// keep strings import used
var _ = strings.ToLower
