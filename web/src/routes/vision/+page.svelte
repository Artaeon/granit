<script lang="ts">
  // /vision — the user's "above goals" anchor in the Rhythmus-OS
  // shape. Five identity statements (one per daily pillar) replace
  // the older mission / values / season_focus trio. The page reads
  // like a poster you re-read every morning, not a dashboard.
  //
  // Migration: a legacy vision.json (mission + values + season_focus)
  // still parses; the page detects that and offers a one-click
  // "Vorschlag aus alten Daten" button that pre-fills the identity
  // form from those fields. The legacy values stay on disk until
  // the user actually saves the new shape — that way they can roll
  // back by editing vision.json manually.
  //
  // The five pillars (spirit / food / work / body / evening) are
  // hard-coded — the discipline of the rhythm is that there are
  // five. Labels can be renamed in /rhythmus; the keys are stable.

  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { DEFAULT_PILLARS, PILLAR_ORDER, type PillarKey } from '$lib/rhythmus/pillars';
  import { rhythmusConfig, pillarLabel } from '$lib/rhythmus/minima';

  let vision = $state<Vision | null>(null);
  let loading = $state(false);
  let editing = $state(false);

  // Edit-form state mirrors the on-disk shape: one identity per
  // pillar plus the free-text notes block. Initialised on edit so
  // a cancel doesn't drop the user's mid-flow edits visibly
  // (they're still discarded — just not on the screen).
  type IdentitiesForm = Record<PillarKey, string>;
  function emptyIdentitiesForm(): IdentitiesForm {
    return { spirit: '', food: '', work: '', body: '', evening: '' };
  }
  let form = $state<{ identities: IdentitiesForm; notes: string }>({
    identities: emptyIdentitiesForm(),
    notes: ''
  });

  let cfg = $derived($rhythmusConfig);

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
    if (!vision) {
      form = { identities: emptyIdentitiesForm(), notes: '' };
      editing = true;
      return;
    }
    const next = emptyIdentitiesForm();
    if (vision.identities) {
      for (const key of PILLAR_ORDER) {
        const v = vision.identities[key];
        if (typeof v === 'string') next[key] = v;
      }
    }
    form = { identities: next, notes: vision.notes ?? '' };
    editing = true;
  }

  function cancelEdit() {
    editing = false;
  }

  async function saveEdit() {
    // Build identities map. Empty strings are fine — the server
    // round-trips them; the read view simply doesn't render an
    // empty identity row, so a half-filled vision still looks
    // intentional.
    const identities: Record<string, string> = {};
    for (const key of PILLAR_ORDER) {
      const trimmed = form.identities[key].trim();
      if (trimmed) identities[key] = trimmed;
    }
    try {
      const next = await api.putVision({
        identities,
        notes: form.notes.trim()
      });
      vision = next;
      editing = false;
      toast.success('vision saved');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Migration helper. Reads the legacy fields (mission / values /
  // season_focus) and pre-fills identity slots from them — the
  // user reviews + commits or discards. Heuristic: mission → work
  // pillar (people most often phrase missions in vocational terms);
  // first three values fan out to the remaining pillars in pillar
  // order; season_focus folds into work too if mission was empty.
  // Deliberately rough — the point is to give the user a starting
  // shape, not a finished translation.
  function suggestFromLegacy() {
    if (!vision) return;
    const next = { ...form.identities };
    const remaining: PillarKey[] = ['spirit', 'body', 'evening', 'food'];
    if (vision.mission && !next.work) next.work = vision.mission;
    else if (vision.season_focus && !next.work) next.work = vision.season_focus;
    const values = vision.values ?? [];
    for (let i = 0; i < remaining.length && i < values.length; i++) {
      const k = remaining[i];
      if (!next[k]) next[k] = values[i];
    }
    form = { ...form, identities: next };
  }

  let hasLegacyData = $derived(
    !!vision &&
      ((vision.mission && vision.mission.trim()) ||
        (vision.values && vision.values.length > 0) ||
        (vision.season_focus && vision.season_focus.trim()))
  );

  let hasNewData = $derived(
    !!vision &&
      vision.identities &&
      Object.values(vision.identities).some((s) => typeof s === 'string' && s.trim())
  );

  let isEmpty = $derived(!hasNewData && !hasLegacyData);

  // ── AI: harden the vision ────────────────────────────────────────
  // Same shape as the previous /vision: stream a critique + sharper
  // alternatives. The context block now leads with identities, but
  // legacy mission/values come along when present so the AI sees the
  // user's whole stated picture, not the half it's been migrated to.
  let aiBusy = $state(false);
  let aiResponse = $state('');
  let aiError = $state('');
  let aiAbort: AbortController | null = null;

  async function hardenVision() {
    if (!vision || aiBusy) return;
    aiBusy = true;
    aiError = '';
    aiResponse = '';
    aiAbort = new AbortController();
    const lines: string[] = [];
    if (vision.identities) {
      lines.push('Identities (one per pillar):');
      for (const key of PILLAR_ORDER) {
        const label = pillarLabel(cfg, key);
        const v = vision.identities[key];
        if (v) lines.push(`  ${label}: ${v}`);
      }
    }
    if (vision.mission) lines.push(`Legacy mission: ${vision.mission}`);
    if (vision.values && vision.values.length > 0) {
      lines.push(`Legacy values: ${vision.values.join(', ')}`);
    }
    if (vision.season_focus) lines.push(`Legacy season focus: ${vision.season_focus}`);
    if (vision.notes) lines.push(`Notes: ${vision.notes}`);
    const ctx = lines.join('\n');
    const userMessage =
      "Critique and sharpen this user's identity-based life vision. " +
      'They re-read this every morning before drilling into tasks, so the language has to be concrete enough to actually steer behaviour. ' +
      'Format your reply with three sections:\n\n' +
      "## Where it's vague\n" +
      'Point out lines that could mean anything. Be specific — quote the phrase.\n\n' +
      '## Sharpened versions\n' +
      'Rewrite the weakest 1-2 lines into versions a stranger could act on without further interpretation. ' +
      'Show the BEFORE in italic and the AFTER in bold so the user can compare.\n\n' +
      '## Questions to sit with\n' +
      "2-3 questions whose honest answers would make the next iteration of this vision better. Don't preach; ask.\n\n" +
      'Vision context:\n\n' + ctx;
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { aiResponse += c; },
          onError: (err) => { aiError = err.message; }
        },
        aiAbort.signal
      );
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
  }
  function cancelAI() { aiAbort?.abort(); }

  function identityFor(v: Vision, key: PillarKey): string | undefined {
    const s = v.identities?.[key];
    return typeof s === 'string' && s.trim() ? s : undefined;
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-2xl mx-auto p-6 sm:p-10 lg:p-14">
    <PageHeader
      title="Vision"
      subtitle="Five identity statements — one per daily pillar"
    />

    {#if loading && !vision}
      <p class="text-sm text-dim">loading…</p>
    {:else if isEmpty && !editing}
      <div class="bg-surface0 border border-surface1 rounded-lg p-8 text-center">
        <p class="text-base text-text">Keine Identity-Statements gesetzt.</p>
        <p class="text-sm text-dim mt-2 max-w-md mx-auto">
          Eine Zeile pro Säule: <em>wer du bist</em> in diesem Bereich, nicht <em>was du erreichen willst</em>.
          „Ich suche Gott täglich" statt „10 kg Muskeln". Du liest das jeden Morgen.
        </p>
        <button
          onclick={startEdit}
          class="mt-5 px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >Identity setzen →</button>
      </div>
    {:else if editing}
      <form onsubmit={(e) => { e.preventDefault(); saveEdit(); }} class="space-y-5">
        {#if hasLegacyData}
          <!-- Migration helper. Visible only when legacy data exists
               AND the user hasn't yet filled identities — saving
               clears the banner because hasLegacyData stays true
               (legacy stays on disk) but the user has already
               handled it. -->
          <div class="bg-surface0 border border-surface1 rounded p-3 flex items-baseline gap-3 flex-wrap">
            <div class="flex-1 min-w-0 text-xs text-subtext">
              Du hast Mission / Values / Season-Focus von früher. Vorschlag aus den alten Daten als Startpunkt füllen?
            </div>
            <button
              type="button"
              onclick={suggestFromLegacy}
              class="text-[11px] px-2 py-1 rounded bg-surface1 border border-surface2 text-primary hover:border-primary"
            >Vorschlag einfüllen</button>
          </div>
        {/if}

        {#each PILLAR_ORDER as key (key)}
          {@const label = pillarLabel(cfg, key)}
          {@const icon = DEFAULT_PILLARS[key].icon}
          <section>
            <label for="id-{key}" class="block text-xs uppercase tracking-wider text-dim mb-2">
              <span class="mr-1" aria-hidden="true">{icon}</span>
              {label}
            </label>
            <input
              id="id-{key}"
              bind:value={form.identities[key]}
              placeholder="Ich …"
              class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-base text-text placeholder-dim focus:outline-none focus:border-primary font-serif"
            />
          </section>
        {/each}

        <section>
          <label for="notes" class="block text-xs uppercase tracking-wider text-dim mb-2">Notes</label>
          <textarea
            id="notes"
            bind:value={form.notes}
            rows="3"
            placeholder="Optionaler Kontext — warum diese Identities, was sich geändert hat …"
            class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
          ></textarea>
        </section>

        <div class="flex gap-2 justify-end pt-2">
          <button
            type="button"
            onclick={cancelEdit}
            class="text-sm px-4 py-2 rounded bg-surface0 text-subtext hover:bg-surface1"
          >Cancel</button>
          <button
            type="submit"
            class="text-sm px-4 py-2 rounded bg-primary text-on-primary font-medium hover:opacity-90"
          >Save vision</button>
        </div>
      </form>
    {:else if vision}
      <!-- Read view. One identity row per pillar that has content;
           empty pillars hide so a partial vision looks intentional
           rather than half-filled. -->
      <article class="space-y-8">
        {#each PILLAR_ORDER as key (key)}
          {@const text = identityFor(vision, key)}
          {#if text}
            {@const label = pillarLabel(cfg, key)}
            {@const icon = DEFAULT_PILLARS[key].icon}
            <section>
              <p class="text-xs uppercase tracking-wider text-dim mb-2 flex items-center gap-2">
                <span aria-hidden="true">{icon}</span>
                {label}
              </p>
              <p class="text-xl sm:text-2xl text-text leading-relaxed font-serif italic">
                {text}
              </p>
            </section>
          {/if}
        {/each}

        {#if hasLegacyData && !hasNewData}
          <!-- Legacy display fallback: the user hasn't migrated yet.
               Show the old data read-only with a hint so the page
               isn't blank for a long-time user who lands here for
               the first time after the pivot. -->
          <section class="bg-surface0 border border-surface1 rounded p-4 space-y-3">
            <p class="text-[11px] uppercase tracking-wider text-dim">Aus früherer Version</p>
            {#if vision.mission}
              <p class="text-base text-text font-serif italic">{vision.mission}</p>
            {/if}
            {#if vision.values && vision.values.length > 0}
              <ul class="flex flex-wrap gap-1.5">
                {#each vision.values as v}
                  <li class="px-2 py-0.5 bg-mantle border border-surface1 rounded-full text-xs text-subtext">{v}</li>
                {/each}
              </ul>
            {/if}
            {#if vision.season_focus}
              <p class="text-sm text-subtext">{vision.season_focus}</p>
            {/if}
            <button
              type="button"
              onclick={startEdit}
              class="text-xs text-primary hover:underline"
            >→ Identity-Statements daraus ableiten</button>
          </section>
        {/if}

        {#if vision.notes}
          <section>
            <p class="text-xs uppercase tracking-wider text-dim mb-2">Notes</p>
            <p class="text-sm text-subtext leading-relaxed whitespace-pre-line">{vision.notes}</p>
          </section>
        {/if}

        <section class="pt-4 border-t border-surface1">
          <div class="flex items-baseline gap-2 mb-2">
            <h2 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">AI · Harden this vision</h2>
            {#if aiBusy}
              <button onclick={cancelAI} class="text-[11px] text-warning hover:underline">cancel</button>
            {:else if aiResponse}
              <button onclick={() => { aiResponse = ''; aiError = ''; }} class="text-[11px] text-dim hover:text-error">clear</button>
            {/if}
            <button
              onclick={() => void hardenVision()}
              disabled={aiBusy || isEmpty}
              class="text-[11px] px-2 py-1 rounded bg-surface1 border border-surface2 text-primary hover:border-primary disabled:opacity-50"
              title="Ask the AI to critique your identities and suggest sharper alternatives"
            >{aiBusy ? '✨ thinking…' : aiResponse ? '✨ regenerate' : '✨ Harden'}</button>
          </div>
          {#if aiError}
            <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2">{aiError}</div>
          {:else if aiResponse || aiBusy}
            <div class="bg-surface0 border border-surface1 rounded-lg px-4 py-3 text-sm text-text">
              <div class="prose prose-sm max-w-none">
                <MarkdownRenderer body={aiResponse || '_…_'} />
              </div>
            </div>
            <p class="text-[10px] text-dim italic mt-2">
              Suggestions from your configured AI. Take what sharpens, ignore what doesn't.
            </p>
          {:else}
            <p class="text-xs text-dim leading-relaxed">
              Re-read it every morning, sharpen it every season. Tap Harden to get a critique
              of vague phrasing + sharpened alternatives + questions to sit with.
            </p>
          {/if}
        </section>

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
