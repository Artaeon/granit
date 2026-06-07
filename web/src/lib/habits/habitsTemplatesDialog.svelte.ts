// Templates-dialog controller — owns open/close state, the currently
// previewed template, and the apply-template flow. Surface is
// intentionally narrow: the dialog component reads `open`, the
// header button calls `toggle()`, and a click on "Add all" runs
// `applyTemplate()`. The controller imports the catalog directly so
// the page doesn't have to plumb the templates list through props.
//
// The apply flow tries createHabit first (metadata in one call). If
// the server rejects the metadata-bearing body (older deployments
// where /habits POST only accepts {name}), we retry name-only then
// patch the metadata on. This keeps the templates feature working
// against any backend that at least exposes habit creation.

import { api } from '$lib/api';
import { HABIT_TEMPLATES, type HabitTemplate } from './habitsTemplates';

export interface TemplatesDialogControllerDeps {
  /** Refresh the habits list after the apply lands. The dialog
   *  closes itself on success so the new habits land visibly. */
  reload: () => Promise<void>;
}

export interface TemplatesDialogController {
  /** Whether the modal renders. Bindable so a wrapper component
   *  can use `bind:open` if the consumer prefers. */
  open: boolean;
  /** Currently previewed template (the one whose habits the dialog
   *  is listing under "Habits in this template"). null = no
   *  selection, list mode. */
  previewId: string | null;
  /** True while an apply is in flight — disables the "Add all"
   *  button and surfaces a "creating…" hint. */
  readonly applying: boolean;
  /** The full catalog. Exposed read-only so the dialog component
   *  iterates over it. */
  readonly templates: HabitTemplate[];
  /** Lookup helper — the preview pane reads
   *  `ctl.preview` to render the selected template's habits. */
  readonly preview: HabitTemplate | null;

  /** Open the dialog. Resets preview selection so a re-open starts
   *  clean. */
  openDialog(): void;
  /** Close + reset. Wired to Esc, click-outside, and the explicit
   *  Cancel button. */
  close(): void;
  /** Toggle a template into / out of preview mode. Click the same
   *  template twice to collapse the preview. */
  selectPreview(id: string): void;
  /** Apply the named template — fans out createHabit calls in
   *  parallel, falls back to create-then-patch on metadata reject,
   *  surfaces a single coalesced toast at the end. */
  applyTemplate(id: string): Promise<void>;
}

export function createTemplatesDialogCtl(
  deps: TemplatesDialogControllerDeps
): TemplatesDialogController {
  let open = $state(false);
  let previewId = $state<string | null>(null);
  let applying = $state(false);

  const templates = HABIT_TEMPLATES;
  const preview = $derived(
    previewId ? templates.find((t) => t.id === previewId) ?? null : null
  );

  function openDialog() {
    previewId = null;
    open = true;
  }
  function close() {
    open = false;
    previewId = null;
  }
  function selectPreview(id: string) {
    previewId = previewId === id ? null : id;
  }

  async function createOne(item: HabitTemplate['habits'][number]) {
    // Try the metadata-bearing create first. If the backend rejects
    // the body shape (older deployment that only accepts {name}),
    // retry name-only then patch the metadata on.
    try {
      await api.createHabit({
        name: item.name,
        category: item.category,
        tags: item.tags,
        frequency: item.frequency,
        reminderTime: item.reminderTime
      });
    } catch {
      await api.createHabit({ name: item.name });
      const patch: Record<string, unknown> = {};
      if (item.category) patch.category = item.category;
      if (item.tags && item.tags.length) patch.tags = item.tags;
      if (item.frequency) patch.frequency = item.frequency;
      if (item.reminderTime) patch.reminderTime = item.reminderTime;
      if (Object.keys(patch).length > 0) {
        await api.patchHabit(item.name, patch);
      }
    }
  }

  async function applyTemplate(id: string) {
    const tpl = templates.find((t) => t.id === id);
    if (!tpl || applying) return;
    applying = true;
    const failed: string[] = [];
    await Promise.all(
      tpl.habits.map(async (item) => {
        try {
          await createOne(item);
        } catch {
          failed.push(item.name);
        }
      })
    );
    applying = false;
    const { toast } = await import('$lib/components/toast');
    const added = tpl.habits.length - failed.length;
    if (failed.length === 0) {
      toast.success(`added ${added} habit${added === 1 ? '' : 's'} from "${tpl.name}"`);
    } else if (added > 0) {
      toast.warning(`added ${added}, failed: ${failed.join(', ')}`);
    } else {
      toast.error(`couldn't add habits from "${tpl.name}"`);
    }
    await deps.reload();
    close();
  }

  return {
    get open() {
      return open;
    },
    set open(v) {
      open = v;
    },
    get previewId() {
      return previewId;
    },
    set previewId(v) {
      previewId = v;
    },
    get applying() {
      return applying;
    },
    get templates() {
      return templates;
    },
    get preview() {
      return preview;
    },
    openDialog,
    close,
    selectPreview,
    applyTemplate
  };
}
