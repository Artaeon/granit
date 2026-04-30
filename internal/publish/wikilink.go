package publish

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// resolveWikilinks rewrites [[target]] and [[target|display]] occurrences
// in markdown source to standard markdown links pointing at the resolved
// note's HTML output. Unresolved targets stay as plain bracketed text so
// the user can spot them visually.
//
// The notes/ prefix is hard-coded relative to the note's own page (which
// lives at notes/{slug}.html), so links are always sibling refs — works
// fine when the site is hosted at any subpath including /username.github.io
// or username.github.io/repo/. Returns the rewritten body and a slice of
// RelPaths that were successfully linked (used to compute backlinks).
//
// Resolver semantics: returns (url, displayTitle, ok). The url is relative
// to the source note's location.
func resolveWikilinks(
	body string,
	resolver func(target string) (url, title string, ok bool),
	bySlug map[string]*Note,
	byTitle map[string]*Note,
) (string, []string) {
	var outlinks []string
	seen := map[string]bool{}

	rewritten := reWikiLinkExt.ReplaceAllStringFunc(body, func(m string) string {
		sub := reWikiLinkExt.FindStringSubmatch(m)
		target := strings.TrimSpace(sub[1])
		display := target
		if len(sub) >= 3 && sub[2] != "" {
			display = strings.TrimSpace(strings.TrimPrefix(sub[2], "|"))
		}
		// Strip a #section anchor for the resolver lookup; re-attach to
		// the URL after we have a hit.
		anchor := ""
		if i := strings.Index(target, "#"); i >= 0 {
			anchor = target[i:]
			target = target[:i]
		}
		url, title, ok := resolver(target)
		if !ok {
			// Unresolved — leave as a faint plain-text reference so it's
			// obvious the link is broken without 404-ing the build.
			return display
		}
		// Track for backlinks. Reverse-lookup: which Note RelPath does this
		// resolve to?
		var relpath string
		key := strings.ToLower(target)
		if n, hit := byTitle[key]; hit {
			relpath = n.RelPath
		} else if n, hit := bySlug[slugify(target)]; hit {
			relpath = n.RelPath
		}
		if relpath != "" && !seen[relpath] {
			seen[relpath] = true
			outlinks = append(outlinks, relpath)
		}
		// If the wikilink had no |display, prefer the resolved note's
		// canonical title — keeps published pages tidy when the source
		// uses lowercase or shorthand link targets.
		if display == strings.TrimSpace(sub[1]) && title != "" {
			display = title
		}
		return "[" + display + "](" + url + anchor + ")"
	})
	return rewritten, outlinks
}

// reWikiLinkExt matches [[target]] and [[target|display]] with optional
// section anchors target#section. Tolerant of whitespace inside the
// brackets so [[ Note Name ]] still resolves.
var reWikiLinkExt = regexp.MustCompile(`\[\[([^\]|]+)(\|[^\]]+)?\]\]`)

// unmarshalYAML wraps the yaml.v2 parser, returned as a tiny helper so the
// build.go entry point doesn't pull yaml.v2 into its imports directly. Keeps
// the YAML choice swappable to yaml.v3 later if we want richer types.
func unmarshalYAML(data []byte, out *map[string]interface{}) error {
	// yaml.v2 returns map[interface{}]interface{} which is awkward to use
	// downstream — convert to map[string]interface{} so callers can do
	// `m["title"].(string)` cleanly.
	var raw map[interface{}]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		ks, ok := k.(string)
		if !ok {
			continue
		}
		(*out)[ks] = convertYAMLValue(v)
	}
	return nil
}

func convertYAMLValue(v interface{}) interface{} {
	switch t := v.(type) {
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = convertYAMLValue(item)
		}
		return out
	case map[interface{}]interface{}:
		out := map[string]interface{}{}
		for k, v := range t {
			if ks, ok := k.(string); ok {
				out[ks] = convertYAMLValue(v)
			}
		}
		return out
	default:
		return v
	}
}
