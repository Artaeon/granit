// Shared priority → palette-tone mapping. Five files had near-
// identical priorityClass/priorityTone helpers, each returning
// slightly different class strings (text vs bg vs border vs full
// composite). The mapping itself never differed — only the
// surrounding class composition did. Centralising the mapping
// here means a future palette change (e.g. swap the P3 tone from
// info to secondary) ripples to every task row + chip + bar in one
// place instead of five.
//
// Call sites pick the tone name and compose their own class
// strings — Tailwind's purger needs static class literals, so we
// can't return a `bg-${tone}/20` template. The mapping is the
// shared bit; the visual treatment stays per-component.

export type PriorityTone = 'error' | 'warning' | 'info' | 'dim';

export function priorityTone(p: number | undefined | null): PriorityTone {
  if (p === 1) return 'error';
  if (p === 2) return 'warning';
  if (p === 3) return 'info';
  return 'dim';
}

// Helper for the most common case: text-color class. Tailwind needs
// static literals, so the switch is unavoidable, but having it once
// here keeps the mapping authoritative.
export function priorityTextClass(p: number | undefined | null): string {
  switch (priorityTone(p)) {
    case 'error': return 'text-error';
    case 'warning': return 'text-warning';
    case 'info': return 'text-info';
    default: return 'text-dim';
  }
}

// Helper for the badge pattern: filled-bg + matching text + border.
// Returns '' for the dim case so the chip doesn't render decoration
// for unprioritised tasks.
export function priorityBadgeClass(p: number | undefined | null): string {
  switch (priorityTone(p)) {
    case 'error': return 'bg-error/20 text-error border-error/30';
    case 'warning': return 'bg-warning/20 text-warning border-warning/30';
    case 'info': return 'bg-info/20 text-info border-info/30';
    default: return '';
  }
}

// Helper for the border-accent pattern (left rail on a card).
export function priorityBorderClass(p: number | undefined | null): string {
  switch (priorityTone(p)) {
    case 'error': return 'border-error';
    case 'warning': return 'border-warning';
    case 'info': return 'border-info';
    default: return 'border-surface1';
  }
}
