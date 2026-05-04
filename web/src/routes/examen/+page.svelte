<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, todayISO, type Note, type PrayerIntention } from '$lib/api';
  import { toast } from '$lib/components/toast';

  // /examen — evening companion to /morning. Two-question Ignatian
  // rhythm (where did I see God? where did I miss?) plus optional
  // gratitude + tomorrow-prep fields. Saves a `## Examen` block to
  // today's daily note via POST /api/v1/examen.
  //
  // Design choices, mirroring /morning:
  //   - Auto-persists every keystroke to localStorage (granit.examen.YYYY-MM-DD)
  //     so a closed tab / phone lock doesn't lose the in-progress entry.
  //   - The save endpoint upserts in place — re-running the wizard later
  //     in the evening replaces the previous version rather than appending.
  //   - Reads context from today's daily note + active prayer intentions
  //     so the user enters the reflection grounded in what they committed
  //     to in the morning, not from a blank screen.

  // Today buffer keys per-day so yesterday's half-finished entry doesn't
  // bleed into today's. Date is YYYY-MM-DD in the user's local zone.
  const today = todayISO();
  const STORAGE_KEY = `granit.examen.${today}`;

  // ----- Form state -----
  let sawGod = $state('');
  let missed = $state('');
  let gratitude = $state('');
  let tomorrow = $state('');
  let saving = $state(false);
  let error = $state('');
  let saved = $state(false);

  // ----- Context from today -----
  // Today's daily-note body (we extract Daily Plan + active prayer
  // intentions to anchor the user before they reflect). Optional —
  // a missing daily note shouldn't block the wizard.
  let dailyNote = $state<Note | null>(null);
  let dailyLoaded = $state(false);
  let activeIntentions = $state<PrayerIntention[]>([]);

  // Snapshot helpers — mirrors /morning's persistence pattern. The
  // payload is small enough that a per-keystroke write is fine.
  interface Snapshot {
    sawGod: string;
    missed: string;
    gratitude: string;
    tomorrow: string;
  }
  function snapshot(): Snapshot {
    return { sawGod, missed, gratitude, tomorrow };
  }
  function persist() {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot())); } catch {}
  }
  function restore() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const s = JSON.parse(raw) as Partial<Snapshot>;
      sawGod = s.sawGod ?? '';
      missed = s.missed ?? '';
      gratitude = s.gratitude ?? '';
      tomorrow = s.tomorrow ?? '';
    } catch {}
  }
  function clearPersisted() {
    try { localStorage.removeItem(STORAGE_KEY); } catch {}
  }
  $effect(() => {
    void sawGod; void missed; void gratitude; void tomorrow;
    persist();
  });

  async function loadContext() {
    if (!$auth) return;
    // Today's daily note + active prayer intentions in parallel.
    // Both are optional context — failures (404 on daily, prayer
    // module disabled) shouldn't block the wizard.
    const [d, p] = await Promise.allSettled([
      api.daily('today'),
      api.listPrayer()
    ]);
    if (d.status === 'fulfilled') dailyNote = d.value;
    if (p.status === 'fulfilled') {
      activeIntentions = p.value.intentions.filter((x) => x.status === 'praying');
    }
    dailyLoaded = true;
  }

  onMount(() => {
    restore();
    loadContext();
  });

  // ----- Daily Plan extraction for the context strip -----
  // Pull the `## Daily Plan` section from the note body so the user
  // sees what they committed to this morning while writing the
  // evening reflection. We render it read-only — no editing on this
  // page (the user can always click into the daily note itself).
  let dailyPlanText = $derived.by(() => {
    if (!dailyNote?.body) return '';
    const body = dailyNote.body;
    const idx = body.indexOf('## Daily Plan');
    if (idx === -1) return '';
    // Section ends at next H2 (line starting with '## ' but not '### ').
    const rest = body.slice(idx);
    const lines = rest.split('\n');
    let endLine = lines.length;
    for (let i = 1; i < lines.length; i++) {
      if (lines[i].startsWith('## ') && !lines[i].startsWith('### ')) {
        endLine = i;
        break;
      }
    }
    return lines.slice(0, endLine).join('\n').trim();
  });

  // Did the user already write an examen earlier this evening? We
  // detect by looking for "## Examen" in the daily note body — the
  // server upserts in place, so the user can re-open the wizard and
  // edit; we just want to surface "you've already done this once
  // today" so they aren't surprised when their save replaces.
  let alreadyExamenedToday = $derived.by(() => {
    if (!dailyNote?.body) return false;
    return dailyNote.body.includes('## Examen');
  });

  // ----- Save -----
  async function save() {
    saving = true;
    error = '';
    try {
      await api.saveExamen({
        saw_god: sawGod.trim() || undefined,
        missed: missed.trim() || undefined,
        gratitude: gratitude.trim() || undefined,
        tomorrow: tomorrow.trim() || undefined
      });
      clearPersisted();
      saved = true;
      toast.success('examen saved');
      // Brief delay so the user sees the success state before the
      // route changes — "saved" feels nicer than an instant nav away.
      setTimeout(() => goto('/'), 400);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      toast.error(`save failed: ${error}`);
    } finally {
      saving = false;
    }
  }

  // Disable save until at least one field has content. A wholly-empty
  // submission would still write a "(no entries this evening)" stub,
  // but that's not what the user wants from a button click.
  let canSave = $derived(
    sawGod.trim().length > 0 ||
      missed.trim().length > 0 ||
      gratitude.trim().length > 0 ||
      tomorrow.trim().length > 0
  );
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-2xl mx-auto">
    <header class="mb-5 sm:mb-6">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Daily examen</h1>
      <p class="text-sm text-dim mt-1">
        Close the day before God. Where did you see Him? Where did you miss?
        {#if alreadyExamenedToday}
          <span class="text-secondary">· already entered today — saving will update.</span>
        {/if}
      </p>
    </header>

    <!-- Context strip: today's daily plan + intentions you carried -->
    {#if dailyLoaded && (dailyPlanText || activeIntentions.length > 0)}
      <section class="mb-6 bg-surface0 border-l-2 border-primary rounded-r-lg p-4 space-y-3 text-sm">
        {#if dailyPlanText}
          <details class="text-subtext" open>
            <summary class="cursor-pointer text-xs uppercase tracking-wider text-dim hover:text-text font-medium">
              Today's plan
            </summary>
            <pre class="text-xs mt-2 whitespace-pre-wrap font-sans text-text/90 leading-relaxed">{dailyPlanText}</pre>
          </details>
        {/if}
        {#if activeIntentions.length > 0}
          <details class="text-subtext">
            <summary class="cursor-pointer text-xs uppercase tracking-wider text-dim hover:text-text font-medium">
              Currently praying for · {activeIntentions.length}
            </summary>
            <ul class="mt-2 space-y-1 text-xs">
              {#each activeIntentions.slice(0, 6) as p (p.id)}
                <li class="text-text/90">
                  · {p.text}
                  {#if p.venture}<span class="text-secondary"> 🏢 {p.venture}</span>{/if}
                </li>
              {/each}
              {#if activeIntentions.length > 6}
                <li class="text-dim italic">+ {activeIntentions.length - 6} more</li>
              {/if}
            </ul>
          </details>
        {/if}
      </section>
    {/if}

    {#if error}
      <div class="text-sm text-error mb-4 p-3 bg-error/10 border border-error/30 rounded">{error}</div>
    {/if}

    <form
      onsubmit={(e) => { e.preventDefault(); save(); }}
      class="space-y-5"
    >
      <!-- Where did I see God? -->
      <section>
        <label for="examen-saw" class="block text-sm font-medium text-text mb-1.5">
          Where did I see God today?
          <span class="text-[11px] text-dim font-normal ml-1">consolation, grace, the unexpected gift</span>
        </label>
        <textarea
          id="examen-saw"
          bind:value={sawGod}
          rows="4"
          placeholder="A conversation. A breakthrough. A moment of peace. Where He showed up."
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        ></textarea>
      </section>

      <!-- Where did I miss? -->
      <section>
        <label for="examen-miss" class="block text-sm font-medium text-text mb-1.5">
          Where did I miss?
          <span class="text-[11px] text-dim font-normal ml-1">desolation, distraction, where I resisted grace</span>
        </label>
        <textarea
          id="examen-miss"
          bind:value={missed}
          rows="4"
          placeholder="Honest, not punishing. What pulled me away today?"
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        ></textarea>
      </section>

      <!-- Optional: gratitude -->
      <section>
        <label for="examen-grat" class="block text-sm font-medium text-text mb-1.5">
          Gratitude
          <span class="text-[11px] text-dim font-normal ml-1">optional — three things, or none</span>
        </label>
        <textarea
          id="examen-grat"
          bind:value={gratitude}
          rows="3"
          placeholder=""
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        ></textarea>
      </section>

      <!-- Optional: tomorrow -->
      <section>
        <label for="examen-tomorrow" class="block text-sm font-medium text-text mb-1.5">
          For tomorrow
          <span class="text-[11px] text-dim font-normal ml-1">optional — what I bring before God for the morning</span>
        </label>
        <textarea
          id="examen-tomorrow"
          bind:value={tomorrow}
          rows="3"
          placeholder="A specific ask. A virtue to practice. The one thing I want to bring."
          class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        ></textarea>
      </section>

      <div class="flex items-center gap-3 pt-2 border-t border-surface1">
        <span class="text-[11px] text-dim flex-1">
          Saves under <code class="text-[10px]">## Examen</code> in today's daily note.
          {#if saved}<span class="text-success ml-1">✓ saved</span>{/if}
        </span>
        <button
          type="button"
          onclick={() => goto('/')}
          class="px-3 py-1.5 text-sm text-subtext hover:text-text"
        >Cancel</button>
        <button
          type="submit"
          disabled={!canSave || saving}
          class="px-4 py-2 bg-primary text-on-primary rounded font-medium text-sm disabled:opacity-50"
        >{saving ? 'saving…' : 'Close the day'}</button>
      </div>
    </form>
  </div>
</div>
