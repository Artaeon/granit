package tasks

import (
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
)

// Fingerprint is a stable hash of a task's normalized text. The
// reconciliation algorithm uses it to glue a sidecar entry to the
// markdown line that originated it across edits that don't touch
// the wording — toggling the checkbox, adjusting due date,
// changing priority, etc.
//
// Returned as a 16-char lowercase hex string of the FNV-64 hash.
// Collisions are possible at large N; the reconciler handles them
// via line-proximity disambiguation.
func Fingerprint(rawTaskLine string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(NormalizeTaskText(rawTaskLine)))
	return strconv.FormatUint(h.Sum64(), 16)
}

// reCheckboxPrefix matches "  - [ ] " — the indent + dash bullet
// + checkbox at the start of a task line. Intentionally limited
// to the dash bullet to match the parser regex (parser.go); a
// looser fingerprint that accepted * or + bullets would normalize
// task-shaped lines that the parser silently skips, and the user
// would see those tasks vanish from every overlay.
var reCheckboxPrefix = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*`)

// reTrailingMeta strips markdown-task metadata that the user (or
// granit) writes back into the line as the task's lifecycle
// progresses. These chunks change frequently and should not affect
// the fingerprint:
//
//   - 📅 due dates, 🛫 start dates, ⏳ scheduled, ✅ done dates
//   - ⏰ HH:MM-HH:MM time ranges
//   - 🔺 ⏫ 🔼 🔽 ⏬ priority emoji
//   - p:N priority shorthand
//   - ~Nm estimate shorthand
//   - 🔁 recurrence rules
//   - [note:...] inline notes
//   - goal:Gxxx links
//   - snooze:... markers
var reTrailingMeta = regexp.MustCompile(
	`📅\s*\d{4}-\d{2}-\d{2}` +
		`|🛫\s*\d{4}-\d{2}-\d{2}` +
		`|⏳\s*\d{4}-\d{2}-\d{2}` +
		`|✅\s*\d{4}-\d{2}-\d{2}` +
		`|⏰\s*\d{1,2}:\d{2}(-\d{1,2}:\d{2})?` +
		`|🔺|⏫|🔼|🔽|⏬` +
		`|\bp:[0-9]\b` +
		`|~\d+m\b` +
		`|🔁\s*\S+` +
		`|\[note:[^\]]*\]` +
		`|\bgoal:G\d+\b` +
		`|\bsnooze:\S+`,
)

// NormalizeTaskText strips checkbox prefix, mutable trailing
// metadata, and surrounding whitespace, then lowercases. The result
// is the load-bearing identifier for fingerprinting.
//
// Tags (#shopping) are preserved — moving a task to a different
// tag is a meaningful enough edit that we'd rather assign a new ID
// (or hit the fuzzy fallback) than silently confuse the user.
func NormalizeTaskText(s string) string {
	s = reCheckboxPrefix.ReplaceAllString(s, "")
	s = reTrailingMeta.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ") // collapse internal whitespace
	return strings.ToLower(strings.TrimSpace(s))
}
