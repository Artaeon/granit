<script lang="ts">
  // /plans/week — Sunday weekly-planning surface.
  //
  // Flow:
  //   1. User brain-dumps the week into the left pane (markdown).
  //   2. Click "Extract" → AI proposes tasks/milestones, matched
  //      against existing ventures/projects/goals. Proposal renders
  //      in the right pane, grouped by venture.
  //   3. User scans each group, ✓ the items they want, edits any
  //      labels inline, ✕ the ones they don't.
  //   4. Click "Commit". Backend creates the tasks inside the canonical
  //      plan note (Plans/<weekISO>.md) under "### <Venture>" sections,
  //      stamps the plan note's frontmatter with task IDs, returns
  //      the result.
  //
  // v1 scope:
  //   - Current ISO week only (no week navigation)
  //   - Append-only re-extraction (re-runs propose additional items;
  //     existing committed tasks aren't touched)
  //   - Required review (every proposal needs explicit ✓)
  //   - Grouped-by-venture with per-group select-all
  //
  // What's deliberately NOT here (yet):
  //   - Week navigation (prev/next ISO week)
  //   - Diff-aware re-extraction (handles edits to lines you committed)
  //   - Friday/Sunday review overlay (what landed vs slipped)

  import { onMount } from 'svelte';
  import {
    api,
    type PlanExtractedItem,
    type PlanExtractionResponse,
    type PlanCommitItem,
    type PlanCommitResponse
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { isoWeekString, planNotePath } from '$lib/util/isoWeek';

  const weekISO = isoWeekString();
  const planPath = planNotePath();

  // ── Editor state ────────────────────────────────────────────────
  let planText = $state('');
  let loadingNote = $state(true);
  let noteLoadError = $state('');

  onMount(async () => {
    // Pull the existing plan note (or leave empty if none yet). Lets
    // the user open the page on a Tuesday and pick up where Sunday's
    // plan left off without retyping. We read just the body — the
    // commit handler owns frontmatter writes, so re-rendering it here
    // would just create reconciliation friction.
    try {
      const r = await api.getNote(planPath);
      planText = stripPlanScaffolding(r.body ?? '');
    } catch {
      // Not found is the expected first-visit case — empty editor.
      planText = '';
    } finally {
      loadingNote = false;
    }
  });

  // The commit handler scaffolds the plan note with `## Plan` and
  // `## Commitments` headers around the user's brain-dump. When
  // re-opening, strip the scaffolding so the user edits just the body
  // they wrote — without this they'd see the structural headings in
  // their editor and have to delete them every visit.
  function stripPlanScaffolding(body: string): string {
    const lines = body.split('\n');
    let start = -1;
    let end = lines.length;
    for (let i = 0; i < lines.length; i++) {
      if (lines[i].trim() === '## Plan') {
        start = i + 1;
        break;
      }
    }
    if (start === -1) return body;
    for (let i = start; i < lines.length; i++) {
      if (lines[i].trim() === '## Commitments') {
        end = i;
        break;
      }
    }
    return lines.slice(start, end).join('\n').trim();
  }

  // ── Extraction state ────────────────────────────────────────────
  let extracting = $state(false);
  let extractError = $state('');
  let proposal = $state<PlanExtractionResponse | null>(null);

  // Per-item acceptance state — keyed by stable index so the user's
  // ✓/✕ + inline edits survive re-renders. Each item has:
  //   accepted: boolean
  //   editLabel: string  (live-edit; only used if accepted)
  //   editVenture / editProject / editDue: optional override
  type ItemEdit = {
    accepted: boolean;
    label: string;
    venture: string;
    project: string;
    goalId: string;
    dueDate: string;
  };
  let edits = $state<Record<number, ItemEdit>>({});

  async function runExtract() {
    if (!planText.trim()) {
      extractError = 'Write something first — the plan is empty.';
      return;
    }
    extracting = true;
    extractError = '';
    proposal = null;
    edits = {};
    try {
      proposal = await api.extractPlan({ plan_text: planText, week_iso: weekISO });
      // Default-accept items the AI was confident about (exact match
      // or fuzzy ≥80). The user can flip them off; the default just
      // saves clicks on the common case where most items are right.
      for (let i = 0; i < proposal.items.length; i++) {
        const it = proposal.items[i];
        const conf = it.match_confidence ?? 0;
        const defaultAccept =
          it.match_type === 'exact' || (it.match_type === 'fuzzy' && conf >= 80);
        edits[i] = {
          accepted: defaultAccept,
          label: it.label,
          venture: it.venture_name ?? '',
          project: it.project_name ?? '',
          goalId: it.goal_id ?? '',
          dueDate: it.due_date ?? ''
        };
      }
    } catch (e) {
      extractError = errorMessage(e);
    } finally {
      extracting = false;
    }
  }

  // Group by venture for the review UI. Personal items (no venture)
  // go under the empty string and render as "Personal" so the heading
  // structure is uniform.
  type Bucket = { venture: string; idxs: number[] };
  let buckets = $derived.by(() => {
    if (!proposal) return [] as Bucket[];
    const map = new Map<string, number[]>();
    for (let i = 0; i < proposal.items.length; i++) {
      const v = proposal.items[i].venture_name ?? '';
      if (!map.has(v)) map.set(v, []);
      map.get(v)!.push(i);
    }
    return [...map.entries()]
      .map(([venture, idxs]) => ({ venture, idxs }))
      .sort((a, b) => {
        // Personal bucket last; other ventures alphabetical.
        if (a.venture === '' && b.venture !== '') return 1;
        if (b.venture === '' && a.venture !== '') return -1;
        return a.venture.localeCompare(b.venture);
      });
  });

  function bucketLabel(venture: string): string {
    return venture === '' ? 'Personal' : venture;
  }
  function acceptedCount(): number {
    return Object.values(edits).filter((e) => e.accepted).length;
  }
  function setBucket(idxs: number[], accept: boolean) {
    const next = { ...edits };
    for (const i of idxs) {
      if (next[i]) next[i] = { ...next[i], accepted: accept };
    }
    edits = next;
  }
  function bucketAllAccepted(idxs: number[]): boolean {
    return idxs.every((i) => edits[i]?.accepted);
  }

  // Match-type → small badge label + color hint
  function matchBadge(it: PlanExtractedItem): { label: string; color: string } {
    const c = it.match_confidence ?? 0;
    switch (it.match_type) {
      case 'exact':    return { label: 'matched',       color: 'text-green' };
      case 'fuzzy':    return { label: `fuzzy ${c}%`,   color: 'text-yellow' };
      case 'new':      return { label: 'NEW',           color: 'text-mauve' };
      case 'personal': return { label: 'personal',      color: 'text-blue' };
      default:         return { label: it.match_type ?? '?', color: 'text-dim' };
    }
  }

  // ── Commit ──────────────────────────────────────────────────────
  let committing = $state(false);
  let commitResult = $state<PlanCommitResponse | null>(null);
  let commitError = $state('');

  async function runCommit() {
    if (!proposal) return;
    const items: PlanCommitItem[] = [];
    for (let i = 0; i < proposal.items.length; i++) {
      const e = edits[i];
      if (!e?.accepted) continue;
      const orig = proposal.items[i];
      items.push({
        kind: orig.kind,
        label: e.label.trim() || orig.label,
        venture_name: e.venture || undefined,
        project_name: e.project || undefined,
        goal_id: e.goalId || undefined,
        due_date: e.dueDate || undefined,
        source_line: orig.source_line
      });
    }
    if (items.length === 0) {
      commitError = 'Nothing accepted — tick the items you want to commit first.';
      return;
    }
    committing = true;
    commitError = '';
    try {
      commitResult = await api.commitPlan({
        plan_text: planText, week_iso: weekISO, items
      });
      toast.success(`Committed ${commitResult.created_task_ids.length} tasks to ${commitResult.plan_path}`);
      // Clear the proposal so a fresh extract is required for the
      // next batch — append-only re-extraction in v1.
      proposal = null;
      edits = {};
    } catch (e) {
      commitError = errorMessage(e);
    } finally {
      committing = false;
    }
  }
</script>

<svelte:head>
  <title>Weekly plan · {weekISO} · granit</title>
</svelte:head>

<div class="h-full overflow-y-auto bg-mantle">
  <div class="max-w-7xl mx-auto px-4 py-6">
    <header class="mb-6 flex items-baseline gap-3 flex-wrap">
      <h1 class="text-2xl font-semibold text-text">Weekly plan</h1>
      <span class="text-sm text-dim font-mono">{weekISO}</span>
      <span class="text-xs text-dim">· saves to <a href="/notes/{encodeURIComponent(planPath)}" class="text-secondary hover:underline">{planPath}</a></span>
    </header>

    <p class="text-sm text-subtext mb-4 max-w-prose">
      Brain-dump this week in plain language — by venture, by project, by goal, however
      you think. Click <span class="text-text font-medium">Extract</span> and the AI
      proposes tasks (and milestones, when they fit an existing goal), matched against
      your real ventures + projects. You review and commit only what you want.
    </p>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <!-- Left: editor -->
      <section class="bg-base border border-surface1 rounded-lg p-3 flex flex-col">
        <header class="flex items-baseline justify-between mb-2">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Plan</h2>
          <span class="text-[10px] text-dim font-mono">{planText.length} chars</span>
        </header>
        {#if loadingNote}
          <p class="text-sm text-dim italic">loading plan note…</p>
        {:else}
          {#if noteLoadError}
            <p class="text-xs text-error mb-2">{noteLoadError}</p>
          {/if}
          <textarea
            bind:value={planText}
            placeholder={`This week —

<Venture A>: ship feature X, call key customer about the pilot
<Venture B>: send 5 client letters by Wednesday, finish landing page
<Venture C>: get the store live, write product copy
Personal: weekly review Friday, prep Sunday sermon notes
`}
            class="flex-1 min-h-[24rem] w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-[13px] focus:outline-none focus:border-primary resize-y leading-relaxed"
            disabled={extracting || committing}
          ></textarea>
          <div class="mt-3 flex items-center gap-2">
            <button
              type="button"
              onclick={runExtract}
              disabled={extracting || !planText.trim()}
              class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
            >{extracting ? 'extracting…' : 'Extract'}</button>
            {#if extractError}
              <span class="text-xs text-error">{extractError}</span>
            {/if}
          </div>
        {/if}
      </section>

      <!-- Right: proposal -->
      <section class="bg-base border border-surface1 rounded-lg p-3 flex flex-col">
        <header class="flex items-baseline justify-between mb-2">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Proposal</h2>
          {#if proposal}
            <span class="text-[10px] text-dim font-mono">
              {acceptedCount()}/{proposal.items.length} accepted
            </span>
          {/if}
        </header>

        {#if commitResult}
          <!-- Post-commit summary. Keeps the user oriented after the
               write — what landed, what skipped, where it lives. -->
          <div class="bg-surface0 border border-success/40 rounded p-3 text-sm">
            <p class="text-text font-medium mb-1">
              Committed {commitResult.created_task_ids.length} task{commitResult.created_task_ids.length === 1 ? '' : 's'}
              {#if commitResult.created_milestones_count > 0}
                + {commitResult.created_milestones_count} milestone{commitResult.created_milestones_count === 1 ? '' : 's'}
              {/if}
            </p>
            <p class="text-xs text-subtext">
              Saved to <a href="/notes/{encodeURIComponent(commitResult.plan_path)}" class="text-secondary hover:underline">{commitResult.plan_path}</a>.
              Tasks live under their venture's section in that note.
            </p>
            {#if commitResult.skipped && commitResult.skipped.length > 0}
              <details class="mt-2">
                <summary class="text-xs text-warning cursor-pointer">Skipped ({commitResult.skipped.length})</summary>
                <ul class="mt-1 text-xs text-dim space-y-0.5">
                  {#each commitResult.skipped as s}
                    <li>· {s.label} — {s.reason}</li>
                  {/each}
                </ul>
              </details>
            {/if}
            <button
              type="button"
              onclick={() => { commitResult = null; }}
              class="mt-3 text-xs text-secondary hover:underline"
            >clear and extract more →</button>
          </div>
        {:else if !proposal && !extracting}
          <p class="text-sm text-dim italic flex-1">
            Write your plan on the left, then click Extract. The proposal lands here.
          </p>
        {:else if extracting}
          <p class="text-sm text-dim italic">thinking — matching items to your ventures, projects, calendar…</p>
        {:else if proposal}
          {#if proposal.warning}
            <p class="text-xs text-warning mb-2">⚠ {proposal.warning}</p>
            {#if proposal.raw}
              <details class="mb-2">
                <summary class="text-xs text-dim cursor-pointer">raw model response</summary>
                <pre class="text-[10px] text-dim bg-surface0 p-2 rounded mt-1 overflow-x-auto whitespace-pre-wrap">{proposal.raw}</pre>
              </details>
            {/if}
          {/if}
          {#if proposal.items.length === 0 && (!proposal.unmatched || proposal.unmatched.length === 0)}
            <p class="text-sm text-dim italic">No items extracted.</p>
          {:else}
            <div class="space-y-3 overflow-y-auto flex-1">
              {#each buckets as b (b.venture)}
                <section class="border border-surface1 rounded">
                  <header class="flex items-baseline gap-2 px-2.5 py-1.5 bg-surface0 border-b border-surface1">
                    <h3 class="text-sm font-medium text-text">{bucketLabel(b.venture)}</h3>
                    <span class="text-[10px] text-dim font-mono">{b.idxs.filter((i) => edits[i]?.accepted).length}/{b.idxs.length}</span>
                    <button
                      type="button"
                      onclick={() => setBucket(b.idxs, !bucketAllAccepted(b.idxs))}
                      class="ml-auto text-[11px] text-secondary hover:underline"
                    >{bucketAllAccepted(b.idxs) ? 'deselect all' : 'accept all'}</button>
                  </header>
                  <ul class="divide-y divide-surface1">
                    {#each b.idxs as i (i)}
                      {@const it = proposal.items[i]}
                      {@const badge = matchBadge(it)}
                      <li class="px-2.5 py-2 flex items-start gap-2 {edits[i]?.accepted ? '' : 'opacity-50'}">
                        <input
                          type="checkbox"
                          bind:checked={edits[i].accepted}
                          class="mt-1"
                        />
                        <div class="flex-1 min-w-0">
                          <div class="flex items-baseline gap-2 flex-wrap">
                            <input
                              type="text"
                              bind:value={edits[i].label}
                              class="flex-1 min-w-0 bg-transparent text-sm text-text border-b border-transparent focus:border-primary focus:outline-none px-0 py-0"
                              disabled={!edits[i]?.accepted}
                            />
                            <span class="text-[10px] font-mono {badge.color} shrink-0">{badge.label}</span>
                            {#if it.kind === 'milestone'}
                              <span class="text-[10px] font-mono text-blue shrink-0">milestone</span>
                            {/if}
                          </div>
                          <div class="text-[11px] text-dim flex items-baseline gap-2 mt-0.5 flex-wrap">
                            {#if it.project_name}
                              <span title="matched project">→ {it.project_name}</span>
                            {/if}
                            {#if it.due_date}
                              <span class="font-mono">due {it.due_date}</span>
                            {/if}
                            {#if it.rationale}
                              <span class="italic">· {it.rationale}</span>
                            {/if}
                          </div>
                          {#if it.source_line && it.source_line.trim() !== it.label.trim()}
                            <div class="text-[10px] text-dim/70 italic mt-0.5 truncate" title={it.source_line}>
                              from: "{it.source_line}"
                            </div>
                          {/if}
                        </div>
                      </li>
                    {/each}
                  </ul>
                </section>
              {/each}

              {#if proposal.unmatched && proposal.unmatched.length > 0}
                <section class="border border-warning/40 rounded">
                  <header class="px-2.5 py-1.5 bg-surface0 border-b border-warning/40">
                    <h3 class="text-sm font-medium text-warning">Couldn't route ({proposal.unmatched.length})</h3>
                  </header>
                  <ul class="px-2.5 py-2 space-y-1 text-xs text-subtext">
                    {#each proposal.unmatched as line}
                      <li>· {line}</li>
                    {/each}
                  </ul>
                  <p class="px-2.5 pb-2 text-[10px] text-dim">
                    Rewrite these lines in your plan with a clearer venture or context, then re-extract.
                  </p>
                </section>
              {/if}
            </div>

            <!-- Commit bar — sticky-ish footer for the proposal pane -->
            <div class="mt-3 pt-3 border-t border-surface1 flex items-center gap-2">
              <button
                type="button"
                onclick={runCommit}
                disabled={committing || acceptedCount() === 0}
                class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
              >{committing ? 'committing…' : `Commit ${acceptedCount()} item${acceptedCount() === 1 ? '' : 's'}`}</button>
              <button
                type="button"
                onclick={() => { proposal = null; edits = {}; extractError = ''; }}
                disabled={committing}
                class="text-xs text-dim hover:text-text underline"
              >discard proposal</button>
              {#if commitError}
                <span class="text-xs text-error ml-auto">{commitError}</span>
              {/if}
            </div>
          {/if}
        {/if}
      </section>
    </div>
  </div>
</div>
