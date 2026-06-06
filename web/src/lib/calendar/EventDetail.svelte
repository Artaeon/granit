<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type CalendarEvent } from '$lib/api';
  import { eventStartDate, eventEndDate, fmtTime, eventTypeColor } from './utils';
  import { createEventDetailLoaders } from './eventDetailLoaders.svelte';
  import { createEventDetailDelete } from './eventDetailDelete.svelte';
  import { duplicateEventPlusOneWeek, createMeetingNoteForEvent } from './calendarEventMutations';
  import { findEventType } from './eventTypes';
  import TimeInput from './TimeInput.svelte';
  import RecurrenceEditor from './RecurrenceEditor.svelte';
  import EventTypeChips from './EventTypeChips.svelte';
  import RecurringScopePicker from './RecurringScopePicker.svelte';
  import ContentPanel from './ContentPanel.svelte';
  import { createEventDetailEdit } from './eventDetailEdit.svelte';
  import { colorOptions } from './eventDetailHelpers';

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

  // Project list + calendar source list — both loaded once at mount
  // by the loaders controller. Failure degrades silently to empty
  // arrays (the project picker hides itself; icsWritable falls back
  // to event.editable). Reactive reads via getters keep the markup
  // shapes unchanged.
  const loaders = createEventDetailLoaders();
  const projects = $derived(loaders.projects);
  const sources = $derived(loaders.sources);

  // Delete / skip / reset controller. Owns the inline scope picker
  // state (deletePrompt) and dispatches confirm / skip / reset
  // mutations. busy is shared with save / duplicate via setBusy so
  // only one mutation can run at a time and the action row's
  // disabled state stays consistent.
  const deleteCtl = createEventDetailDelete({
    getEvent: () => event,
    setBusy: (v) => { busy = v; },
    close: () => { open = false; },
    onChanged: () => onChanged?.()
  });
  const deletePrompt = $derived(deleteCtl.deletePrompt);

  // Edit-form controller — owns the entire buffer (title / date / time /
  // location / color / kind / project / content fields / rrule / scope)
  // and the save dispatcher across the four event paths (ICS instance,
  // ICS series, native instance override, native patch). Recurring-event
  // semantics live in the controller; the modal is just markup + binds.
  const editCtl = createEventDetailEdit({
    getEvent: () => event,
    setBusy: (v) => { busy = v; },
    close: () => { open = false; },
    onChanged: () => onChanged?.()
  });
  const editing = $derived(editCtl.editing);

  // ICS events from a writable calendar are editable through the
  // ics-events endpoints; events.json events keep their existing path.
  //
  // Source of truth: the calendar feed now stamps each ICS event
  // with editable=true/false directly (based on the file's location),
  // so we trust event.editable when present. Falls back to a sources
  // lookup for backward compatibility with older feed payloads or
  // events that pre-date the editable flag (e.g. in-memory entries
  // built from API responses that don't echo it). Without this, the
  // user's most common bug was: feed picks the writable copy of a
  // duplicated .ics file but EventDetail's source-lookup finds the
  // read-only one first and disables editing.
  let icsWritable = $derived.by(() => {
    if (event?.type !== 'ics_event') return false;
    if (typeof event.editable === 'boolean') return event.editable;
    if (!event.source) return false;
    const src = sources.find((s) => s.source === event.source);
    return !!src?.writable;
  });
  let editable = $derived((event?.type === 'event' && !!event?.eventId) || icsWritable);

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

  // Duplicate +1 week — thin wrapper that owns busy/close/onChanged
  // around the pure dispatcher in calendarEventMutations.
  async function duplicateEvent() {
    if (!event || busy) return;
    busy = true;
    try {
      const ok = await duplicateEventPlusOneWeek(event);
      if (ok) {
        onChanged?.();
        close();
      }
    } finally {
      busy = false;
    }
  }

  // Meeting-note creation — same wrapping pattern. The dispatcher
  // navigates on success; we still need to close the modal so the
  // notes route renders cleanly without the dialog overlay.
  let creatingMeetingNote = $state(false);
  async function createMeetingNote() {
    if (!event || creatingMeetingNote) return;
    creatingMeetingNote = true;
    try {
      const ok = await createMeetingNoteForEvent(event);
      if (ok) open = false;
    } finally {
      creatingMeetingNote = false;
    }
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
          <div class="text-xs uppercase tracking-wider text-dim flex items-center gap-1.5">
            <span>{event.type.replace('_', ' ')}</span>
            {#if event.kind}
              {@const evType = findEventType(event.kind)}
              {#if evType}
              <span aria-hidden="true">·</span>
              <span
                class="inline-flex items-center gap-1 px-1 py-0.5 text-[10px] font-medium border"
                style:color={`var(--color-${evType.color})`}
                style:border-color={`color-mix(in srgb, var(--color-${evType.color}) 45%, transparent)`}
                style:background={`color-mix(in srgb, var(--color-${evType.color}) 12%, transparent)`}
                title={evType.description}
              >
                <span class="font-mono">{evType.glyph}</span>
                <span>{evType.label}</span>
              </span>
              {/if}
            {/if}
          </div>
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
          {#if event.project_id}
            <!-- Project chip — links to the project page. Same surface
                 that drives the calendar's per-project filter; clicking
                 here jumps to the project's detail view. -->
            <a
              href={`/projects/${encodeURIComponent(event.project_id)}`}
              class="inline-flex items-center gap-1 text-xs px-2 py-0.5 mt-2 rounded-full bg-surface1 text-secondary border border-surface2 hover:bg-surface2"
              title="open project"
            >
              <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 7h7l2 2h9v11H3z" stroke-linejoin="round"/>
              </svg>
              {event.project_id}
            </a>
          {/if}
          {#if event.notePath}
            <div class="text-xs text-dim mt-2 font-mono truncate">{event.notePath}</div>
          {/if}
          {#if event.kind === 'content' && (event.status || (event.channels?.length ?? 0) > 0 || (event.tags?.length ?? 0) > 0)}
            <!-- Read-only content panel for the view-mode card. Status
                 + channels + tags only render when at least one is set
                 so a content event with no metadata yet stays clean. -->
            <div class="mt-2 flex flex-wrap items-center gap-1.5 text-[11px]">
              {#if event.status}
                <span class="px-1.5 py-0.5 rounded border border-lavender/40 bg-lavender/15 text-lavender uppercase tracking-wider text-[10px] font-semibold">{event.status}</span>
              {/if}
              {#each event.channels ?? [] as ch (ch)}
                <span class="px-1.5 py-0.5 rounded bg-surface1 text-subtext">{ch}</span>
              {/each}
              {#each event.tags ?? [] as t (t)}
                <span class="px-1.5 py-0.5 rounded bg-surface0 text-dim">#{t}</span>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      {#if editing && editable}
        <form onsubmit={editCtl.saveEdit} class="space-y-2 pt-2 border-t border-surface1">
          <input bind:value={editCtl.editTitle} required placeholder="title" class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Stack on mobile: a 3-column row crushes the date input below
               usable width on phones. The date gets its own row, then
               start/end share a row. -->
          <input type="date" bind:value={editCtl.editDate} required class="w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- 24-hour HH:MM picker — shared TimeInput keeps the same
               paired-select markup. Native <input type="time"> respects
               the OS locale, not the element's lang, so a US-locale
               user saw AM/PM on every event edit. The selects always
               show 24-hour values. step=5 matches the seed rounding
               in startEdit so the bound value lines up with an option. -->
          <TimeInput bind:startTime={editCtl.editStartTime} bind:endTime={editCtl.editEndTime} step={5} />
          <input bind:value={editCtl.editLocation} placeholder="location (optional)" class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text" />
          <!-- Edit scope picker — only relevant for recurring NATIVE
               events. 'this' writes a per-occurrence override (title /
               time / date / location / color of just this one); 'series'
               rewrites the parent so every occurrence shifts. ICS gets
               no scope picker because the patch endpoint has no override
               slot. Defaults to 'this' on open — same conservative
               default as the drag-move flow. -->
          {#if event?.type === 'event' && event?.rrule}
            <fieldset class="border border-surface1 p-2 space-y-1">
              <legend class="text-[10px] uppercase tracking-wider text-dim px-1">Apply to</legend>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editCtl.editScope} value="instance" name="ev-edit-scope" />
                <span>Just this occurrence</span>
                <span class="text-[10px] text-dim">— series base unchanged</span>
              </label>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editCtl.editScope} value="series" name="ev-edit-scope" />
                <span>The entire series</span>
                <span class="text-[10px] text-dim">— shifts every instance</span>
              </label>
            </fieldset>
          {/if}
          <!-- ICS recurring scope picker. ICS events have no
               first-class override path in our schema, but we can
               approximate "this occurrence only" by EXDATE'ing the
               source series and creating a standalone replacement
               VEVENT in the same .ics file — same observable result
               from the user's perspective. The series option keeps
               the existing path (rewrite the base VEVENT). -->
          {#if event?.type === 'ics_event' && event?.rrule && icsWritable}
            <fieldset class="border border-surface1 p-2 space-y-1">
              <legend class="text-[10px] uppercase tracking-wider text-dim px-1">Apply to · ICS</legend>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editCtl.editScope} value="instance" name="ev-edit-scope-ics" />
                <span>Just this occurrence</span>
                <span class="text-[10px] text-dim">— EXDATE + new standalone VEVENT</span>
              </label>
              <label class="flex items-center gap-2 text-xs text-text cursor-pointer">
                <input type="radio" bind:group={editCtl.editScope} value="series" name="ev-edit-scope-ics" />
                <span>The entire series</span>
                <span class="text-[10px] text-dim">— rewrites the base VEVENT</span>
              </label>
            </fieldset>
          {/if}
          <!-- Repeat picker — same shape as CreateEvent so the muscle
               memory is identical. ICS events DO get this shown for
               read-only feedback (the rrule of the source series),
               but the patch path for ICS doesn't currently accept
               rrule changes — surfacing the rule still helps the
               user understand what's recurring. The picker is
               disabled when the user is editing a single occurrence
               (recurrence is a series-level concept). -->
          {#if event?.type === 'event' && (!event?.rrule || editCtl.editScope === 'series')}
            <RecurrenceEditor
              bind:rrule={editCtl.editRRule}
              layout="inline"
              minDate={editCtl.editDate}
              idPrefix="ev-edit"
            />
          {/if}
          <!-- Project link picker — drives the calendar's project
               filter + colour-by-project overlay. Hidden when no
               projects exist (fresh vault); ICS events get it too
               so a writable .ics calendar can carry project links
               via the events.json sidecar (the link is on the
               native event record, not in the ICS payload). -->
          {#if event?.type === 'event' && projects.length > 0}
            <div class="flex items-center gap-2 flex-wrap">
              <label class="text-[11px] text-dim uppercase tracking-wider" for="ev-edit-project">Project</label>
              <select
                id="ev-edit-project"
                bind:value={editCtl.editProjectId}
                class="bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text"
              >
                <option value="">No project</option>
                {#each projects as p (p.name)}
                  <option value={p.name}>{p.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          <!-- Event-type picker. Same catalog + chip shape as
               CreateEvent so the muscle memory is identical. Empty
               state = no type; clicking the active chip clears it. -->
          <div>
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1.5">Type</span>
            <EventTypeChips bind:kind={editCtl.editKind} chipSize="compact" />
          </div>
          <!-- Content-pipeline panel — only when the user has selected
               the 'content' kind. Status / channels / tags live here.
               Switching kind away hides the panel; the values stay in
               local state so flipping back-and-forth doesn't lose work
               mid-edit. -->
          {#if editCtl.editKind === 'content'}
            <ContentPanel
              status={editCtl.editStatus}
              channels={editCtl.editChannels}
              tags={editCtl.editTags}
              onStatusChange={(s) => (editCtl.editStatus = s)}
              onChannelsChange={(c) => (editCtl.editChannels = c)}
              onTagsChange={(t) => (editCtl.editTags = t)}
            />
          {/if}
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-dim uppercase tracking-wider">Color</span>
            {#each colorOptions as c (c.name)}
              <button
                type="button"
                onclick={() => (editCtl.editColor = c.name)}
                aria-label={c.name}
                title={c.name}
                class="w-5 h-5 rounded-full border-2 {editCtl.editColor === c.name ? 'border-text' : 'border-surface1'}"
                style="background: {c.hex}"
              ></button>
            {/each}
          </div>
          <div class="flex gap-2">
            <button type="submit" disabled={busy} class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded disabled:opacity-50">save</button>
            <button type="button" onclick={editCtl.cancelEdit} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
            <span class="flex-1"></span>
            <button type="button" onclick={deleteCtl.deleteEvent} disabled={busy} class="px-3 py-1.5 text-sm text-error hover:bg-surface0 rounded">delete</button>
          </div>
        </form>
      {:else}
      <!-- Inline delete-scope picker. Replaces the previous two-confirm
           pattern where the destructive 'delete entire series' branch
           sat behind a Cancel keystroke — too easy to trigger by
           reflexively pressing Esc/Cancel to abort. Three explicit
           buttons; nothing happens until one is clicked. -->
      {#if deletePrompt !== 'none'}
        <RecurringScopePicker
          eventTitle={event.title}
          action="delete"
          onChoose={(scope) => {
            if (scope === 'this') void deleteCtl.confirmDeleteOccurrence();
            else if (scope === 'series') void deleteCtl.confirmDeleteSeries();
          }}
          onCancel={deleteCtl.cancelDeletePrompt}
          {busy}
        />
      {/if}
      <div class="flex flex-wrap gap-2 pt-2 border-t border-surface1" class:opacity-40={deletePrompt !== 'none'}>
        {#if event.taskId}
          <button onclick={toggleDone} disabled={busy} class="px-3 py-1.5 text-sm bg-surface0 text-success rounded hover:bg-surface1 disabled:opacity-50">
            {event.done ? 'mark not done' : 'mark done'}
          </button>
          {#if event.start}
            <button onclick={clearSchedule} disabled={busy} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
              unschedule
            </button>
          {/if}
        {/if}
        {#if editable}
          <button onclick={editCtl.startEdit} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">edit</button>
          <!-- Duplicate the event +1 week ahead — common "repeat
               last week's structure for next week" workflow without
               needing to set up a full RRULE. Hidden for tasks /
               deadlines / read-only ICS sources via `editable`. -->
          <button
            onclick={duplicateEvent}
            disabled={busy}
            class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1"
            title="Create a copy of this event one week from now"
          >+1 week</button>
          {#if event.type === 'event' && event.rrule}
            <!-- Skip THIS occurrence only — adds an EXDATE so the
                 expander filters this single instance from future
                 renders. Series stays intact. The text reads as a
                 distinct verb from 'delete' so the user's mental
                 model of cancel-once vs end-series stays clear. -->
            <button
              onclick={deleteCtl.skipOccurrence}
              disabled={busy}
              class="px-3 py-1.5 text-sm bg-surface0 text-warning rounded hover:bg-surface1"
              title="Cancel just this occurrence — keep the rest of the series"
            >skip this</button>
          {/if}
          {#if event.type === 'event' && event.override_key}
            <!-- This occurrence has a per-instance override (set via
                 drag-move or the 'just this' edit scope). One-click
                 to drop the override and surface the series default
                 again. Hidden when override_key is empty (plain
                 occurrence or non-recurring event) so the action
                 row doesn't grow buttons that wouldn't do anything. -->
            <button
              onclick={deleteCtl.resetOccurrence}
              disabled={busy}
              class="px-3 py-1.5 text-sm bg-surface0 text-info rounded hover:bg-surface1"
              title="Drop the per-occurrence override and inherit the series defaults"
            >reset this</button>
          {/if}
          <button
            onclick={deleteCtl.deleteEvent}
            disabled={busy || deletePrompt !== 'none'}
            class="px-3 py-1.5 text-sm text-error hover:bg-surface0 rounded disabled:opacity-50"
            title={event.rrule ? 'Pick scope: this occurrence or the entire series' : 'Delete this event'}
          >{event.rrule ? 'delete…' : 'delete'}</button>
        {/if}
        {#if event.notePath}
          <button onclick={openNote} class="px-3 py-1.5 text-sm bg-surface0 text-subtext rounded hover:bg-surface1">
            open note
          </button>
        {/if}
        <button
          onclick={createMeetingNote}
          disabled={creatingMeetingNote}
          class="px-3 py-1.5 text-sm bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50"
          title="Create a meeting note for this event with frontmatter"
        >
          {creatingMeetingNote ? 'creating…' : '✎ meeting note'}
        </button>
        <span class="flex-1"></span>
        <button onclick={close} class="px-3 py-1.5 text-sm text-subtext hover:text-text">close</button>
      </div>
      {/if}
    </div>
  </div>
{/if}

