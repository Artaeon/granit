// Strong's Lexicon — Greek (G####) + Hebrew (H####) word-study data.
//
// This is the lexicon half of the word-study feature: given a Strong's
// number (e.g. "G1722" for ἐν / "in"), look up lemma + transliteration
// + definitions. The companion tagged.go bundles a Strong's-tagged
// bible so the reader can map word → Strong's code in the first place.
//
// The lexicon JSON is fetched by scripts/fetch-strongs.sh from
// openscriptures/strongs (public-domain) and dropped at strongs.json
// next to web.json. The file is NOT checked in — it's ~50MB — so we
// ship a one-byte placeholder ("{}") in the repo to satisfy go:embed
// (Go's embed directive requires the file to exist at compile time
// and there's no native "optional file" syntax). The loader treats an
// empty/placeholder JSON as "lexicon not bundled" and returns nil, so
// callers can gracefully degrade.
package bible

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed strongs.json
var strongsJSON []byte

// StrongsEntry mirrors the openscriptures/strongs record shape. Every
// field is optional in practice because the upstream data quality
// varies between Greek (cleaner) and Hebrew (richer derivation notes).
type StrongsEntry struct {
	Lemma      string `json:"lemma"`       // original-language form, e.g. "ἀγάπη"
	Translit   string `json:"translit"`    // transliteration, e.g. "agápē"
	StrongsDef string `json:"strongs_def"` // Strong's own definition
	KJVDef     string `json:"kjv_def"`     // gloss of how the KJV renders the word
	Derivation string `json:"derivation"`  // etymology / root note
}

var (
	strongsOnce   sync.Once
	strongsMap    map[string]StrongsEntry
	strongsLoaded bool // true iff a real lexicon was bundled (not the {} stub)
	strongsErr    error
)

// LoadStrongs returns the full lexicon as a map keyed by Strong's code
// ("G1722", "H7225", …). Returns (nil, nil) if the lexicon JSON isn't
// bundled (placeholder shipped in source tree) — that's not an error,
// it's a graceful-degradation signal. Idempotent + concurrency-safe.
func LoadStrongs() (map[string]StrongsEntry, error) {
	strongsOnce.Do(func() {
		// Treat empty/placeholder JSON ("", "{}", whitespace) as
		// "not bundled" rather than an error. This is what lets the
		// build succeed before the user runs scripts/fetch-strongs.sh.
		trimmed := strings.TrimSpace(string(strongsJSON))
		if trimmed == "" || trimmed == "{}" {
			return
		}
		var m map[string]StrongsEntry
		if err := json.Unmarshal(strongsJSON, &m); err != nil {
			strongsErr = err
			return
		}
		if len(m) == 0 {
			return
		}
		// Normalise keys to upper-case so lookups are case-insensitive.
		// We don't expect mixed case in practice but the upstream JSON
		// has been known to ship lower-case "g1722" in patched forks.
		norm := make(map[string]StrongsEntry, len(m))
		for k, v := range m {
			norm[strings.ToUpper(strings.TrimSpace(k))] = v
		}
		strongsMap = norm
		strongsLoaded = true
	})
	return strongsMap, strongsErr
}

// StrongsBundled reports whether a real lexicon (not the placeholder)
// was compiled into the binary. Useful for the /status endpoint.
func StrongsBundled() bool {
	_, _ = LoadStrongs()
	return strongsLoaded
}

// LookupStrong returns one lexicon entry by code. The code is
// case-normalised so "g1722", "G1722", and " G1722 " all hit. The
// boolean is false when the lexicon isn't bundled OR the code is
// genuinely missing — callers should check StrongsBundled() first if
// they need to distinguish those cases.
func LookupStrong(code string) (StrongsEntry, bool) {
	m, err := LoadStrongs()
	if err != nil || m == nil {
		return StrongsEntry{}, false
	}
	key := strings.ToUpper(strings.TrimSpace(code))
	if key == "" {
		return StrongsEntry{}, false
	}
	e, ok := m[key]
	return e, ok
}
