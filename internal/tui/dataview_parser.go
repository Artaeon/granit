package tui

import (
	"strconv"
	"strings"
)

// ═══════════════════════════════════════════════════════════════════════════
// Dataview Query Parser — tokenizes and parses SQL-like query strings
// ═══════════════════════════════════════════════════════════════════════════

// DVQueryMode is the output format of a dataview query.
type DVQueryMode int

const (
	DVModeTable DVQueryMode = iota
	DVModeList
	DVModeTask
)

// DVCondition is a single WHERE predicate.
type DVCondition struct {
	Field    string // frontmatter key or virtual field
	Op       string // =, !=, >, <, >=, <=, CONTAINS
	Value    string // comparison value
	Negate   bool   // true for !completed style
}

// DVSort describes a sort directive.
type DVSort struct {
	Field string
	Desc  bool
}

// DVParsedQuery is the fully parsed result of a dataview query string.
type DVParsedQuery struct {
	Mode       DVQueryMode
	Fields     []string      // TABLE column fields (empty for LIST/TASK)
	Source     string        // folder path or "" for entire vault
	SourceTag  string        // #tag source filter
	Conditions []DVCondition // WHERE conditions
	Sort       *DVSort       // SORT clause (nil if not specified)
	Limit      int           // LIMIT clause (0 = no limit)
	RawQuery   string        // original query text
}

// ParseDVQuery parses a dataview query string into a DVParsedQuery.
//
// Supported syntax:
//
//	TABLE field1, field2 FROM "folder" WHERE field CONTAINS "value" SORT field DESC LIMIT 10
//	LIST FROM #tag WHERE date >= 2024-01-01 SORT date DESC
//	TASK FROM "projects" WHERE !completed
func ParseDVQuery(raw string) *DVParsedQuery {
	q := &DVParsedQuery{
		Mode:     DVModeTable,
		RawQuery: raw,
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return q
	}

	tokens := tokenizeDV(raw)
	if len(tokens) == 0 {
		return q
	}

	pos := 0

	// Parse mode keyword
	first := strings.ToUpper(tokens[pos])
	switch first {
	case "TABLE":
		q.Mode = DVModeTable
		pos++
	case "LIST":
		q.Mode = DVModeList
		pos++
	case "TASK":
		q.Mode = DVModeTask
		pos++
	default:
		// Default to TABLE if no mode keyword
		q.Mode = DVModeTable
	}

	// For TABLE mode, parse field list before FROM
	if q.Mode == DVModeTable && pos < len(tokens) {
		q.Fields, pos = parseFieldList(tokens, pos)
	}

	// Parse FROM clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "FROM" {
		pos++
		if pos < len(tokens) {
			src := tokens[pos]
			if strings.HasPrefix(src, "#") {
				q.SourceTag = src[1:]
			} else {
				q.Source = unquote(src)
			}
			pos++
		}
	}

	// Parse WHERE clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "WHERE" {
		pos++
		q.Conditions, pos = parseConditions(tokens, pos)
	}

	// Parse SORT clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "SORT" {
		pos++
		if pos < len(tokens) {
			s := DVSort{Field: tokens[pos]}
			pos++
			if pos < len(tokens) {
				dir := strings.ToUpper(tokens[pos])
				if dir == "DESC" {
					s.Desc = true
					pos++
				} else if dir == "ASC" {
					pos++
				}
			}
			q.Sort = &s
		}
	}

	// Parse LIMIT clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "LIMIT" {
		pos++
		if pos < len(tokens) {
			if n, err := strconv.Atoi(tokens[pos]); err == nil && n > 0 {
				q.Limit = n
			}
		}
	}

	return q
}

// tokenizeDV splits a query string into tokens, respecting quoted strings.
// Commas between field names are consumed as separators.
func tokenizeDV(s string) []string {
	var tokens []string
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		// Skip whitespace
		if runes[i] == ' ' || runes[i] == '\t' {
			i++
			continue
		}

		// Skip standalone commas (field separators)
		if runes[i] == ',' {
			i++
			continue
		}

		// Quoted string
		if runes[i] == '"' || runes[i] == '\'' {
			quote := runes[i]
			i++
			start := i
			for i < len(runes) && runes[i] != quote {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			if i < len(runes) {
				i++ // skip closing quote
			}
			continue
		}

		// Comparison operators (>=, <=, !=)
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			if two == ">=" || two == "<=" || two == "!=" {
				tokens = append(tokens, two)
				i += 2
				continue
			}
		}

		// Single-char operators
		if runes[i] == '=' || runes[i] == '>' || runes[i] == '<' {
			tokens = append(tokens, string(runes[i]))
			i++
			continue
		}

		// Regular word (includes #tags, field names, etc.)
		start := i
		for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' &&
			runes[i] != ',' && runes[i] != '"' && runes[i] != '\'' &&
			runes[i] != '=' && runes[i] != '>' && runes[i] != '<' &&
			(i+1 >= len(runes) || runes[i] != '!' || runes[i+1] != '=') {
			i++
		}
		if i > start {
			tokens = append(tokens, string(runes[start:i]))
		}
	}

	return tokens
}

// parseFieldList extracts comma-separated field names, stopping at a keyword.
func parseFieldList(tokens []string, pos int) ([]string, int) {
	var fields []string

	for pos < len(tokens) {
		upper := strings.ToUpper(tokens[pos])
		if upper == "FROM" || upper == "WHERE" || upper == "SORT" || upper == "LIMIT" {
			break
		}
		fields = append(fields, tokens[pos])
		pos++
	}

	return fields, pos
}

// parseConditions parses WHERE conditions connected by AND.
func parseConditions(tokens []string, pos int) ([]DVCondition, int) {
	var conditions []DVCondition

	for pos < len(tokens) {
		upper := strings.ToUpper(tokens[pos])
		if upper == "SORT" || upper == "LIMIT" {
			break
		}

		// Skip AND connectors
		if upper == "AND" {
			pos++
			continue
		}

		// Check for negation prefix (e.g., !completed)
		if strings.HasPrefix(tokens[pos], "!") && len(tokens[pos]) > 1 {
			conditions = append(conditions, DVCondition{
				Field:  tokens[pos][1:],
				Op:     "=",
				Value:  "true",
				Negate: true,
			})
			pos++
			continue
		}

		// Need at least field, op, value
		if pos+2 >= len(tokens) {
			// Check if next token is a keyword; if so, treat as boolean field
			if pos+1 < len(tokens) {
				nextUpper := strings.ToUpper(tokens[pos+1])
				if nextUpper == "SORT" || nextUpper == "LIMIT" || nextUpper == "AND" {
					conditions = append(conditions, DVCondition{
						Field: tokens[pos],
						Op:    "=",
						Value: "true",
					})
					pos++
					continue
				}
			}
			// Not enough tokens for a full condition
			if pos < len(tokens) {
				conditions = append(conditions, DVCondition{
					Field: tokens[pos],
					Op:    "=",
					Value: "true",
				})
				pos++
			}
			break
		}

		field := tokens[pos]
		pos++

		op := strings.ToUpper(tokens[pos])
		pos++

		// Normalize operator
		switch op {
		case "=", "!=", ">", "<", ">=", "<=":
			// already fine
		case "CONTAINS":
			op = "CONTAINS"
		default:
			// Unknown operator, skip
			continue
		}

		value := tokens[pos]
		pos++

		conditions = append(conditions, DVCondition{
			Field: field,
			Op:    op,
			Value: value,
		})
	}

	return conditions, pos
}

// unquote removes surrounding quotes from a string if present.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
