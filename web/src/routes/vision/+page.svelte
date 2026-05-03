<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /vision is the user's "above goals" layer — life mission, core
  // values, current season focus. The page is intentionally calm:
  // big serif typography for the read view, single-column edit form,
  // no chrome. The point is for the user to come here and re-read,
  // not poke at controls.

  let vision = $state<Vision | null>(null);
  let loading = $state(false);

  // Edit-mode state. The page renders read view by default and
  // flips to a form when the user clicks "edit" — staying in
  // read mode by default keeps the page feeling like a poster
  // rather than a dashboard.
  let editing = $state(false);
  let form = $state({
    mission: '',
    valuesText: '', // newline- or comma-separated, parsed on submit
    season_focus: '',
    notes: ''
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      vision = await api.getVision();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/vision.json') load();
    });
  });

  function startEdit() {
    if (!vision) return;
    form = {
      mission: vision.mission ?? '',
      // Show one value per line — easier to scan + reorder than a
      // single comma-separated input.
      valuesText: (vision.values ?? []).join('\n'),
      season_focus: vision.season_focus ?? '',
      notes: vision.notes ?? ''
    };
    editing = true;
  }
  function cancelEdit() {
    editing = false;
  }
  async function saveEdit() {
    // Parse values: split on newline OR comma, trim, drop empties.
    // Both are common ways users write a short list and we don't
    // need to be strict.
    const values = form.valuesText
      .split(/[\n,]+/)
      .map((v) => v.trim())
      .filter(Boolean);
    try {
      const next = await api.putVision({
        mission: form.mission.trim(),
        values,
        season_focus: form.season_focus.trim(),
        notes: form.notes.trim(),
        // Don't supply season_started_at — server stamps it when
        // the focus changes from prev. Sending an empty string
        // would force-clear it, which is the wrong default.
      });
      vision = next;
      editing = false;
      toast.success('vision saved');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Helper: renders the season-day pill text. Returns empty when
  // the season hasn't started — the pill simply doesn't render.
  function seasonPill(v: Vision): string {
    if (!v.season_day || !v.season_total) return '';
    const remaining = v.season_total - v.season_day;
    if (remaining === 0) return `Day ${v.season_day} of ${v.season_total} — last day`;
    return `Day ${v.season_day} of ${v.season_total} · ${remaining} days left`;
  }

  let isEmpty = $derived(
    !vision ||
      ((!vision.mission || vision.mission === '') &&
        !vision.season_focus &&
        (!vision.values || vision.values.length === 0))
  );
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-2xl mx-auto p-6 sm:p-10 lg:p-14">
    <PageHeader title="Vision" subtitle="Life mission, core values, season focus — the layer above goals" />

    {#if loading && !vision}
      <p class="text-sm text-dim">loading…</p>
    {:else if isEmpty && !editing}
      <!-- First-time / empty state. Single CTA. The copy is the
           page's invitation to actually do the exercise — most
           users won't have done this before, and a pile of empty
           form fields is the wrong invite. -->
      <div class="bg-surface0 border border-surface1 rounded-lg p-8 text-center">
        <p class="text-base text-text">No vision set yet.</p>
        <p class="text-sm text-dim mt-2 max-w-md mx-auto">
          One sentence about why you're here. Three to five words for what you stand for. One phrase for what this season is about.
          You'll re-read it every morning before drilling into tasks.
        </p>
        <button
          onclick={startEdit}
          class="mt-5 px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >Set your vision →</button>
      </div>
    {:else if editing}
      <!-- Edit form. Generous spacing, big inputs — discourages
           treating this like a quick form-fill. -->
      <form onsubmit={(e) => { e.preventDefault(); saveEdit(); }} class="space-y-6">
        <section>
          <label for="mission" class="block text-xs uppercase tracking-wider text-dim mb-2">Life mission</label>
          <textarea
            id="mission"
            bind:value={form.mission}
            rows="2"
            placeholder="One sentence about why you're here."
            class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-base text-text placeholder-dim focus:outline-none focus:border-primary resize-y font-serif"
          ></textarea>
        </section>

        <section>
          <label for="values" class="block text-xs uppercase tracking-wider text-dim mb-2">Core values</label>
          <textarea
            id="values"
            bind:value={form.valuesText}
            rows="5"
            placeholder="One per line, e.g.&#10;Faith&#10;Family&#10;Craft&#10;Honesty"
            class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y font-mono"
          ></textarea>
          <p class="text-[11px] text-dim mt-1">3-5 words or short phrases. Newlines or commas — both work.</p>
        </section>

        <section>
          <label for="season" class="block text-xs uppercase tracking-wider text-dim mb-2">Season focus</label>
          <input
            id="season"
            bind:value={form.season_focus}
            placeholder="One phrase for the next 90 days."
            class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-base text-text placeholder-dim focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1">
            {#if vision?.season_started_at && vision.season_focus === form.season_focus}
              Started {vision.season_started_at} · changing this resets the day counter.
            {:else}
              Changing this stamps today as day 1 of the new 90-day season.
            {/if}
          </p>
        </section>

        <section>
          <label for="notes" class="block text-xs uppercase tracking-wider text-dim mb-2">Notes</label>
          <textarea
            id="notes"
            bind:value={form.notes}
            rows="3"
            placeholder="Optional context — why these values, why this season, what triggered the change…"
            class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
          ></textarea>
        </section>

        <div class="flex gap-2 justify-end pt-2">
          <button type="button" onclick={cancelEdit} class="text-sm px-4 py-2 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
          <button type="submit" class="text-sm px-4 py-2 rounded bg-primary text-on-primary font-medium hover:opacity-90">Save vision</button>
        </div>
      </form>
    {:else if vision}
      <!-- Read view. Big serif typography, generous spacing — meant
           to be re-read, not skimmed past. -->
      <article class="space-y-10">
        {#if vision.mission}
          <section>
            <p class="text-xs uppercase tracking-wider text-dim mb-2">Mission</p>
            <p class="text-xl sm:text-2xl text-text leading-relaxed font-serif italic">
              {vision.mission}
            </p>
          </section>
        {/if}

        {#if vision.values && vision.values.length > 0}
          <section>
            <p class="text-xs uppercase tracking-wider text-dim mb-3">Values</p>
            <ul class="flex flex-wrap gap-2">
              {#each vision.values as v}
                <li class="px-3 py-1.5 bg-surface0 border border-surface1 rounded-full text-sm text-text font-medium">{v}</li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if vision.season_focus}
          <section>
            <p class="text-xs uppercase tracking-wider text-dim mb-2">This season</p>
            <p class="text-lg sm:text-xl text-text leading-relaxed font-serif">
              {vision.season_focus}
            </p>
            {#if seasonPill(vision)}
              <p class="text-[11px] text-dim mt-2">{seasonPill(vision)}</p>
              <!-- Visual progress bar mirroring the day-counter so
                   the season's runway feels concrete. -->
              {#if vision.season_total}
                {@const pct = Math.min(100, Math.round(((vision.season_day ?? 0) / vision.season_total) * 100))}
                <div class="h-1 mt-1.5 bg-mantle rounded-full overflow-hidden max-w-md">
                  <div class="h-full bg-primary transition-all" style="width: {pct}%"></div>
                </div>
              {/if}
            {/if}
          </section>
        {/if}

        {#if vision.notes}
          <section>
            <p class="text-xs uppercase tracking-wider text-dim mb-2">Notes</p>
            <p class="text-sm text-subtext leading-relaxed whitespace-pre-line">{vision.notes}</p>
          </section>
        {/if}

        <div class="pt-4 border-t border-surface1 flex items-center justify-between">
          <button onclick={startEdit} class="text-xs text-dim hover:text-text">edit</button>
          {#if vision.updated_at}
            <span class="text-[11px] text-dim">last updated {new Date(vision.updated_at).toLocaleDateString()}</span>
          {/if}
        </div>
      </article>
    {/if}

    <p class="text-[11px] text-dim italic mt-10">
      Synced via <code>.granit/vision.json</code> — same file the granit TUI reads.
    </p>
  </div>
</div>
