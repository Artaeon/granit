<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type CalendarEvent, type CalendarSource } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onMount } from 'svelte';
  import { eventStartDate, eventEndDate, fmtTime, eventTypeColor } from './utils';

  let {
    open = $bindable(false),
    event,
    onChanged
  }: {
    open?: boolean;
    event: CalendarEvent | null;
    onChanged?: () => void;
  } = $props();

  let busy = $state(false);
  let editing = $state(false);
  let editTitle = $state('');
  let editDate = $state('');
  let editLocation = $state('');
  let editColor = $state('cyan');

  // 24-hour HH:MM picker buffers — same pattern as UnifiedCreate.
  // Native <input type="time"> renders AM/PM on most OS locales
  // regardless of any lang attribute. Custom HH + MM selects
  // guarantee 24-hour display everywhere.
  let editStartH = $state(0);
  let editStartM = $state(0);
  let editEndH = $state(0);
  let editEndM = $state(0);
  // editStartTime / editEndTime are DERIVED, not state-with-effect.
  // The previous $effect-driven sync had a flush race: the user
  // could change a select and immediately click Save, and the
  // submit handler would read a stale time string that hadn't
  // picked up the new H/M yet. Events landed at the previous
  // time, not the one the user just chose. The user reported this
  // as "editing the times also not, they get scheduled somewhere".
  // $derived is read synchronously on access — no flush race.
  let editStartTime = $derived(
    `${String(editStartH).padStart(2, '0')}:${String(editStartM).padStart(2, '0')}`
  );
  let editEndTime = $derived(
    `${String(editEndH).padStart(2, '0')}:${String(editEndM).padStart(2, '0')}`
  );

  // Calendar sources — needed to know if an ICS event came from a
  // writable .ics file. Loaded once on mount; refreshed on demand.
  let sources = $state<CalendarSource[]>([]);
  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      sources = r.sources;
    } catch {
      sources = [];
    }
  }
  onMount(loadSources);

  // ICS events from a writable calendar are editable through the
  // ics-events endpoints; events.json events keep their existing path.
  let icsWritable = $derived.by(() => {
    if (event?.type !== 'ics_event' || !event.source) return false;
    const src = sources.find((s) => s.source === event.source);
    return !!src?.writable;
  });
  let editable = $derived((event?.type === 'event' && !!event?.eventId) || icsWritable);

  function startEdit() {
    if (!event) return;
    editTitle = event.title;
    editDate = event.date ?? (event.start ? event.start.slice(0, 10) : '');
    // editStartTime / editEndTime are derived from H+M now — seed
    // the H/M state instead. Direct string assignment to the
    // derived bindings would throw at runtime in Svelte 5.
    editLocation = event.location ?? '';
    editColor = event.color ?? 'cyan';
    // Seed the 24-hour selects from the event's start/end so the
    // picker shows the current value when edit-mode opens. Round
    // minutes to the nearest 5 to align with the select options;
    // the underlying time string still carries the exact value
    // until the user changes it.
    if (event.start) {
      const sd = new Date(event.start);
      editStartH = sd.getHours();
      editStartM = Math.round(sd.getMinutes() / 5) * 5;
      if (editStartM === 60) { editStartH = (editStartH + 1) % 24; editStartM = 0; }
    } else {
      editStartH = 0;
      editStartM = 0;
    }
    if (event.end) {
      const ed = new Date(event.end);
      editEndH = ed.getHours();
      editEndM = Math.round(ed.getMinutes() / 5) * 5;
      if (editEndM === 60) { editEndH = (editEndH + 1) % 24; editEndM = 0; }
    } else {
      editEndH = 0;
      editEndM = 0;
    }
    editing = true;
  }

  // Build an RFC3339 string from YYYY-MM-DD + HH:MM. Mirrors what the
  // create form posts so the server's parser sees the same shape on
  // both endpoints.
  function localRFC3339FromParts(date: string, time: string): string {
    const [y, mo, d] = date.split('-').map(Number);
    const [h, mi] = time.split(':').map(Number);
    return new Date(y, mo - 1, d, h, mi, 0, 0).toISOString();
  }

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!event) return;
    busy = true;
    try {
      if (event.type === 'ics_event' && event.source && event.eventId) {
        await api.patchICSEvent(event.source, event.eventId, {
          summary: editTitle,
          start: localRFC3339FromParts(editDate, editStartTime || '00:00'),
          end: editEndTime ? localRFC3339FromParts(editDate, editEndTime) : undefined,
          location: editLocation
        });
      } else if (event.eventId) {
        await api.patchEvent(event.eventId, {
          title: editTitle,
          date: editDate,
          start_time: editStartTime,
          end_time: editEndTime,
          location: editLocation,
          color: editColor
        });
      } else {
        return;
      }
      editing = false;
      onChanged?.();
      open = false;
      toast.success('event updated');
    } catch (err) {
      toast.error('save failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      busy = false;
    }
  }

  async function deleteEvent() {
    if (!event?.eventId) return;
    if (!confirm(`Delete event "${event.title}"?`)) return;
    busy = true;
    try {
      if (event.type === 'ics_event' && event.source) {
        await api.deleteICSEvent(event.source, event.eventId);
      } else {
        await api.deleteEvent(event.eventId);
      }
      onChanged?.();
      open = false;
      toast.success('event deleted');
    } catch (err) {
      toast.error('delete failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      busy = false;
    }
  }

  const colorOptions = ['red', 'yellow', 'orange', 'green', 'blue', 'purple', 'cyan'];

  async function toggleDone() {
    if (!event?.taskId) return;
    busy = true;
    try {
      await api.patchTask(event.taskId, { done: !event.done });
      onChanged?.();
      open = false;
    } finally {
      busy = false;
    }
  }
  async function clearSchedule() {
    if (!event?.taskId) return;
    busy = true;
    try {
      await api.patchTask(event.taskId, { clearSchedule: true });
      onChanged?.();
      open = false;
    } finally {
      busy = false;
    }
  }

  function openNote() {
    if (event?.notePath) goto(`/notes/${encodeURIComponent(event.notePath)}`);
    open = false;
  }

  function close() { open = false; }
</script>

{#if open && event}
  {@const c = eventTypeColor(event)}
  {@const start = eventStartDate(event)}
  {@const end = eventEndDate(event)}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-end sm:items-center justify-center sm:p-4"
    onclick={close}
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
    role="dialog"
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg p-5 space-y-3 max-h-[90dvh] overflow-y-auto pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] sm:pb-5"
      role="document"
    >
      <div class="flex items-start gap-3">
        <div class="w-1 self-stretch rounded-full" style="background: {c.border}"></div>
        <div class="flex-1">
          <div class="text-xs uppercase tracking-wider text-dim">{event.type.replace('_', ' ')}</div>
          <h2 class="text-lg font-semibold text-text {event.done ? 'line-through opacity-70' : ''}">{event.title}</h2>
          {#if start}
            <div class="text-sm text-subtext mt-1">
              {start.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}
              {#if event.start} · {fmtTime(start)}{#if end} – {fmtTime(end)}{/if}{/if}
            </div>
          {:else if event.date}
            <div class="text-sm text-subtext mt-1">{event.date}</div>
          {/if}
          {#if event.location}
            <div class="text-sm text-dim mt-1">@ {event.location}</div>
          {/if}
          {#if event.notePath}
            <div class="text-xs text-dim mt-2 font-mono truncate">{event.notePath}</div>
          {/if}
        </div>
      </div>

      {#if editing && editable}
        <form onsubmit={saveEdit} class="space-y-2 pt-2 border-t border-surface1">
          <input bind:value={editTitle} required placeholder="title" class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Stack on mobile: a 3-column row crushes the date input below
               usable width on phones. The date gets its own row, then
               start/end share a row. -->
          <input type="date" bind:value={editDate} required class="w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- 24-hour HH:MM picker — paired selects, same pattern as
               UnifiedCreate. Native <input type="time"> respects the
               OS locale, not the element's lang, so a US-locale user
               saw AM/PM on every event edit. These selects always
               show 24-hour values. -->
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">Start (24h)</span>
              <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
                <select
                  bind:value={editStartH}
                  aria-label="start hour"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 24 }, (_, i) => i) as h}
                    <option value={h}>{String(h).padStart(2, '0')}</option>
                  {/each}
                </select>
                <span class="text-dim px-1">:</span>
                <select
                  bind:value={editStartM}
                  aria-label="start minute"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 12 }, (_, i) => i * 5) as m}
                    <option value={m}>{String(m).padStart(2, '0')}</option>
                  {/each}
                </select>
              </div>
            </div>
            <div>
              <span class="block text-[10px] uppercase tracking-wider text-dim mb-1">End (24h)</span>
              <div class="flex items-center bg-surface0 border border-surface1 rounded overflow-hidden focus-within:border-primary">
                <select
                  bind:value={editEndH}
                  aria-label="end hour"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 24 }, (_, i) => i) as h}
                    <option value={h}>{String(h).padStart(2, '0')}</option>
                  {/each}
                </select>
                <span class="text-dim px-1">:</span>
                <select
                  bind:value={editEndM}
                  aria-label="end minute"
                  class="time-select flex-1 px-2 py-2 text-sm text-text font-mono tabular-nums focus:outline-none"
                >
                  {#each Array.from({ length: 12 }, (_, i) => i * 5) as m}
                    <option value={m}>{String(m).padStart(2, '0')}</option>
                  {/each}
                </select>
              </div>
            </div>
          </div>
          <input bind:value={editLocation} placeholder="location (optional)" class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
            {#each colorOptions as c}
              <button
                type="button"
                onclick={() => (editColor = c)}
                aria-label={c}
                class="w-5 h-5 rounded-full border-2 {editColor === c ? 'border-text' : 'border-surface1'}"
                style="background: var(--color-{c === 'red' ? 'error' : c === 'yellow' ? 'warning' : c === 'orange' ? 'accent' : c === 'green' ? 'success' : c === 'blue' ? 'secondary' : c === 'purple' ? 'primary' : 'info'})"
              ></button>
            {/each}
          </div>
          <div class="flex gap-2">
            <button type="submit" disabled={busy} class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded disabled:opacity-50">save</button>
            <button type="button" onclick={() => (editing = false)} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
            <span class="flex-1"></span>
            <button type="button" onclick={deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-error/10 rounded">delete</button>
          </div>
        </form>
      {:else}
      <div class="flex flex-wrap gap-2 pt-2 border-t border-surface1">
        {#if event.taskId}
          <button onclick={toggleDone} disabled={busy} class="px-3 py-1.5 text-sm bg-success/20 text-success rounded hover:bg-success/30 disabled:opacity-50">
            {event.done ? 'mark not done' : 'mark done'}
          </button>
          {#if event.start}
            <button onclick={clearSchedule} disabled={busy} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
              unschedule
            </button>
          {/if}
        {/if}
        {#if editable}
          <button onclick={startEdit} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">edit</button>
          <button onclick={deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-error/10 rounded">delete</button>
        {/if}
        {#if event.notePath}
          <button onclick={openNote} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
            open note
          </button>
        {/if}
        <span class="flex-1"></span>
        <button onclick={close} class="px-3 py-1.5 text-sm text-subtext hover:text-text">close</button>
      </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* Native <select> dropdown panel is rendered with OS chrome and
     defaults to white-on-white in dark mode. Same fix as
     UnifiedCreate: hint color-scheme + explicit option colors so
     the dropdown is readable on every browser/OS. */
  .time-select {
    color-scheme: dark;
    background: var(--color-surface0);
  }
  .time-select option {
    background: var(--color-base);
    color: var(--color-text);
  }
  :global([data-theme="light"]) .time-select {
    color-scheme: light;
  }
</style>
