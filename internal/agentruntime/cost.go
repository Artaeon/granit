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

// Refreshed against developers.openai.com/api/docs/pricing in May 2026.
// Numbers are micro-cents per token: $X per 1M tokens × 100 → micro-cents
// per token (so $2.50/1M = 250 µ¢/token). Refresh whenever OpenAI ships
// a new tier or shutters one.
var modelPrices = map[string]modelPrice{
	// GPT-5.5 family — current flagship.
	"gpt-5.5":             {inputPerToken: 500, outputPerToken: 3000},
	"gpt-5.5-pro":         {inputPerToken: 3000, outputPerToken: 18000},
	// GPT-5.4 family — current workhorse, recommended defaults.
	"gpt-5.4":             {inputPerToken: 250, outputPerToken: 1500},
	"gpt-5.4-mini":        {inputPerToken: 75, outputPerToken: 450},
	"gpt-5.4-nano":        {inputPerToken: 20, outputPerToken: 125},
	"gpt-5.4-pro":         {inputPerToken: 3000, outputPerToken: 18000},
	// GPT-5.3 chat / codex.
	"gpt-5.3-chat-latest": {inputPerToken: 175, outputPerToken: 1400},
	"gpt-5.3-codex":       {inputPerToken: 175, outputPerToken: 1400},
	// GPT-5 base family — still served.
	"gpt-5":               {inputPerToken: 125, outputPerToken: 1000},
	"gpt-5-mini":          {inputPerToken: 25, outputPerToken: 200},
	"gpt-5-nano":          {inputPerToken: 5, outputPerToken: 40},
	// GPT-4.1 family — 1M-context legacy track. Migration target for
	// the gpt-4* snapshots that shut down 2026-10-23.
	"gpt-4.1":             {inputPerToken: 200, outputPerToken: 800},
	"gpt-4.1-mini":        {inputPerToken: 40, outputPerToken: 160},
	"gpt-4.1-nano":        {inputPerToken: 10, outputPerToken: 40},
	// GPT-4o family — deprecation-bound (shutdown 2026-10-23) but
	// still served, kept for transitional cost reporting.
	"gpt-4o":              {inputPerToken: 250, outputPerToken: 1000},
	"gpt-4o-mini":         {inputPerToken: 15, outputPerToken: 60},
	// o-series reasoning models.
	"o3":                  {inputPerToken: 200, outputPerToken: 800},
	"o4-mini":             {inputPerToken: 110, outputPerToken: 440},
	// Legacy snapshots — deprecated, scheduled shutdown 2026-10-23.
	"gpt-4-turbo":         {inputPerToken: 1000, outputPerToken: 3000},
	"gpt-4":               {inputPerToken: 3000, outputPerToken: 6000},
	"gpt-3.5-turbo":       {inputPerToken: 50, outputPerToken: 150},
}

// RecommendedModel describes a model surfaced in the settings picker.
// Pricing is per 1M tokens (USD), shown alongside each option so the
// user picks knowingly. Order matters — first item is the default.
type RecommendedModel struct {
	ID          string `json:"id"`
	Family      string `json:"family"`
	InputPerM   string `json:"input_per_m"`  // pre-formatted "$0.20"
	OutputPerM  string `json:"output_per_m"` // pre-formatted "$1.25"
	Note        string `json:"note,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// RecommendedOpenAIModels returns a curated, ordered list of OpenAI
// chat models a user is likely to want — newest-first, value-tier
// labelled. Used by the settings page to render a dropdown instead of
// a free-form text input. Stays in sync with modelPrices above.
func RecommendedOpenAIModels() []RecommendedModel {
	return []RecommendedModel{
		{ID: "gpt-5.4-mini", Family: "GPT-5.4", InputPerM: "$0.75", OutputPerM: "$4.50", Note: "best default", Recommended: true},
		{ID: "gpt-5.4-nano", Family: "GPT-5.4", InputPerM: "$0.20", OutputPerM: "$1.25", Note: "cheapest 5.4 tier", Recommended: true},
		{ID: "gpt-5.4", Family: "GPT-5.4", InputPerM: "$2.50", OutputPerM: "$15.00", Note: "flagship 5.4"},
		{ID: "gpt-5-nano", Family: "GPT-5", InputPerM: "$0.05", OutputPerM: "$0.40", Note: "ultra-cheap"},
		{ID: "gpt-5-mini", Family: "GPT-5", InputPerM: "$0.25", OutputPerM: "$2.00"},
		{ID: "gpt-5", Family: "GPT-5", InputPerM: "$1.25", OutputPerM: "$10.00"},
		{ID: "gpt-5.5", Family: "GPT-5.5", InputPerM: "$5.00", OutputPerM: "$30.00", Note: "newest tier"},
		{ID: "gpt-4.1-mini", Family: "GPT-4.1", InputPerM: "$0.40", OutputPerM: "$1.60", Note: "1M context"},
		{ID: "gpt-4.1-nano", Family: "GPT-4.1", InputPerM: "$0.10", OutputPerM: "$0.40", Note: "1M context, extraction tier"},
		{ID: "gpt-4.1", Family: "GPT-4.1", InputPerM: "$2.00", OutputPerM: "$8.00", Note: "1M context flagship"},
		{ID: "o3", Family: "o-series", InputPerM: "$2.00", OutputPerM: "$8.00", Note: "reasoning"},
		{ID: "gpt-4o-mini", Family: "GPT-4o", InputPerM: "$0.15", OutputPerM: "$0.60", Note: "deprecation 2026-10-23"},
		{ID: "gpt-4o", Family: "GPT-4o", InputPerM: "$2.50", OutputPerM: "$10.00", Note: "deprecation 2026-10-23"},
	}
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
	if _, ok := modelPrices[m]; ok {
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
