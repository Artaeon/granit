// Package scripture is the shared verse/quote loader used by both the
// granit TUI's scripture overlay and the web's /scripture page (plus
// the dashboard's "verse of the day" widget).
//
// Source of truth: <vault>/.granit/scriptures.md — one entry per line,
// with optional " — Source" / " – Source" / " - Source" suffix to
// separate the quote text from its citation. The TUI established this
// format; we keep it byte-for-byte compatible so a vault edited in
// either surface stays portable.
package scripture

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scripture represents a single verse / quote entry.
type Scripture struct {
	Text   string `json:"text"`             // the verse or quote text
	Source string `json:"source,omitempty"` // e.g. "Proverbs 3:5-6" or "Marcus Aurelius"
}

// Load reads scriptures from <vault>/.granit/scriptures.md, returning
// the built-in defaults when the file is missing or empty. Lines starting
// with '#' are treated as comments/headers and skipped — useful so users
// can group their custom scriptures into sections in the markdown file.
func Load(vaultRoot string) []Scripture {
	path := filepath.Join(vaultRoot, ".granit", "scriptures.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults()
	}

	var scriptures []Scripture
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s := Scripture{Text: line}
		// LastIndex (not Index): a verse may itself contain a hyphen.
		// We only treat the FINAL separator as the text/source split.
		for _, sep := range []string{" — ", " – ", " - "} {
			if idx := strings.LastIndex(line, sep); idx > 0 {
				s.Text = strings.TrimSpace(line[:idx])
				s.Source = strings.TrimSpace(line[idx+len(sep):])
				break
			}
		}
		if s.Text != "" {
			scriptures = append(scriptures, s)
		}
	}
	if len(scriptures) == 0 {
		return Defaults()
	}
	return scriptures
}

// Daily returns a deterministic verse for today — same input across
// every device on the same day, so a phone and a laptop see the same
// verse. Rotation seed combines day-of-year and year so the cycle
// shifts across years even with the same vault.
func Daily(vaultRoot string) Scripture {
	all := Load(vaultRoot)
	if len(all) == 0 {
		return Defaults()[0]
	}
	now := time.Now()
	idx := now.YearDay() + now.Year()*367
	return all[idx%len(all)]
}

// Random returns one verse uniformly from the loaded set. Used by the
// TUI's "another one" button; the web's quiz mode uses the full set
// directly so it can pick without replacement.
func Random(vaultRoot string) Scripture {
	all := Load(vaultRoot)
	if len(all) == 0 {
		return Defaults()[0]
	}
	return all[rand.Intn(len(all))]
}

// Defaults is the seed scripture set shipped with granit. Same content
// the TUI used historically — moved here so a fresh vault has something
// to display on the very first launch without needing to populate
// scriptures.md.
func Defaults() []Scripture {
	return []Scripture{
		{Text: "Trust in the LORD with all your heart and lean not on your own understanding; in all your ways submit to him, and he will make your paths straight.", Source: "Proverbs 3:5-6"},
		{Text: "I can do all things through Christ who strengthens me.", Source: "Philippians 4:13"},
		{Text: "Be strong and courageous. Do not be afraid; do not be discouraged, for the LORD your God will be with you wherever you go.", Source: "Joshua 1:9"},
		{Text: "Commit to the LORD whatever you do, and he will establish your plans.", Source: "Proverbs 16:3"},
		{Text: "But those who hope in the LORD will renew their strength. They will soar on wings like eagles; they will run and not grow weary, they will walk and not be faint.", Source: "Isaiah 40:31"},
		{Text: "Whatever you do, work at it with all your heart, as working for the Lord, not for human masters.", Source: "Colossians 3:23"},
		{Text: "The fear of the LORD is the beginning of wisdom, and knowledge of the Holy One is understanding.", Source: "Proverbs 9:10"},
		{Text: "Do not conform to the pattern of this world, but be transformed by the renewing of your mind.", Source: "Romans 12:2"},
		{Text: "For God gave us a spirit not of fear but of power and love and self-discipline.", Source: "2 Timothy 1:7"},
		{Text: "No discipline seems pleasant at the time, but painful. Later on, however, it produces a harvest of righteousness and peace for those who have been trained by it.", Source: "Hebrews 12:11"},
		{Text: "The plans of the diligent lead to profit as surely as haste leads to poverty.", Source: "Proverbs 21:5"},
		{Text: "Whatever your hand finds to do, do it with all your might.", Source: "Ecclesiastes 9:10"},
		{Text: "The righteous are as bold as a lion.", Source: "Proverbs 28:1"},
		{Text: "He who is faithful in a very little thing is faithful also in much.", Source: "Luke 16:10"},
		{Text: "And let us not grow weary of doing good, for in due season we will reap, if we do not give up.", Source: "Galatians 6:9"},
		{Text: "Iron sharpens iron, and one man sharpens another.", Source: "Proverbs 27:17"},
		{Text: "The LORD is my shepherd, I lack nothing.", Source: "Psalm 23:1"},
		{Text: "Be very careful, then, how you live — not as unwise but as wise, making the most of every opportunity.", Source: "Ephesians 5:15-16"},
		{Text: "Blessed is the one who perseveres under trial because, having stood the test, that person will receive the crown of life.", Source: "James 1:12"},
		{Text: "Set your minds on things above, not on earthly things.", Source: "Colossians 3:2"},
		{Text: "The LORD will fight for you; you need only to be still.", Source: "Exodus 14:14"},
	}
}
