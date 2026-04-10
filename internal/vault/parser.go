package vault

import (
	"regexp"
	"strings"
)

var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

func ParseWikiLinks(content string) []string {
	matches := wikiLinkRegex.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var links []string
	for _, match := range matches {
		if len(match) >= 2 {
			link := strings.TrimSpace(match[1])
			if !seen[link] {
				links = append(links, link)
				seen[link] = true
			}
		}
	}
	return links
}

// frontmatterBounds locates the opening and closing fences of a YAML
// frontmatter block. It only considers content that starts with "---\n"
// (a literal opening fence followed by a newline) and requires the block
// between the fences to contain at least one "key: value" line. Returns
// (blockStart, blockEnd, bodyStart, true) where content[blockStart:blockEnd]
// is the YAML body between the fences and content[bodyStart:] is everything
// after the closing fence. Returns ok=false if no real frontmatter is present.
//
// The "at least one key:value" requirement is what prevents StripFrontmatter
// from eating content in notes that legitimately begin with a "---" horizontal
// rule and happen to contain another "---" further down.
func frontmatterBounds(content string) (blockStart, blockEnd, bodyStart int, ok bool) {
	if !strings.HasPrefix(content, "---\n") {
		return 0, 0, 0, false
	}
	blockStart = 4 // skip "---\n"
	// Find a closing fence that is "\n---\n" or "\n---" at end-of-string.
	rest := content[blockStart:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return 0, 0, 0, false
	}
	blockEnd = blockStart + idx + 1 // include the trailing newline of the YAML body
	afterClose := blockStart + idx + len("\n---")
	// Closing fence must be followed by newline or be at EOF.
	switch {
	case afterClose == len(content):
		bodyStart = afterClose
	case content[afterClose] == '\n':
		bodyStart = afterClose + 1
	default:
		return 0, 0, 0, false
	}
	// Require at least one valid key:value line so a pair of "---"
	// horizontal rules around prose is not mistaken for frontmatter.
	if !hasFrontmatterKV(content[blockStart:blockEnd]) {
		return 0, 0, 0, false
	}
	return blockStart, blockEnd, bodyStart, true
}

// hasFrontmatterKV reports whether the given block contains at least one
// "key: value" line where the key is a non-empty identifier-style token.
func hasFrontmatterKV(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		if key == "" {
			continue
		}
		// Key must look like a YAML identifier (letters, digits, _ , -).
		valid := true
		for _, r := range key {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_' || r == '-') {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func ParseFrontmatter(content string) map[string]interface{} {
	fm := make(map[string]interface{})
	blockStart, blockEnd, _, ok := frontmatterBounds(content)
	if !ok {
		return fm
	}

	block := content[blockStart:blockEnd]
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Handle YAML arrays (simple case: comma-separated in brackets)
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				inner := value[1 : len(value)-1]
				items := strings.Split(inner, ",")
				trimmed := make([]string, 0, len(items))
				for _, item := range items {
					trimmed = append(trimmed, strings.TrimSpace(item))
				}
				fm[key] = trimmed
			} else {
				fm[key] = value
			}
		}
	}
	return fm
}

func StripFrontmatter(content string) string {
	_, _, bodyStart, ok := frontmatterBounds(content)
	if !ok {
		return content
	}
	return strings.TrimSpace(content[bodyStart:])
}
