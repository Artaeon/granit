// Trigger the global QuickCaptureFab via its registered shortcut.
//
// We synthesize the Mod-Shift-N keystroke rather than exposing a new
// global store because the fab already owns its open-state through
// a window keydown listener. Reusing that listener keeps the fab as
// the single source of truth — no parallel "open" flag to drift out
// of sync with reality.
//
// Falls through gracefully when the fab isn't mounted (no listener
// is registered → no-op). Use the function from any surface that
// wants a "capture something" CTA.

export function triggerQuickCapture(): void {
  if (typeof window === 'undefined') return;
  const evt = new KeyboardEvent('keydown', {
    key: 'N',
    code: 'KeyN',
    metaKey: true,
    shiftKey: true,
    bubbles: true
  });
  window.dispatchEvent(evt);
}
