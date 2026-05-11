// isTypingTarget — true when the keyboard event is happening inside
// an input / textarea / select / contenteditable, so page-level
// hotkey handlers can early-return without stealing keystrokes
// from the user's typing.
//
// Used by every page that wires a window-level keydown listener
// for single-letter shortcuts (j/k navigation, 'a' to open the
// agent, '?' for help, etc.). Centralised so future cases — say
// shadow-DOM custom inputs — only need one fix.

export function isTypingTarget(el: EventTarget | null): boolean {
	if (!(el instanceof HTMLElement)) return false;
	const tag = el.tagName;
	if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
	if (el.isContentEditable) return true;
	return false;
}
