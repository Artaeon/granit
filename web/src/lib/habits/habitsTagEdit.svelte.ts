// Inline tag-edit controller for the habits surface.
//
// Sibling of habitsCategoryEdit: same factory shape, but tags are
// multi-select and the UI exposes both per-tag chips (with × to
// remove) and a "+ tag" adder with autocomplete from
// api.listHabitTags. The controller owns the open habit + the
// current draft set + the suggestion input; commit hits the patch
// endpoint with the full replacement list.
//
// Tags are stored as an ordered array on the server. We dedupe +
// trim on commit so a user typing "Focus, focus, FOCUS" lands one
// canonical "Focus" tag. Case is preserved verbatim — the server
// already normalises in its sidecar layer, no need to lowercase
// here.

import { api } from '$lib/api';

export interface TagEditController {
  /** Name of the habit whose editor is open. null when nothing open. */
  openFor: string | null;
  /** Working set of tags (preserves order). Mutated as the user
   *  adds / removes chips inside the popover. Committed wholesale. */
  draft: string[];
  /** Free-text input for the "+ tag" adder. */
  addText: string;
  /** True while a patch is in flight — disables double-submit. */
  readonly busy: boolean;
  /** Memoised known tags. Populated on first loadTags(). */
  readonly tags: string[];

  /** Open the editor for `habitName`, pre-filling the draft with
   *  the habit's current tags (so the popover shows current
   *  selection without surprise). Triggers a tag-list refresh in
   *  the background. */
  open(habitName: string, current: string[] | undefined): void;
  /** Close without committing. */
  cancel(): void;
  /** Lazy-load + memoise the known-tag list. Pass force=true after
   *  a successful patch to pick up newly-introduced tags. */
  loadTags(force?: boolean): Promise<void>;
  /** Push `tag` onto the draft (deduped + trimmed). No-op when
   *  empty or already present. */
  addTag(tag: string): void;
  /** Drop `tag` from the draft. No-op when absent. */
  removeTag(tag: string): void;
  /** Commit the draft for `habitName`. Empty list clears the tags
   *  (server: sending [] removes the sidecar entry). Calls onPatch
   *  on success so the caller can reload. */
  commit(habitName: string): Promise<void>;
}

export type TagEditDeps = {
  /** Called after a successful patch. */
  onPatch: (habitName: string, tags: string[]) => void | Promise<void>;
};

export function createTagEditCtl(deps: TagEditDeps): TagEditController {
  let openFor = $state<string | null>(null);
  let draft = $state<string[]>([]);
  let addText = $state('');
  let busy = $state(false);
  let tags = $state<string[]>([]);
  let loaded = false;
  let loading: Promise<void> | null = null;

  function open(habitName: string, current: string[] | undefined) {
    openFor = habitName;
    // Defensive copy so editing the popover doesn't mutate the
    // underlying habit object in the route's reactive state.
    draft = current ? [...current] : [];
    addText = '';
    void loadTags();
  }

  function cancel() {
    openFor = null;
    draft = [];
    addText = '';
  }

  async function loadTags(force = false): Promise<void> {
    if (loaded && !force) return;
    if (loading && !force) return loading;
    loading = (async () => {
      try {
        const r = await api.listHabitTags();
        tags = (r.tags ?? []).slice().sort((a, b) => a.localeCompare(b));
        loaded = true;
      } catch {
        tags = [];
      } finally {
        loading = null;
      }
    })();
    return loading;
  }

  function addTag(tag: string) {
    const t = tag.trim();
    if (!t) return;
    if (draft.includes(t)) return;
    draft = [...draft, t];
    addText = '';
  }

  function removeTag(tag: string) {
    draft = draft.filter((t) => t !== tag);
  }

  async function commit(habitName: string): Promise<void> {
    if (busy) return;
    busy = true;
    try {
      // Final dedupe + trim — the popover already guards against
      // dupes, but a user can paste a list straight into the field.
      const cleaned: string[] = [];
      for (const raw of draft) {
        const t = raw.trim();
        if (t && !cleaned.includes(t)) cleaned.push(t);
      }
      await deps.onPatch(habitName, cleaned);
      openFor = null;
      draft = [];
      addText = '';
      if (cleaned.length > 0) void loadTags(true);
    } finally {
      busy = false;
    }
  }

  return {
    get openFor() { return openFor; },
    set openFor(v) { openFor = v; },
    get draft() { return draft; },
    set draft(v) { draft = v; },
    get addText() { return addText; },
    set addText(v) { addText = v; },
    get busy() { return busy; },
    get tags() { return tags; },
    open,
    cancel,
    loadTags,
    addTag,
    removeTag,
    commit
  };
}
