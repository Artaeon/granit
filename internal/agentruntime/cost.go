package agentruntime

import "strings"

// Pricing in micro-cents (millionths of a cent) per token. We use
// integer math throughout the cost path — float drift on a budget
// boundary is exactly the kind of off-by-one that bites in
// production. €0.25 → 25_000 micro-cents; one prompt token at
// gpt-4o-mini is 15 micro-cents (0.00015 cents). All comparisons
// are integer.
//
// Numbers below reflect public OpenAI list pricing as of late 2025
// rounded to micro-cents. They're a starting point; any model not
// in this table will skip budget enforcement (the runner falls back
// to the iteration cap).
//
// Source: openai.com/api/pricing — keep this comment updated when
// the table is refreshed.
type modelPrice struct {
	inputPerToken  int64 // micro-cents per prompt token
	outputPerToken int64 // micro-cents per completion token
}

var modelPrices = map[string]modelPrice{
	// GPT-4o family
	"gpt-4o-mini":      {inputPerToken: 15, outputPerToken: 60},
	"gpt-4o":           {inputPerToken: 250, outputPerToken: 1000},
	// GPT-5 family (placeholder for what the user mentioned; numbers
	// will need updating when official pricing lands).
	"gpt-5-nano":       {inputPerToken: 5, outputPerToken: 40},
	"gpt-5-mini":       {inputPerToken: 25, outputPerToken: 200},
	"gpt-5":            {inputPerToken: 125, outputPerToken: 1000},
	// Older snapshots
	"gpt-4-turbo":      {inputPerToken: 1000, outputPerToken: 3000},
	"gpt-4":            {inputPerToken: 3000, outputPerToken: 6000},
	"gpt-3.5-turbo":    {inputPerToken: 50, outputPerToken: 150},
}

// CostMicroCents returns the cost of a Usage in micro-cents (1/1_000_000
// of a cent). Returns -1 when we don't have pricing for the model so
// the caller can fall back to iteration-cap-only.
func CostMicroCents(u Usage) int64 {
	if u.Model == "" {
		return -1
	}
	p, ok := modelPrices[normalizeModel(u.Model)]
	if !ok {
		return -1
	}
	return int64(u.PromptTokens)*p.inputPerToken + int64(u.CompletionTokens)*p.outputPerToken
}

// normalizeModel collapses model snapshots to their family key. OpenAI
// API names like "gpt-4o-mini-2024-07-18" should match "gpt-4o-mini"
// in the price table.
func normalizeModel(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	if p, ok := modelPrices[m]; ok {
		_ = p
		return m
	}
	// Try progressively shorter prefixes. "gpt-4o-mini-2024-07-18"
	// → "gpt-4o-mini-2024-07" → … → "gpt-4o-mini" matches.
	for {
		idx := strings.LastIndex(m, "-")
		if idx <= 0 {
			break
		}
		m = m[:idx]
		if _, ok := modelPrices[m]; ok {
			return m
		}
	}
	return m
}

// FormatCents renders a micro-cent amount as a human-friendly euro/dollar
// string with up to 4 decimal places. We intentionally stay currency-
// neutral — the user's API account decides the actual currency. The web
// renders this as "€0.0042" or "$0.0042" depending on locale; the
// number is the same.
func FormatCents(microCents int64) string {
	if microCents < 0 {
		return "—"
	}
	// Render as cents.fraction. 25000 micro-cents = 0.025 cents = $0.00025
	// — but most calls cost much less than a cent so we render to four
	// decimal places of a cent, which lets sub-millisecond model
	// responses still show as something other than 0.
	cents := float64(microCents) / 1_000_000.0
	// Format with trailing-zero trimming so $0.0100 renders as $0.01.
	return formatFloat(cents)
}

func formatFloat(v float64) string {
	// 4-decimal default is enough resolution for $0.0001 rounding while
	// staying readable. We avoid Sprintf to dodge the package import in
	// hot paths; tests cover the formatting.
	if v < 0 {
		return "-" + formatFloat(-v)
	}
	cents4 := int64(v*10000 + 0.5) // round
	whole := cents4 / 10000
	frac := cents4 % 10000
	// Trim trailing zeros from the fraction.
	digits := []byte{
		byte('0' + frac/1000),
		byte('0' + (frac/100)%10),
		byte('0' + (frac/10)%10),
		byte('0' + frac%10),
	}
	end := 4
	for end > 1 && digits[end-1] == '0' {
		end--
	}
	out := make([]byte, 0, 16)
	out = appendInt(out, whole)
	out = append(out, '.')
	out = append(out, digits[:end]...)
	return string(out)
}

func appendInt(buf []byte, n int64) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(buf, tmp[i:]...)
}
