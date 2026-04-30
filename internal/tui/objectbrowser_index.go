package tui

import (
	"strings"

	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/vault"
)

// rebuildObjectsIndex is the bridge from the vault layer to the
// objects package. It walks every loaded note, parses its
// frontmatter into a flat map, and feeds the result into an
// objects.Builder. The resulting Index is what the Object Browser
// renders against.
//
// We deliberately re-parse frontmatter here rather than depend on
// the vault package storing it — keeps internal/objects free of
// any vault import. Frontmatter parsing is shallow (key: value
// only) which matches the typed-objects mental model: anything
// fancier (nested maps, multiline strings) belongs in the note
// body, not the frontmatter.
//
// Cost is O(N) over notes with O(L) per note where L is the
// frontmatter line count. On a 1000-note vault that's a few ms —
// well below the threshold where caching pays off.
func rebuildObjectsIndex(reg *objects.Registry, v *vault.Vault) *objects.Index {
	if reg == nil || v == nil {
		return objects.NewIndex()
	}
	b := objects.NewBuilder(reg)
	for _, note := range v.Notes {
		fm := parseFlatFrontmatter(note.Content)
		title := fm["title"]
		if title == "" {
			title = noteTitleFallback(note.RelPath, note.Content)
		}
		b.Add(note.RelPath, title, fm)
	}
	return b.Finalize()
}

// parseFlatFrontmatter pulls a YAML-ish key/value map from the top of
// a markdown body, between leading "---" delimiters. Only single-line
// `key: value` entries are recognised — array literals and nested
// maps are skipped (the typed-objects schema covers what UI surfaces
// need, and richer YAML can live in the body).
//
// Tolerates surrounding quotes on the value. Strips inline `  # ...`
// comments. When a key has an empty value AND the next non-blank
// line is an indented continuation (array or nested map), the key
// is treated as a multi-line YAML construct and dropped — without
// this, `tags:\n  - foo` produced `tags=""` in the map and showed
// up as a stray empty column in the Object Browser.
//
// Returns an empty (non-nil) map when no frontmatter is present so
// callers don't need a nil check.
func parseFlatFrontmatter(body string) map[string]string {
	out := map[string]string{}
	if !strings.HasPrefix(body, "---\n") && !strings.HasPrefix(body, "---\r\n") {
		return out
	}
	rest := body[4:]
	if strings.HasPrefix(body, "---\r\n") {
		rest = body[5:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return out
	}
	front := rest[:end]
	lines := strings.Split(front, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// Skip YAML continuation lines (array items, indented
		// nested-map keys). These should already be subsumed by
		// the parent key's "empty value → skip" branch below;
		// the explicit skip here belt-and-braces against odd
		// frontmatter shapes.
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Empty value + indented next line = multi-line YAML
		// (array, nested map). Skip the key entirely so the
		// flat map doesn't carry a stray empty entry that
		// pollutes downstream gallery columns.
		if value == "" {
			next := i + 1
			for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
				next++
			}
			if next < len(lines) {
				nl := lines[next]
				if strings.HasPrefix(nl, "  ") || strings.HasPrefix(nl, "\t") || strings.HasPrefix(nl, "- ") {
					continue
				}
			}
		}

		// Strip surrounding quotes.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		// Strip trailing inline comments after a "  #" run. Naive
		// split — won't handle hash-in-string but Capacities-style
		// types rarely need that.
		if hashIdx := strings.Index(value, "  #"); hashIdx >= 0 {
			value = strings.TrimSpace(value[:hashIdx])
		}
		out[key] = value
	}
	return out
}

// noteTitleFallback returns a display title for a note that doesn't
// declare `title:` in frontmatter. Order: first H1 in body, else
// filename without extension. Mirrors what most markdown editors do
// in their breadcrumb / tab title.
func noteTitleFallback(relPath, body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "#"))
		}
	}
	// Filename without extension.
	base := relPath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[:i]
	}
	return base
}
