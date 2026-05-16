// focusOnMount — Svelte action replacement for the `autofocus`
// attribute. Svelte 5 raises an a11y warning on every <input autofocus>
// because the attribute hijacks tab order and is the wrong primitive
// for the "focus this when it appears" case (autofocus only fires
// once per page load — re-rendering an element with autofocus does
// NOT re-focus it).
//
// Use this for inputs that should grab focus the moment they mount:
// inline rename forms, modal first-fields, quick-add inputs that
// open conditionally. The action runs `.focus()` after the element
// is in the DOM, which honours assistive-tech focus management and
// re-fires every time the action attaches (each time the {#if} that
// wraps the input flips true).
//
// Usage:
//   <input use:focusOnMount ... />
//
// For the rare "only focus if X" case, pass a parameter that the
// action reads on attach:
//   <input use:focusOnMount={shouldFocus} ... />

export function focusOnMount(node: HTMLElement, enabled: boolean = true): { destroy: () => void } {
  if (enabled) {
    // queueMicrotask defers past Svelte's own DOM-insertion frame,
    // so any sibling element claiming focus first (e.g. a click
    // handler that opened the form) doesn't steal it back.
    queueMicrotask(() => node.focus());
  }
  return { destroy: () => {} };
}
