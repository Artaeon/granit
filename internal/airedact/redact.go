// Package airedact strips personally-identifiable information
// from prompts before they leave the device for a cloud LLM. The
// AI features in granit pass user content through Redact() — the
// resulting string still reads naturally but with the PII bits
// substituted for category placeholders.
//
// The point isn't bullet-proof anonymisation (an LLM could
// re-identify by context); the point is to never accidentally
// upload literal email addresses, phone numbers, IBANs, or
// credit-card-shaped strings to a third-party API. Defence in
// depth alongside the Local-Ollama-by-default posture.
//
// Redaction is a configurable list of regex → replacement pairs
// stored in the prefs sidecar. The defaults below cover the
// common high-risk shapes; users can disable or extend per their
// risk tolerance.
package airedact

import (
	"regexp"
)

// Rule is one regex with its replacement label. Compiled lazily
// by the package on first use.
type Rule struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement string
}

// DefaultRules returns the pre-baked redaction set. The order
// matters: more-specific patterns first so generic fallbacks don't
// eat structured matches.
func DefaultRules() []Rule {
	return []Rule{
		{
			// IBAN — letters + digits, country prefix. Done before
			// generic numbers so a real IBAN gets [IBAN] not a
			// run of [PHONE] + [DIGITS].
			Name:        "iban",
			Pattern:     regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{10,30}\b`),
			Replacement: "[IBAN]",
		},
		{
			// Credit-card shape: 13-19 digits with optional dashes
			// or spaces. Liberal — false positives (long phone
			// numbers, order numbers) are acceptable here, the
			// alternative is silent leak.
			Name:        "credit_card",
			Pattern:     regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`),
			Replacement: "[CARD]",
		},
		{
			Name:        "email",
			Pattern:     regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
			Replacement: "[EMAIL]",
		},
		{
			// E.164-ish phone: + followed by 7-15 digits, with
			// optional spaces / dashes. The leading + cuts
			// false-positive rate significantly.
			Name:        "phone_intl",
			Pattern:     regexp.MustCompile(`\+\d{1,3}[ -]?(?:\d[ -]?){6,14}\b`),
			Replacement: "[PHONE]",
		},
		{
			// IPv4 — privacy-relevant on home networks even though
			// the address itself is rarely sensitive.
			Name:        "ipv4",
			Pattern:     regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
			Replacement: "[IP]",
		},
	}
}

// Redact applies all rules in order. Empty rules → returns input
// unchanged so the caller can disable globally without special-
// casing.
func Redact(text string, rules []Rule) string {
	for _, r := range rules {
		if r.Pattern == nil {
			continue
		}
		text = r.Pattern.ReplaceAllString(text, r.Replacement)
	}
	return text
}

// Diff returns the (count, byteDelta) pair telling the caller how
// many redactions happened. Useful for the audit log so users see
// "12 emails + 3 phone numbers redacted" without storing the
// originals.
type Stat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func RedactWithStats(text string, rules []Rule) (string, []Stat) {
	stats := make([]Stat, 0, len(rules))
	for _, r := range rules {
		if r.Pattern == nil {
			continue
		}
		matches := r.Pattern.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}
		stats = append(stats, Stat{Name: r.Name, Count: len(matches)})
		text = r.Pattern.ReplaceAllString(text, r.Replacement)
	}
	return text, stats
}
