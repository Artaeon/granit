// Shared color + status helpers for the planning surfaces
// (ventures, projects, goals). Pulled together so the ventures
// and projects pages stop maintaining their own copies of the
// palette map — a colour rename (e.g. mauve → primary) used to
// require touching every consumer; now it's one edit.
//
// colorVar accepts the Catppuccin-flavoured user palette label
// ('red', 'mauve', 'peach', ...) and returns the matching CSS
// variable. Unknown labels fall back to --color-secondary so a
// venture with no color set still renders a visible swatch.
//
// statusTone + statusIcon are the venture/project/goal lifecycle
// helpers. 'completed' is project-only; ventures don't have that
// state. Returning 'subtext' for unknown values keeps every card
// renderable even if the backend introduces a new status the
// client hasn't shipped support for yet.

const PALETTE_MAP: Record<string, string> = {
  red: 'error',
  yellow: 'warning',
  orange: 'accent',
  green: 'success',
  blue: 'secondary',
  purple: 'primary',
  cyan: 'info',
  mauve: 'primary',
  peach: 'accent',
  teal: 'info',
  sapphire: 'secondary',
  pink: 'accent',
  lavender: 'primary',
  flamingo: 'error'
};

export function colorVar(c?: string): string {
  return `var(--color-${PALETTE_MAP[c ?? ''] ?? 'secondary'})`;
}

export function statusTone(status?: string): string {
  if (status === 'active') return 'success';
  if (status === 'paused') return 'warning';
  if (status === 'completed') return 'info';
  if (status === 'archived') return 'subtext';
  return 'subtext';
}

export function statusIcon(status?: string): string {
  if (status === 'active') return '●';
  if (status === 'paused') return '◐';
  if (status === 'archived') return '◯';
  return '●';
}
