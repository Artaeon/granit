<script lang="ts">
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { fmtDateISO } from './utils';

  // Unified create modal — task or event — invoked from drag-to-create on
  // the calendar grid OR from the sidebar buttons. Single dialog so the
  // user picks "task vs event" at the point of creation; same start/duration
  // can flow through either branch.

  let {
    open = $bindable(false),
    start,        // Date the user dragged from
    end,          // Date the user dragged to (or start+30min if no drag)
    defaultKind = 'task',
    defaultNotePath,
    onCreated
  }: {
    open?: boolean;
    start: Date;
    end: Date;
    defaultKind?: 'task' | 'event';
    defaultNotePath?: string;
    onCreated: () => void | Promise<void>;
  } = $props();

  let kind = $state<'task' | 'event'>(defaultKind);
  let title = $state('');
  let dateISO = $state('');
  let startTime = $state('');
  let endTime = $state('');
  let notePath = $state('');
  let priority = $state(0);
  let location = $state('');
  // Empty default → eventTypeColor falls through to the per-title hash
  // rotate, so a series of drag-created events get distinct hues without
  // the user picking each one. Picking a color here flips it explicit.
  let color = $state('');
  let saving = $state(false);
  let titleEl: HTMLInputElement | undefined = $state();

  // Re-seed every time the modal opens — `start`/`end` reflect the most
  // recent drag, so the buffer must follow.
  $effect(() => {
    if (!open) return;
    kind = defaultKind;
    dateISO = fmtDateISO(start);
    startTime = `${String(start.getHours()).padStart(2, '0')}:${String(start.getMinutes()).padStart(2, '0')}`;
    endTime = `${String(end.getHours()).padStart(2, '0')}:${String(end.getMinutes()).padStart(2, '0')}`;
    title = '';
    notePath = defaultNotePath ?? `Jots/${dateISO}.md`;
    priority = 0;
    location = '';
    color = '';
    setTimeout(() => titleEl?.focus(), 50);
  });

  function close() { open = false; }

  function durationMinutes(): number {
    const [sh, sm] = startTime.split(':').map(Number);
    const [eh, em] = endTime.split(':').map(Number);
    return Math.max(15, (eh * 60 + em) - (sh * 60 + sm));
  }

  function startISO(): string {
    // Build local-time RFC3339 so the server's parser uses the user's TZ.
    const [h, m] = startTime.split(':').map(Number);
    const d = new Date(start);
    d.setHours(h, m, 0, 0);
    return d.toISOString();
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    saving = true;
    try {
      if (kind === 'task') {
        await api.createTask({
          notePath,
          text: title.trim(),
          priority: priority || undefined,
          scheduledStart: startISO(),
          durationMinutes: durationMinutes(),
          section: '## Tasks'
        });
      } else {
        await api.createEvent({
          title: title.trim(),
          date: dateISO,
          start_time: startTime,
          end_time: endTime,
          location: location.trim(),
          color
        });
      }
      close();
      await onCreated();
      toast.success(kind === 'task' ? 'task scheduled' : 'event created');
    } catch (err) {
      toast.error('save failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }

  const colorOptions = ['red', 'yellow', 'orange', 'green', 'blue', 'purple', 'cyan'];
  function tone(c: string): string {
    const map: Record<string, string> = { red: 'error', yellow: 'warning', orange: 'accent', green: 'success', blue: 'secondary', purple: 'primary', cyan: 'info' };
    return `var(--color-${map[c] ?? 'info'})`;
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={close}
    role="dialog"
    tabindex="-1"
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md bg-mantle border border-surface1 rounded-lg shadow-xl"
      role="document"
    >
      <header class="px-5 py-3 border-b border-surface1 flex items-center gap-2">
        <h2 class="text-base font-semibold text-text flex-1">New {kind}</h2>
        <button onclick={close} class="text-dim hover:text-text" aria-label="close">×</button>
      </header>

      <!-- Type toggle: task ↔ event. Big and obvious so the user can flip
           after a drag without re-doing the time selection. -->
      <div class="px-5 pt-4">
        <div class="flex bg-surface0 border border-surface1 rounded-lg overflow-hidden">
          <button
            type="button"
            onclick={() => (kind = 'task')}
            class="flex-1 px-3 py-2 text-sm flex items-center justify-center gap-2 {kind === 'task' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="6" height="6" rx="1"/><path d="m4.5 8 1 1 2-2M13 7h8M13 17h8M3 17h6"/></svg>
            Task
          </button>
          <button
            type="button"
            onclick={() => (kind = 'event')}
            class="flex-1 px-3 py-2 text-sm flex items-center justify-center gap-2 {kind === 'event' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/></svg>
            Event
          </button>
        </div>
      </div>

      <form onsubmit={submit} class="p-5 space-y-3">
        <input
          bind:this={titleEl}
          bind:value={title}
          required
          placeholder={kind === 'task' ? 'what needs doing?' : 'event title'}
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded-lg text-sm text-text focus:outline-none focus:border-primary"
        />

        <!-- Time row — shared between task & event branches. -->
        <div class="grid grid-cols-3 gap-2">
          <input type="date" bind:value={dateISO} required class="px-2 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text" />
          <input type="time" bind:value={startTime} required class="px-2 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text" />
          <input type="time" bind:value={endTime} required class="px-2 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text" />
        </div>
        <div class="text-[11px] text-dim text-center -mt-1">{durationMinutes()} min</div>

        {#if kind === 'task'}
          <input
            bind:value={notePath}
            placeholder="note path (vault-relative .md)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text font-mono"
          />
          <div class="flex items-center gap-2">
            <span class="text-xs text-dim flex-shrink-0">Priority</span>
            <div class="flex bg-surface0 border border-surface1 rounded-lg overflow-hidden text-xs">
              {#each [{ p: 0, label: 'none' }, { p: 1, label: 'P1' }, { p: 2, label: 'P2' }, { p: 3, label: 'P3' }] as o}
                <button
                  type="button"
                  onclick={() => (priority = o.p)}
                  class="px-3 py-1.5 {priority === o.p ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
                >{o.label}</button>
              {/each}
            </div>
          </div>
        {:else}
          <input
            bind:value={location}
            placeholder="location (optional)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded-lg text-sm text-text"
          />
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
            {#each colorOptions as c}
              <button
                type="button"
                onclick={() => (color = c)}
                aria-label={c}
                class="w-6 h-6 rounded-full border-2 {color === c ? 'border-text' : 'border-surface1'}"
                style="background: {tone(c)}"
              ></button>
            {/each}
          </div>
        {/if}

        <button
          type="submit"
          disabled={!title.trim() || saving}
          class="w-full px-4 py-2.5 bg-primary text-mantle rounded-lg font-medium disabled:opacity-50"
        >{saving ? 'creating…' : `Create ${kind}`}</button>
      </form>
    </div>
  </div>
{/if}
