package objects

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Object templates — path & frontmatter generation for new typed notes
// ---------------------------------------------------------------------------
//
// Used by both the agents package's create_object tool and the Object
// Browser's "n" key to produce identical output regardless of who's
// driving creation. Lives here in the objects package so neither side
// has to reach into the other's internals.

// PathFor returns the vault-relative path for a new object of type t with
// the given title. Substitutes the type's FilenamePattern (defaults to
// "{title}") and adds .md if the pattern doesn't already.
//
// Examples:
//
//	PathFor(Type{Folder: "People", FilenamePattern: "{title}"}, "Alice Chen")
//	  → "People/Alice Chen.md"
//	PathFor(Type{}, "Quick Note")
//	  → "Quick Note.md"
func PathFor(t Type, title string) string {
	pattern := strings.TrimSpace(t.FilenamePattern)
	if pattern == "" {
		pattern = "{title}"
	}
	filename := strings.ReplaceAll(pattern, "{title}", SanitiseFilename(title))
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}
	folder := strings.TrimSpace(t.Folder)
	if folder == "" {
		return filename
	}
	return filepath.Join(folder, filename)
}

// BuildFrontmatter constructs the YAML frontmatter block for a new object.
// Always emits `type:` and `title:` first; required properties from the
// schema follow with their default value (or empty string if no default);
// `extras` overlay/append additional values from the caller.
//
// Substitutions in default values:
//
//	{today}  →  YYYY-MM-DD (current date)
//	{now}    →  current ISO-8601 timestamp
//
// The output ends with a trailing blank line so callers can append a
// markdown body without thinking about spacing.
func BuildFrontmatter(t Type, title string, extras map[string]string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: %s\n", t.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlSingleLine(title))

	written := map[string]bool{"type": true, "title": true}

	// Required properties next so the user sees a fully-shaped header.
	for _, p := range t.Properties {
		if !p.Required || p.Name == "title" || p.Name == "type" {
			continue
		}
		if extras != nil {
			if v, ok := extras[p.Name]; ok {
				fmt.Fprintf(&b, "%s: %s\n", p.Name, yamlSingleLine(v))
				written[p.Name] = true
				continue
			}
		}
		val := substituteTemplate(p.Default)
		fmt.Fprintf(&b, "%s: %s\n", p.Name, yamlSingleLine(val))
		written[p.Name] = true
	}

	// Optional properties with non-empty defaults — saves the user from
	// re-typing common values like "saved: {today}".
	for _, p := range t.Properties {
		if written[p.Name] || p.Required || p.Name == "title" || p.Name == "type" {
			continue
		}
		if extras != nil {
			if v, ok := extras[p.Name]; ok {
				fmt.Fprintf(&b, "%s: %s\n", p.Name, yamlSingleLine(v))
				written[p.Name] = true
				continue
			}
		}
		if p.Default == "" {
			continue
		}
		val := substituteTemplate(p.Default)
		if val == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", p.Name, yamlSingleLine(val))
		written[p.Name] = true
	}

	// Anything in `extras` that wasn't a known property — append as-is so
	// callers can pass arbitrary user-supplied values.
	for k, v := range extras {
		if written[k] {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", k, yamlSingleLine(v))
	}

	b.WriteString("---\n\n")
	return b.String()
}

// SanitiseFilename strips characters that break common filesystems
// (Windows, network shares, FAT32 USB sticks) so the generated filename
// is portable. Spaces are kept — they work everywhere modern.
func SanitiseFilename(s string) string {
	s = strings.TrimSpace(s)
	for _, bad := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		s = strings.ReplaceAll(s, bad, "")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

// substituteTemplate replaces the recognised tokens in a default value.
// Currently {today} and {now} — extensible.
func substituteTemplate(v string) string {
	if v == "" {
		return v
	}
	now := time.Now()
	v = strings.ReplaceAll(v, "{today}", now.Format("2006-01-02"))
	v = strings.ReplaceAll(v, "{now}", now.Format(time.RFC3339))
	return v
}

// yamlSingleLine quotes a value for safe inclusion as a YAML scalar.
// Conservative: only quotes when bare emission would actually break a
// parser (empty, embedded newline/tab/quote, ": " mapping pattern, or
// leading reserved indicator).
func yamlSingleLine(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, "\n\t\"") {
		return strconvQuote(v)
	}
	if strings.Contains(v, ": ") {
		return strconvQuote(v)
	}
	switch v[0] {
	case '!', '&', '*', '@', '`', '|', '>', '%', '#', '?', ':', '-', '{', '[':
		return strconvQuote(v)
	}
	return v
}

// strconvQuote wraps in double quotes with backslash-escaping. We don't
// import strconv to keep the package's import surface tight — a tiny
// hand-rolled escaper handles the small set of characters we ever see.
func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
