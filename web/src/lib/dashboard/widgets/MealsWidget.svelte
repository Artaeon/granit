<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type MealsResponse, type MealSlot } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';

  // MealsWidget — three (or more) per-day meal slots rendered as a
  // compact check list. The goal is glanceable "did I eat today?"
  // visibility, NOT calorie tracking — the user explicitly asked for
  // simple. Source of truth is the daily note's `## Meals` section
  // (server-side: internal/meals), so a row ticked here shows up in
  // the calendar's meal_slot strip immediately and a row edited
  // directly in the daily note syncs back.
  //
  // Why not the habits widget? Different cadence: meals happen 3+ times
  // per day with time + optional "what I ate" capture, while habits are
  // one-tick-per-day rows with streak math.

  let data = $state<MealsResponse | null>(null);
  let loaded = $state(false);
  let loadError = $state('');
  let busyKey = $state<string | null>(null);
  // Debounced text-input state — local per-row buffer keyed by
  // (time|name) so a fast typist doesn't fire a PATCH per keystroke.
  let drafts = $state<Record<string, string>>({});
  let saveTimers: Record<string, ReturnType<typeof setTimeout>> = {};

  function slotKey(s: { time: string; name: string }): string {
    return `${s.time}|${s.name.toLowerCase()}`;
  }

  async function load() {
    loadError = '';
    try {
      data = await api.listMeals();
    } catch (e) {
      // Don't toast on background reload failure — the inline retry
      // state below is enough signal without spamming a global toast
      // (this widget reloads on every note.changed). User-driven
      // actions DO toast, since the user just initiated something
      // and deserves immediate feedback.
      console.warn('[meals] load failed:', e);
      data = null;
      loadError = errorMessage(e);
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      // Today's daily note touched anywhere reloads us. Filter by
      // path prefix would require knowing the daily folder name —
      // for now any note.changed triggers reload; cheap enough.
      if (ev.type === 'note.changed') load();
    });
  });

  async function toggle(slot: MealSlot) {
    const key = slotKey(slot);
    if (busyKey === key) return;
    busyKey = key;
    // Optimistic flip so the tick feels instant.
    if (data) {
      data.slots = data.slots.map((s) =>
        slotKey(s) === key ? { ...s, done: !s.done } : s
      );
      data.done = data.slots.filter((s) => s.done).length;
    }
    try {
      await api.patchMeal({
        time: slot.time,
        name: slot.name,
        date: data?.date,
        done: !slot.done
      });
      // Server returns canonical state — re-sync to clear any drift.
      await load();
    } catch (e) {
      toast.error(`couldn't tick ${slot.name}: ${errorMessage(e)}`);
      await load();
    } finally {
      busyKey = null;
    }
  }

  function onTextInput(slot: MealSlot, value: string) {
    const key = slotKey(slot);
    drafts[key] = value;
    if (saveTimers[key]) clearTimeout(saveTimers[key]);
    // 600ms debounce — long enough that mid-word pauses don't fire,
    // short enough that the user trusts the autosave by the time
    // they look away.
    saveTimers[key] = setTimeout(() => void saveText(slot, value), 600);
  }

  async function saveText(slot: MealSlot, value: string) {
    const key = slotKey(slot);
    const trimmed = value.trim();
    if (trimmed === (slot.text ?? '')) {
      // No-op but still clear the draft — keeps displayText reading
      // from canonical slot.text after the user has finished typing.
      delete drafts[key];
      return;
    }
    try {
      await api.patchMeal({
        time: slot.time,
        name: slot.name,
        date: data?.date,
        text: trimmed
      });
      // Drop the local draft so any subsequent server-driven reload
      // (e.g. another tab editing the same slot) wins. Without this
      // the stale buffer would shadow the canonical text forever.
      delete drafts[key];
      await load();
    } catch (e) {
      toast.error(`couldn't save ${slot.name} text: ${errorMessage(e)}`);
    }
  }

  function displayText(slot: MealSlot): string {
    const key = slotKey(slot);
    if (key in drafts) return drafts[key];
    return slot.text ?? '';
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Meals</h2>
    <span class="flex-1"></span>
    {#if data && data.total > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">
        {data.done}/{data.total}
      </span>
    {/if}
  </div>

  {#if !loaded}
    <ul class="space-y-2" aria-hidden="true">
      {#each Array(3) as _, i (i)}
        <li class="flex items-center gap-2">
          <span class="w-4 h-4 rounded bg-surface1 animate-pulse"></span>
          <span class="w-10 h-3 rounded bg-surface1 animate-pulse"></span>
          <span class="h-3 flex-1 rounded bg-surface1 animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else if loadError}
    <!-- Distinct from empty-slots: the load failed, and we want the
         user to know they can retry without the rest of the dashboard
         shouting a toast on every background reload. -->
    <div class="text-xs text-dim space-y-1.5">
      <p>Couldn't load today's meals.</p>
      <button
        type="button"
        onclick={load}
        class="text-secondary hover:underline"
      >Retry</button>
    </div>
  {:else if !data || data.slots.length === 0}
    <div class="text-sm text-dim italic leading-relaxed">
      no meal slots configured.
    </div>
  {:else}
    {@const pct = data.total === 0 ? 0 : Math.round((data.done / data.total) * 100)}
    <div class="mb-3 h-1.5 bg-surface1 rounded-full overflow-hidden">
      <div
        class="h-full transition-all duration-300 {pct === 100 ? 'bg-success' : 'bg-primary'}"
        style="width: {pct}%"
      ></div>
    </div>

    <ul class="space-y-1.5">
      {#each data.slots as s (slotKey(s))}
        {@const key = slotKey(s)}
        <li class="flex items-center gap-2">
          <!-- Touch target: 24px square on phones (above the 44 HIG
               via the surrounding padding eaten by the row), 16px on
               desktop where the cursor lands precisely. -->
          <button
            onclick={() => toggle(s)}
            disabled={busyKey === key}
            class="w-5 h-5 sm:w-4 sm:h-4 rounded border flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
              {s.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
            aria-label="toggle {s.name}"
            aria-pressed={s.done}
          >
            {#if s.done}
              <svg viewBox="0 0 12 12" class="w-3.5 h-3.5 sm:w-3 sm:h-3 text-mantle"
                ><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z" /></svg
              >
            {/if}
          </button>
          <span
            class="text-[11px] font-mono tabular-nums text-dim flex-shrink-0 w-10"
          >{s.time}</span>
          <span
            class="text-sm flex-shrink-0 {s.done
              ? 'text-dim line-through decoration-dim/40'
              : 'text-text'}"
          >{s.name}</span>
          <input
            type="text"
            placeholder="…"
            value={displayText(s)}
            oninput={(e) => onTextInput(s, (e.currentTarget as HTMLInputElement).value)}
            onblur={(e) => void saveText(s, (e.currentTarget as HTMLInputElement).value)}
            class="flex-1 min-w-0 bg-transparent text-xs text-subtext placeholder-dim focus:outline-none focus:text-text"
          />
        </li>
      {/each}
    </ul>
  {/if}
</section>
