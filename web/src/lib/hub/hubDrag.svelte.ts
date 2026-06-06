// Native HTML5 drag-to-reorder controller for the /hub launcher.
//
// Fourth extraction step out of routes/hub/+page.svelte. Owns the
// two pieces of transient drag state (dragId for the source card,
// dragOverId for the hover target) and the five drag-event handlers
// the template wires to each card's draggable attributes.
//
// Why native drag-and-drop: no new dependency, pointer-perfect on
// every browser, plays nicely with the existing card hover state.
// Setting data-transfer is required on Firefox or the drag never
// starts, so we put the item ID there even though dragId is the
// state that actually drives the reorder. Cards are draggable
// WITHIN a category section only — cross-category moves require
// editing the card (category is a free-text field, not an enum,
// so a drag metaphor doesn't map cleanly).
//
// reorder() is fired by onDrop() with the same categoryItems slice
// the page renders so the controller never has to know about
// grouping. The caller injects reload() so this file stays
// fetch-plumbing free (matches the form controller shape).

import { api, type HubItem } from '$lib/api';
import { toast } from '$lib/components/toast';

export interface HubDragController {
  /** ID of the card being dragged, or null when idle. Template
   *  reads this to dim the source. */
  readonly dragId: string | null;
  /** ID of the card the cursor is over while dragging, or null.
   *  Template reads this to outline the drop target. */
  readonly dragOverId: string | null;

  onDragStart(id: string, ev: DragEvent): void;
  onDragOver(id: string, ev: DragEvent): void;
  onDragLeave(id: string): void;
  /** Apply the reorder within the dropped-on card's category — the
   *  template scopes the drag handlers to category sections so
   *  categoryItems is already the right slice. */
  onDrop(targetId: string, categoryItems: HubItem[], ev: DragEvent): Promise<void>;
  onDragEnd(): void;
}

export interface HubDragDeps {
  /** Caller-owned reload — fires after a successful reorder so the
   *  items array reflects the new ordering. Wired by the page to the
   *  data controller's load(). */
  reload: () => Promise<void> | void;
}

export function createHubDrag(deps: HubDragDeps): HubDragController {
  let dragId = $state<string | null>(null);
  let dragOverId = $state<string | null>(null);

  function onDragStart(id: string, ev: DragEvent) {
    dragId = id;
    if (ev.dataTransfer) {
      ev.dataTransfer.effectAllowed = 'move';
      // Firefox requires SOMETHING on dataTransfer or the drag
      // session is killed before it starts. Wrap in try/catch
      // because some embedded browsers throw on setData().
      try { ev.dataTransfer.setData('text/plain', id); } catch {}
    }
  }

  function onDragOver(id: string, ev: DragEvent) {
    if (!dragId || dragId === id) return;
    ev.preventDefault();
    if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move';
    dragOverId = id;
  }

  function onDragLeave(id: string) {
    if (dragOverId === id) dragOverId = null;
  }

  async function onDrop(targetId: string, categoryItems: HubItem[], ev: DragEvent) {
    ev.preventDefault();
    const from = dragId;
    dragId = null;
    dragOverId = null;
    if (!from || from === targetId) return;
    // Reorder WITHIN the dropped-on card's category. The drag
    // handlers are scoped to category sections in the template,
    // so categoryItems is the right slice already.
    const ids = categoryItems.map((x) => x.id);
    const fromIdx = ids.indexOf(from);
    const toIdx = ids.indexOf(targetId);
    if (fromIdx < 0 || toIdx < 0) return;
    const [moved] = ids.splice(fromIdx, 1);
    ids.splice(toIdx, 0, moved);
    try {
      await api.reorderHubItems(ids);
      await deps.reload();
    } catch (e) {
      toast.error('reorder failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function onDragEnd() {
    dragId = null;
    dragOverId = null;
  }

  return {
    get dragId() { return dragId; },
    get dragOverId() { return dragOverId; },
    onDragStart,
    onDragOver,
    onDragLeave,
    onDrop,
    onDragEnd
  };
}
