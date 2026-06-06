// Command palette public shapes — kept dependency-free so every other
// extracted file (catalog, data cache, items builder, keyboard install)
// can import them without dragging Svelte runes / API types along.
//
// Group: every section header the palette renders. The order in the
// derived items list is decided by groupRank (see paletteItems), NOT
// by union-member position — keep the union alphabetical-ish for
// readability rather than ordering it by precedence.
//
// CmdItem: the unified row shape. Every section produces these and
// they merge into one list before sort. `id` doubles as the recents
// key — see paletteRecents.RECENT_KEY for the format conventions.
//
// AgentCmd: pre-merge shape for the static AGENTS catalog. Carries
// `slug` instead of a built `id` so the catalog can stay independent
// of how the palette namespaces its IDs ("agent:" + slug happens at
// build time inside the items derivation).

export type Group =
  | 'Pages'
  | 'Workspace'
  | 'Projects'
  | 'Goals'
  | 'Notes'
  | 'Tasks'
  | 'Events'
  | 'Deadlines'
  | 'Habits'
  | 'Agents'
  | 'Content';

export interface CmdItem {
  /** Stable ID — also the localStorage recent key. Pages: 'page:/path',
   *  Projects: 'project:<name>', Goals: 'goal:<id>', Notes: 'note:<path>',
   *  Agents: 'agent:<slug>'. Content (search hits) are excluded from
   *  recents because they're query-driven, not destinations. */
  id: string;
  label: string;
  detail?: string;
  icon: string;
  group: Group;
  /** Keyboard hint rendered on the right (e.g. 'Mod+P', 'a' on /tasks).
   *  Optional — most items don't carry a hotkey. */
  hint?: string;
  /** Pure side-effect. Closes the palette before running so the caller
   *  doesn't have to. */
  run: () => void | Promise<void>;
}

export interface AgentCmd {
  slug: string;
  label: string;
  detail: string;
  icon: string;
  hint?: string;
  run: () => void | Promise<void>;
}
