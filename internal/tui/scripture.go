package tui

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scripture represents a single verse/quote entry.
type Scripture struct {
	Text   string // the verse or quote text
	Source string // e.g. "Proverbs 3:5-6" or "Marcus Aurelius"
}

// LoadScriptures reads scriptures from .granit/scriptures.md.
// Format: each entry is a line or paragraph. Lines starting with ">" are
// the verse text; the next non-empty line is the source/reference.
// Simple format: "verse text — Source Reference" (one per line).
func LoadScriptures(vaultRoot string) []Scripture {
	path := filepath.Join(vaultRoot, ".granit", "scriptures.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultScriptures()
	}

	var scriptures []Scripture
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Try to split on " — " or " - " for source
		s := Scripture{Text: line}
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
		return defaultScriptures()
	}
	return scriptures
}

// DailyScripture returns a scripture for today (deterministic by date).
func DailyScripture(vaultRoot string) Scripture {
	scriptures := LoadScriptures(vaultRoot)
	if len(scriptures) == 0 {
		return defaultScriptures()[0]
	}
	// Use day-of-year as seed for deterministic daily rotation
	now := time.Now()
	dayIndex := now.YearDay() + now.Year()*367
	return scriptures[dayIndex%len(scriptures)]
}

// RandomScripture returns a random scripture.
func RandomScripture(vaultRoot string) Scripture {
	scriptures := LoadScriptures(vaultRoot)
	return scriptures[rand.Intn(len(scriptures))]
}

func defaultScriptures() []Scripture {
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
		{Text: "Have I not commanded you? Be strong and courageous. Do not be terrified; do not be discouraged.", Source: "Joshua 1:9"},
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
