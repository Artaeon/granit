// Add / edit modal controller for the /hub launcher.
//
// Third extraction step out of routes/hub/+page.svelte. Owns the
// modal-open flag, the editing target (null = create), the eight
// form buffers bound to the modal inputs, the saving flag, and the
// open/save/remove handlers that talk to the API.
//
// Why a separate controller: the form fields + their open/save/
// remove handlers are a self-contained edit-buffer pattern — they
// have no overlap with the read-side view derivations or the data
// loader's WS plumbing. Splitting them keeps the page's <script>
// tight and gives the modal template a single binding root
// (`bind:value={formCtl.title}`) instead of eight loose `let`s.
//
// The buffers are deliberately separate from the items array so a
// "Cancel" press cleanly discards in-flight edits without mutating
// the on-disk record. The save() handler validates the title field
// is non-empty before firing the network call.

import { api, type HubItem } from '$lib/api';
import { toast } from '$lib/components/toast';

export interface HubFormController {
  /** Modal visibility. Bind directly from the template. */
  modalOpen: boolean;
  /** Edit target — null means "create new". Setting from outside is
   *  rare but the binding pair stays symmetric. */
  editing: HubItem | null;

  // Form buffers — bound to the modal inputs so cancel cleanly
  // discards without mutating the on-disk record.
  title: string;
  url: string;
  category: string;
  icon: string;
  notes: string;
  username: string;
  password: string;
  favorite: boolean;

  /** True while save() is in flight. Disables the submit button. */
  readonly saving: boolean;

  /** Open the modal in create mode — clears all buffers. */
  openCreate(): void;
  /** Open the modal in edit mode — hydrates buffers from the item. */
  openEdit(it: HubItem): void;
  /** Persist the current buffer (create or patch depending on
   *  editing). Closes the modal and triggers a reload on success. */
  save(): Promise<void>;
  /** Delete an item with a confirm() guard. Triggers a reload on
   *  success. Used both from the per-card menu and the modal's
   *  bottom-left "Delete" button. */
  remove(it: HubItem): Promise<void>;
  /** Patch the favorite flag with a single round-trip. Pulled out
   *  of remove() because the star toggle is a one-tap action that
   *  shouldn't require opening the modal. */
  toggleFavorite(it: HubItem): Promise<void>;
}

export interface HubFormDeps {
  /** Caller-owned reload — fires after every successful mutation so
   *  the items array reflects the change. Wired by the page to the
   *  data controller's load(). */
  reload: () => Promise<void> | void;
}

export function createHubForm(deps: HubFormDeps): HubFormController {
  // Modal + edit-target state.
  let modalOpen = $state(false);
  let editing = $state<HubItem | null>(null);

  // Form buffers.
  let title = $state('');
  let url = $state('');
  let category = $state('');
  let icon = $state('');
  let notes = $state('');
  let username = $state('');
  let password = $state('');
  let favorite = $state(false);
  let saving = $state(false);

  function openCreate() {
    editing = null;
    title = '';
    url = '';
    category = '';
    icon = '';
    notes = '';
    username = '';
    password = '';
    favorite = false;
    modalOpen = true;
  }

  function openEdit(it: HubItem) {
    editing = it;
    title = it.title;
    url = it.url ?? '';
    category = it.category ?? '';
    icon = it.icon ?? '';
    notes = it.notes ?? '';
    username = it.username ?? '';
    password = it.password ?? '';
    favorite = !!it.favorite;
    modalOpen = true;
  }

  async function save() {
    if (!title.trim()) {
      toast.warning('title is required');
      return;
    }
    saving = true;
    const payload: Partial<HubItem> = {
      title: title.trim(),
      url: url.trim() || undefined,
      category: category.trim() || undefined,
      icon: icon.trim() || undefined,
      notes: notes.trim() || undefined,
      username: username.trim() || undefined,
      password: password || undefined,
      favorite: favorite || undefined
    };
    try {
      if (editing) {
        await api.patchHubItem(editing.id, payload);
        toast.success('updated');
      } else {
        await api.createHubItem(payload);
        toast.success('added to hub');
      }
      modalOpen = false;
      await deps.reload();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function remove(it: HubItem) {
    if (!confirm(`Remove "${it.title}" from the hub?`)) return;
    try {
      await api.deleteHubItem(it.id);
      await deps.reload();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function toggleFavorite(it: HubItem) {
    try {
      await api.patchHubItem(it.id, { favorite: !it.favorite });
      await deps.reload();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get modalOpen() { return modalOpen; },
    set modalOpen(v) { modalOpen = v; },
    get editing() { return editing; },
    set editing(v) { editing = v; },
    get title() { return title; },
    set title(v) { title = v; },
    get url() { return url; },
    set url(v) { url = v; },
    get category() { return category; },
    set category(v) { category = v; },
    get icon() { return icon; },
    set icon(v) { icon = v; },
    get notes() { return notes; },
    set notes(v) { notes = v; },
    get username() { return username; },
    set username(v) { username = v; },
    get password() { return password; },
    set password(v) { password = v; },
    get favorite() { return favorite; },
    set favorite(v) { favorite = v; },
    get saving() { return saving; },
    openCreate,
    openEdit,
    save,
    remove,
    toggleFavorite
  };
}
