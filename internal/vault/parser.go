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

func ParseFrontmatter(content string) map[string]interface{} {
	fm := make(map[string]interface{})
	if !strings.HasPrefix(content, "---") {
		return fm
	}

	end := strings.Index(content[3:], "---")
	if end == -1 {
		return fm
	}

	block := content[3 : 3+end]
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
	if !strings.HasPrefix(content, "---") {
		return content
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return content
	}
	return strings.TrimSpace(content[3+end+3:])
}
