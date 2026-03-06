package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ZettelkastenGenerator generates unique Zettelkasten-style note IDs and
// provides helpers for creating linked Zettelkasten notes.
type ZettelkastenGenerator struct {
	format  string // "timestamp", "datetime", "sequential"
	prefix  string // optional prefix e.g. "Z"
	counter int    // for sequential mode
}

// NewZettelkastenGenerator creates a new ZettelkastenGenerator with default
// settings (timestamp format, no prefix).
func NewZettelkastenGenerator() *ZettelkastenGenerator {
	return &ZettelkastenGenerator{
		format:  "timestamp",
		prefix:  "",
		counter: 0,
	}
}

// SetFormat sets the ID format. Supported formats:
//   - "timestamp"  — YYYYMMDDHHmmss (e.g. 20260306143022)
//   - "datetime"   — YYYYMMDD-HHmm (e.g. 20260306-1430)
//   - "sequential" — zero-padded sequential numbers (e.g. 0001, 0002)
func (zg *ZettelkastenGenerator) SetFormat(format string) {
	zg.format = format
}

// SetPrefix sets an optional prefix that is prepended to every generated ID.
// For example, setting "Z" produces IDs like "Z20260306143022".
func (zg *ZettelkastenGenerator) SetPrefix(prefix string) {
	zg.prefix = prefix
}

// SetCounter sets the starting counter value for sequential mode.
func (zg *ZettelkastenGenerator) SetCounter(n int) {
	zg.counter = n
}

// Generate produces a new unique ID based on the current format and prefix.
func (zg *ZettelkastenGenerator) Generate() string {
	now := time.Now()
	var id string

	switch zg.format {
	case "datetime":
		id = now.Format("20060102-1504")
	case "sequential":
		zg.counter++
		id = fmt.Sprintf("%04d", zg.counter)
	default: // "timestamp"
		id = now.Format("20060102150405")
	}

	return zg.prefix + id
}

// GenerateNoteName generates a filename in the form "ID title.md".
// For example: "20260306143022 My Note.md".
func (zg *ZettelkastenGenerator) GenerateNoteName(title string) string {
	id := zg.Generate()
	return id + " " + title + ".md"
}

// GenerateTemplate returns a full Zettelkasten note template with YAML
// frontmatter, including the generated ID, title, and structural sections
// for connections and references.
func (zg *ZettelkastenGenerator) GenerateTemplate(title string) string {
	id := zg.Generate()
	today := time.Now().Format("2006-01-02")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString("title: " + title + "\n")
	b.WriteString("date: " + today + "\n")
	b.WriteString("tags: []\n")
	b.WriteString("links: []\n")
	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString("# " + title + "\n")
	b.WriteString("\n")
	b.WriteString("## Main Idea\n")
	b.WriteString("\n")
	b.WriteString("\n")
	b.WriteString("## Connections\n")
	b.WriteString("\n")
	b.WriteString("- Related: [[]]\n")
	b.WriteString("\n")
	b.WriteString("## Source\n")
	b.WriteString("\n")
	b.WriteString("\n")
	b.WriteString("## References\n")

	return b.String()
}

// ScanExistingIDs scans the given note paths to find the highest existing
// sequential ID and returns it. This is used to avoid ID collisions when
// using sequential mode. It also updates the internal counter to the
// highest value found.
func (zg *ZettelkastenGenerator) ScanExistingIDs(paths []string) int {
	highest := 0
	for _, p := range paths {
		base := filepath.Base(p)
		base = strings.TrimSuffix(base, filepath.Ext(base))

		// Extract the first space-delimited token as the potential ID.
		parts := strings.SplitN(base, " ", 2)
		if len(parts) == 0 {
			continue
		}
		token := parts[0]

		// Strip the prefix if one is set.
		if zg.prefix != "" {
			token = strings.TrimPrefix(token, zg.prefix)
		}

		n, err := strconv.Atoi(token)
		if err != nil {
			continue
		}
		if n > highest {
			highest = n
		}
	}

	if highest > zg.counter {
		zg.counter = highest
	}
	return highest
}

// ZettelkastenSnippets returns snippet triggers that integrate the
// Zettelkasten generator with the snippet expansion system.
// Triggers:
//   - "/zk"     — inserts a generated Zettelkasten ID
//   - "/zklink" — inserts a wikilink wrapping a generated ID, e.g. [[<id>]]
//   - "/zknote" — inserts a full Zettelkasten note template
func ZettelkastenSnippets(gen *ZettelkastenGenerator) map[string]string {
	id := gen.Generate()
	template := gen.GenerateTemplate("Untitled")

	return map[string]string{
		"/zk":     id,
		"/zklink": "[[" + id + "]]",
		"/zknote": template,
	}
}
