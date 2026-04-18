package tui

// Centralized AI persona helpers. Half a dozen overlays embed the
// "You are DEEPCOVEN, a direct and honest {role}" preamble inline, with
// drift between sites — some say "direct, honest, and action-oriented,"
// some say "direct and honest," some omit the persona entirely. This
// file is the single point of truth so:
//
//   - The persona's name, voice, and stance change in one place.
//   - New AI features get a one-liner instead of copy-pasting four
//     lines of preamble that risk drifting.
//   - Tests can introspect the persona without grep'ing eight files.
//
// Each call site picks an intro variant matching the feature's voice
// (concise vs. structured) and appends its task-specific instructions.

import "strings"

// DeepCovenIntro returns the short single-line system-prompt opener used
// by most coach-style overlays (daily review, weekly review, habits,
// goals, projects). Pass the role description, e.g. "end-of-day coach"
// or "habit coach"; the function wraps it into a complete sentence.
//
//	DeepCovenIntro("end-of-day coach")
//	→ "You are DEEPCOVEN, a direct and honest end-of-day coach."
func DeepCovenIntro(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "personal assistant"
	}
	return "You are DEEPCOVEN, a direct and honest " + role + "."
}

// DeepCovenSystem composes the standard short intro with task-specific
// guidance, separated by a blank line so the model sees them as
// distinct paragraphs. Equivalent to writing
//
//	DeepCovenIntro(role) + "\n\n" + task
//
// inline, with the convenience that the joining is consistent across
// callers.
func DeepCovenSystem(role, task string) string {
	intro := DeepCovenIntro(role)
	task = strings.TrimSpace(task)
	if task == "" {
		return intro
	}
	return intro + "\n\n" + task
}

// DeepCovenLongPreamble is the verbose 4-line manifesto used by the
// daily-briefing prompt. Other overlays generally don't need this much —
// they have their own structured-output requirements. Kept here so a
// future redesign of the persona's stance updates everywhere.
const DeepCovenLongPreamble = `You are DEEPCOVEN — a direct, honest, and action-oriented personal assistant embedded in the user's knowledge base.

CORE PRINCIPLES:
- 100% honesty: every insight must be true and transparent
- 100% service: every word must move the user forward
- Be direct, clear, no filler — focus on what matters NOW
- Encouraging but honest — never sugarcoat, never hold back`
