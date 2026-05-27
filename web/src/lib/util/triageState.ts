// Shared triage-state helpers. Previously inline in TaskCard.svelte;
// any other surface that wanted to render or cycle a task's triage
// would have to clone these. Centralised here so the cycle order +
// tone mapping stay authoritative.
//
// The cycle order matches granit's UX:
//   inbox → triaged → scheduled → done → dropped → snoozed → inbox

import type { Task } from '$lib/api';

export const triageOrder: Array<NonNullable<Task['triage']>> = [
  'inbox',
  'triaged',
  'scheduled',
  'done',
  'dropped',
  'snoozed'
];

export function nextTriage(cur?: string): NonNullable<Task['triage']> {
  const i = triageOrder.indexOf((cur as NonNullable<Task['triage']>) || 'inbox');
  return triageOrder[(i + 1) % triageOrder.length];
}

// Palette-tone name (NOT a class). Call sites compose their own class
// string (chip / pill / outline) — Tailwind purger can't see dynamic
// composition, so we return the tone and let the caller pick the
// visual treatment.
export function triageTone(t?: string): string {
  if (t === 'inbox') return 'subtext';
  if (t === 'triaged') return 'info';
  if (t === 'scheduled') return 'primary';
  if (t === 'done') return 'success';
  if (t === 'dropped') return 'dim';
  if (t === 'snoozed') return 'warning';
  return 'subtext';
}
