package publish

import (
	"regexp"
	"strings"
)

// outlineEntry is one heading in a note, used to build the right-rail
// "Contents" panel on note pages. Level is the markdown heading depth
// (2 for ##, 3 for ###); H1 is excluded because it's already the page
// title and including it would duplicate the visual focal point.
type outlineEntry struct {
	Level int
	Text  string
	Slug  string // anchor target — must match goldmark's auto-id slug
}

// extractOutline scans markdown body for ## / ### / #### headings and
// returns them in document order. Skips H1 (page title) and anything
// deeper than H4 (too noisy for a sidebar). The slugs are computed the
// same way goldmark's auto-id extension does so the anchors match.
func extractOutline(body string) []outlineEntry {
	var out []outlineEntry
	for _, line := range strings.Split(body, "\n") {
		m := reOutlineH.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		level := len(m[1])
		if level < 2 || level > 4 {
			continue
		}
		text := strings.TrimSpace(m[2])
		out = append(out, outlineEntry{
			Level: level,
			Text:  text,
			Slug:  goldmarkAutoID(text),
		})
	}
	return out
}

var reOutlineH = regexp.MustCompile(`^(#{2,4})\s+(.+?)\s*#*\s*$`)

// goldmarkAutoID approximates the slug that goldmark's auto-id extension
// generates: lowercase, replace non-alphanumeric runs with single hyphens,
// trim leading/trailing hyphens. Using the same algorithm here keeps the
// outline links working without coordinating with goldmark internals.
var reAutoIDSep = regexp.MustCompile(`[^a-z0-9]+`)

func goldmarkAutoID(s string) string {
	s = strings.ToLower(s)
	s = reAutoIDSep.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
