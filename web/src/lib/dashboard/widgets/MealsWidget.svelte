<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type MealsResponse, type MealSlot } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { createCoalescedReload } from '$lib/util/coalesce';

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
  // Set of slot keys currently mid-PATCH. We use a set (not a single
  // key) so the user can tick multiple slots in fast succession — the
  // reconciling load() only fires when the last in-flight patch
  // resolves, otherwise an early reload could snap a still-pending
  // optimistic flip back to the pre-patch server state.
  let busyKeys = $state(new Set<string>());
  // Debounced text-input state — local per-row buffer keyed by
  // (time|name) so a fast typist doesn't fire a PATCH per keystroke.
  let drafts = $state<Record<string, string>>({});
  let saveTimers: Record<string, ReturnType<typeof setTimeout>> = {};

  function addBusy(key: string) {
    busyKeys.add(key);
    busyKeys = new Set(busyKeys);
  }
  function removeBusy(key: string) {
    busyKeys.delete(key);
    busyKeys = new Set(busyKeys);
  }

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

  // Schedule a reload at next midnight so the widget rolls over to
  // the new day without the user having to refresh. setTimeout is
  // armed for the exact remaining ms until 00:00:05 local (the 5s
  // pad absorbs clock-drift edge cases where the timer fires
  // microseconds before midnight). After firing, re-arm for the
  // following midnight.
  let midnightTimer: ReturnType<typeof setTimeout> | null = null;
  function armMidnight() {
    if (midnightTimer) clearTimeout(midnightTimer);
    const now = new Date();
    const next = new Date(now);
    next.setHours(24, 0, 5, 0);
    midnightTimer = setTimeout(() => {
      void load();
      armMidnight();
    }, next.getTime() - now.getTime());
  }

  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    load();
    armMidnight();
    const unsub = onWsEvent((ev) => {
      // Today's daily note touched anywhere reloads us. Filter by
      // path prefix would require knowing the daily folder name —
      // for now any note.changed triggers reload; cheap enough.
      // But skip while a local patch is in flight: that patch's
      // own reload will catch the server state without trampling
      // pending optimistic flips for other slots the user is
      // ticking in parallel.
      if (ev.type !== 'note.changed') return;
      if (busyKeys.size > 0) return;
      reload.trigger();
    });
    return () => {
      unsub();
      reload.cancel();
      if (midnightTimer) clearTimeout(midnightTimer);
      // Flush every pending debouncer so a navigate-away during the
      // 600ms window doesn't leave dangling timers firing patchMeal
      // on a torn-down component (would no-op but generates a
      // console warning + a wasted network round-trip).
      for (const k of Object.keys(saveTimers)) {
        clearTimeout(saveTimers[k]);
      }
    };
  });

  async function toggle(slot: MealSlot) {
    const key = slotKey(slot);
    if (busyKeys.has(key)) return;
    addBusy(key);
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
    } catch (e) {
      toast.error(`couldn't tick ${slot.name}: ${errorMessage(e)}`);
    } finally {
      removeBusy(key);
    }
    // Reconcile only once the last in-flight patch resolves —
    // otherwise an early load() during a multi-tick burst would
    // overwrite still-pending optimistic flips for sibling slots.
    if (busyKeys.size === 0) {
      await load();
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
    addBusy(key);
    try {
      await api.patchMeal({
        time: slot.time,
        name: slot.name,
        date: data?.date,
        text: trimmed
      });
      // Drop the local draft so any subsequent server-driven reload
      // (e.g. another tab editing the same slot) wins. But ONLY drop
      // it if the user hasn't typed anything new since we started the
      // save — otherwise the next debouncer tick has nothing to save
      // and the user sees their fresh characters vanish from the
      // input. (Found while stress-typing during a slow PATCH.)
      if (drafts[key] === value) {
        delete drafts[key];
      }
    } catch (e) {
      toast.error(`couldn't save ${slot.name} text: ${errorMessage(e)}`);
    } finally {
      removeBusy(key);
    }
    if (busyKeys.size === 0) {
      await load();
    }
  }

  function displayText(slot: MealSlot): string {
    const key = slotKey(slot);
    if (key in drafts) return drafts[key];
    return slot.text ?? '';
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs text-dim font-semibold">Meals</h2>
    <span class="flex-1"></span>
    {#if data && data.total > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">
        {data.done}/{data.total}
      </span>
    {/if}
  </div>

  {#if !loaded}
    <!-- Skeleton dimensions mirror the real row so the dashboard
         doesn't reflow when data arrives. Same w-5/w-4 split as the
         live checkbox below. -->
    <ul class="space-y-1.5" aria-hidden="true">
      {#each Array(3) as _, i (i)}
        <li class="flex items-center gap-2">
          <span class="w-5 h-5 sm:w-4 sm:h-4 rounded bg-surface1 animate-pulse flex-shrink-0"></span>
          <span class="w-10 h-3 rounded bg-surface1 animate-pulse flex-shrink-0"></span>
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
            disabled={busyKeys.has(key)}
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
            onblur={(e) => {
              // Cancel any pending debouncer first — without this,
              // blurring with <600ms since the last keystroke would
              // fire saveText twice (once from blur, once from the
              // pending timer) and produce two PATCHes with the
              // same body.
              if (saveTimers[key]) clearTimeout(saveTimers[key]);
              void saveText(s, (e.currentTarget as HTMLInputElement).value);
            }}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                // Save-and-blur on Enter — cancels the in-flight
                // debouncer so the save fires immediately, and
                // focus leaves so a Tab-after-Enter goes to the
                // next row, not the same input.
                const inp = e.currentTarget as HTMLInputElement;
                if (saveTimers[key]) clearTimeout(saveTimers[key]);
                inp.blur();
              } else if (e.key === 'Escape') {
                // Discard local edits, restore canonical text.
                if (saveTimers[key]) clearTimeout(saveTimers[key]);
                delete drafts[key];
                (e.currentTarget as HTMLInputElement).blur();
              }
            }}
            class="flex-1 min-w-0 bg-transparent text-base sm:text-xs text-subtext placeholder-dim focus:outline-none focus:text-text"
          />
        </li>
      {/each}
    </ul>
  {/if}
</section>
