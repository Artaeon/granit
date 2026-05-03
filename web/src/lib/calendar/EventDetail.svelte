<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type CalendarEvent } from '$lib/api';
  import { toast } from '$lib/components/toast';
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
  let editStartTime = $state('');
  let editEndTime = $state('');
  let editLocation = $state('');
  let editColor = $state('cyan');

  // Editable only for events backed by events.json (type === 'event' with eventId)
  let editable = $derived(event?.type === 'event' && !!event?.eventId);

  function startEdit() {
    if (!event) return;
    editTitle = event.title;
    editDate = event.date ?? (event.start ? event.start.slice(0, 10) : '');
    editStartTime = event.start ? new Date(event.start).toTimeString().slice(0, 5) : '';
    editEndTime = event.end ? new Date(event.end).toTimeString().slice(0, 5) : '';
    editLocation = event.location ?? '';
    editColor = event.color ?? 'cyan';
    editing = true;
  }

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!event?.eventId) return;
    busy = true;
    try {
      await api.patchEvent(event.eventId, {
        title: editTitle,
        date: editDate,
        start_time: editStartTime,
        end_time: editEndTime,
        location: editLocation,
        color: editColor
      });
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
      await api.deleteEvent(event.eventId);
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
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={close}
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
    role="dialog"
    tabindex="-1"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md bg-mantle border border-surface1 rounded-lg p-5 space-y-3"
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
          <div class="grid grid-cols-3 gap-2">
            <input type="date" bind:value={editDate} required class="px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
            <input type="time" bind:value={editStartTime} placeholder="start" class="px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
            <input type="time" bind:value={editEndTime} placeholder="end" class="px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
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
