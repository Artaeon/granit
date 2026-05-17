package serveapi

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

// Concept-graph endpoint. Returns the vault as nodes (notes) + edges
// (wikilinks), pre-shaped for a force-directed view on the client.
//
// Why this lives here and not behind the existing /links surface:
// /links is per-note (incoming + outgoing for ONE path), used by the
// editor's backlinks panel. The graph view needs the whole web at
// once, with degree and tag/folder metadata baked in so the client
// can size + colour nodes without N round-trips. A separate endpoint
// keeps the per-note shape lean and lets this one carry the global
// indexing cost only when the user actually opens /notes/graph.

type graphNode struct {
	ID     string   `json:"id"`     // canonical relpath, same key the rest of /notes uses
	Title  string   `json:"title"`
	Path   string   `json:"path"`
	Degree int      `json:"degree"`           // incoming + outgoing wikilink count (after dedupe)
	Tags   []string `json:"tags,omitempty"`
}

type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func (s *Server) handleNotesGraph(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tagF := q.Get("tag")
	folderF := strings.TrimSuffix(q.Get("folder"), "/")
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 300
	}

	notes := s.cfg.Vault.SnapshotNotes()

	// Build a title→relpath map so outgoing wikilinks (which are stored
	// as bare titles, not paths) can be resolved without re-walking the
	// vault per link. Mirrors what vault.Index.Build does internally;
	// duplicating the resolution here avoids hard-coding a dependency
	// on a built Index (serveapi doesn't hold one — n.Backlinks isn't
	// reliably populated under the web server's scan path).
	byTitle := make(map[string]string, len(notes))
	byBase := make(map[string]string, len(notes))
	for relPath := range notes {
		base := filepath.Base(relPath)
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		// Prefer the first occurrence (Index.Build does the same) so
		// duplicate basenames in different folders resolve stably.
		if _, ok := byBase[noExt]; !ok {
			byBase[noExt] = relPath
		}
	}
	for relPath, n := range notes {
		if n.Title != "" {
			if _, ok := byTitle[n.Title]; !ok {
				byTitle[n.Title] = relPath
			}
		}
	}

	resolveLink := func(link string) string {
		if hashIdx := strings.Index(link, "#"); hashIdx >= 0 {
			link = link[:hashIdx]
		}
		if link == "" {
			return ""
		}
		// Direct path match (with .md).
		probe := link
		if !strings.HasSuffix(probe, ".md") {
			probe = probe + ".md"
		}
		if _, ok := notes[probe]; ok {
			return probe
		}
		// Title match — the form wikilinks usually take.
		if p, ok := byTitle[link]; ok {
			return p
		}
		// Basename match (no extension).
		base := strings.TrimSuffix(filepath.Base(link), filepath.Ext(link))
		if p, ok := byBase[base]; ok {
			return p
		}
		return ""
	}

	// Stage 1: build the FULL edge set + per-node degree across the
	// entire vault, BEFORE folder/tag filters. We need global degree
	// for the "prefer high-degree nodes when clipping to limit" rule
	// to be meaningful — clipping by a degree value that was already
	// reduced to the filtered subgraph would be self-referential.
	type edgeKey struct{ a, b string } // canonicalised so a < b
	edgeSet := make(map[edgeKey]struct{}, len(notes))
	degree := make(map[string]int, len(notes))
	addEdge := func(src, tgt string) {
		if src == "" || tgt == "" || src == tgt {
			return
		}
		// Canonicalise so {src,tgt} and {tgt,src} hash to the same
		// key — backlinks would otherwise mirror every outgoing link
		// into a second edge.
		a, b := src, tgt
		if a > b {
			a, b = b, a
		}
		k := edgeKey{a, b}
		if _, ok := edgeSet[k]; ok {
			return
		}
		edgeSet[k] = struct{}{}
		degree[src]++
		degree[tgt]++
	}
	for relPath, n := range notes {
		// We need n.Links populated; ScanFast leaves it empty. Pay the
		// load cost lazily here — first /graph hit on a cold vault
		// reads every .md, but subsequent hits hit the in-memory cache.
		s.cfg.Vault.EnsureLoaded(relPath)
		for _, link := range n.Links {
			tgt := resolveLink(link)
			if tgt == "" {
				continue
			}
			addEdge(relPath, tgt)
		}
	}

	// Stage 2: apply tag/folder filter, expanded to direct neighbours.
	// "Direct neighbours" means: if a tagged/folder-matched note links
	// to (or is linked from) a note outside the filter, that neighbour
	// is included so the user sees the bridge. Without that, the
	// filter would carve a hole in the visible structure.
	tagSet := tagF != ""
	folderSet := folderF != ""
	included := make(map[string]bool, len(notes))
	if !tagSet && !folderSet {
		for p := range notes {
			included[p] = true
		}
	} else {
		seed := make(map[string]bool, len(notes))
		for relPath, n := range notes {
			if folderSet && !strings.HasPrefix(relPath, folderF+"/") {
				continue
			}
			if tagSet {
				has := false
				for _, t := range tagsFor(n) {
					if t == tagF {
						has = true
						break
					}
				}
				if !has {
					continue
				}
			}
			seed[relPath] = true
		}
		for p := range seed {
			included[p] = true
		}
		// Walk edges, pulling in neighbours of any seed node.
		for k := range edgeSet {
			if seed[k.a] {
				included[k.b] = true
			}
			if seed[k.b] {
				included[k.a] = true
			}
		}
	}

	// Stage 3: rank by degree, clip to limit. Higher-degree first so
	// the visible structure is the anchored part of the web rather
	// than 300 random orphans.
	candidates := make([]string, 0, len(included))
	for p := range included {
		candidates = append(candidates, p)
	}
	sort.Slice(candidates, func(i, j int) bool {
		di, dj := degree[candidates[i]], degree[candidates[j]]
		if di != dj {
			return di > dj
		}
		// Stable tiebreaker so the response is deterministic across
		// repeated calls — useful for the client cache + tests.
		return candidates[i] < candidates[j]
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	keep := make(map[string]bool, len(candidates))
	for _, p := range candidates {
		keep[p] = true
	}

	// Stage 4: emit. Node degree exposed to the client is the GLOBAL
	// degree (computed pre-clip) so the visual size of a node matches
	// "how connected it really is" even when half its neighbours got
	// clipped. Edges, conversely, must be restricted to the visible
	// set or we'd serialise dangling references the client couldn't
	// render.
	nodes := make([]graphNode, 0, len(candidates))
	for _, p := range candidates {
		n := notes[p]
		if n == nil {
			continue
		}
		nodes = append(nodes, graphNode{
			ID:     p,
			Title:  noteTitleOrPath(n),
			Path:   p,
			Degree: degree[p],
			Tags:   tagsFor(n),
		})
	}
	edges := make([]graphEdge, 0, len(edgeSet))
	for k := range edgeSet {
		if !keep[k.a] || !keep[k.b] {
			continue
		}
		edges = append(edges, graphEdge{Source: k.a, Target: k.b})
	}
	// Deterministic edge order — cheap and helps the client treat
	// repeated fetches as identical (no spurious diff highlight).
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Target < edges[j].Target
	})

	writeJSON(w, http.StatusOK, graphResponse{Nodes: nodes, Edges: edges})
}

// noteTitleOrPath falls back to the relpath when a note's parsed
// title is empty — usually because frontmatter overrode it with ""
// or the file is content-less. The frontend uses Title for tooltip +
// click target so a non-empty string is required.
func noteTitleOrPath(n *vault.Note) string {
	if n.Title != "" {
		return n.Title
	}
	return n.RelPath
}
