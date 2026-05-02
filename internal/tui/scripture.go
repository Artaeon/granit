package tui

import "github.com/artaeon/granit/internal/scripture"

// Scripture is re-exported so existing TUI overlays compile unchanged.
// New code should import internal/scripture directly.
type Scripture = scripture.Scripture

// LoadScriptures, DailyScripture, RandomScripture preserve the legacy
// TUI-local function names. They thinly wrap the shared package.
func LoadScriptures(vaultRoot string) []Scripture { return scripture.Load(vaultRoot) }
func DailyScripture(vaultRoot string) Scripture   { return scripture.Daily(vaultRoot) }
func RandomScripture(vaultRoot string) Scripture  { return scripture.Random(vaultRoot) }

// defaultScriptures stays exported-package-local for any tests that
// expect the legacy private function name.
func defaultScriptures() []Scripture { return scripture.Defaults() }
