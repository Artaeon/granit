<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type PrayerIntention, type Project, type Venture, type Goal, type Scripture } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { page } from '$app/stores';

  // /prayer is the dedicated prayer surface, separate from the
  // /scripture#intentions tab. The product story: scripture-driven
  // intercession over the user's actual life — they pray *for* the
  // ventures, projects, goals, and people they've already modelled in
  // granit. Intentions group by what they're tied to so a quick scan
  // shows "what am I bringing before God for Stoicera right now?" at
  // a glance.

  let intentions = $state<PrayerIntention[]>([]);
  let projects = $state<Project[]>([]);
  let ventures = $state<Venture[]>([]);
  let goals = $state<Goal[]>([]);
  let verseToday = $state<Scripture | null>(null);
  let loading = $state(false);

  // Create-form state. ?project=… / ?goal=… / ?venture=… / ?person=…
  // / ?passage=… in the URL pre-fills the form so the 'Pray for this'
  // buttons on detail pages land here with the linkage already set.
  let nText = $state('');
  let nCategory = $state('');
  let nProject = $state('');
  let nGoal = $state('');
  let nVenture = $state('');
  let nPerson = $state('');
  let nNotePath = $state('');
  let nPassageRef = $state('');
  let saving = $state(false);
  let formOpen = $state(false);

  const categoryOptions = ['family', 'self', 'work', 'world', 'friends', 'church', 'guidance', 'thanksgiving'];

  async function loadAll() {
    if (!$auth) return;
    loading = true;
    try {
      // Parallel — small responses, single round-trip wall time.
      const [iRes, pRes, vRes, gRes, sRes] = await Promise.all([
        api.listPrayer(),
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 })),
        api.listVentures().catch(() => ({ ventures: [] as Venture[], total: 0 })),
        api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 })),
        api.todayScripture().catch(() => null)
      ]);
      intentions = iRes.intentions;
      projects = pRes.projects;
      ventures = vRes.ventures;
      goals = gRes.goals;
      verseToday = sRes;
    } catch (e) {
      toast.error('failed to load prayer page: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadAll();
    // Hydrate create form from URL params for the cross-page 'Pray
    // for this' jump path.
    const sp = $page.url.searchParams;
    if (sp.has('project')) nProject = sp.get('project') ?? '';
    if (sp.has('goal')) nGoal = sp.get('goal') ?? '';
    if (sp.has('venture')) nVenture = sp.get('venture') ?? '';
    if (sp.has('person')) nPerson = sp.get('person') ?? '';
    if (sp.has('note')) nNotePath = sp.get('note') ?? '';
    if (sp.has('passage')) nPassageRef = sp.get('passage') ?? '';
    if (sp.has('text')) nText = sp.get('text') ?? '';
    // Auto-open the form if any link arrived — the user's intent in
    // landing here from a 'Pray for this' button is to write something.
    if (nProject || nGoal || nVenture || nPerson || nNotePath || nPassageRef) {
      formOpen = true;
    }

    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') loadAll();
      if (ev.type.startsWith('project.')) loadAll();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') loadAll();
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') loadAll();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') loadAll();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  function resetForm() {
    nText = '';
    nCategory = '';
    nProject = '';
    nGoal = '';
    nVenture = '';
    nPerson = '';
    nNotePath = '';
    nPassageRef = '';
  }

  async function submitCreate(e?: SubmitEvent) {
    e?.preventDefault();
    if (!nText.trim()) return;
    saving = true;
    try {
      const created = await api.createPrayer({
        text: nText.trim(),
        category: nCategory || undefined,
        project: nProject.trim() || undefined,
        goal: nGoal.trim() || undefined,
        venture: nVenture.trim() || undefined,
        person: nPerson.trim() || undefined,
        note_path: nNotePath.trim() || undefined,
        passage_ref: nPassageRef.trim() || undefined,
        status: 'praying'
      });
      // Optimistic prepend.
      if (!intentions.some((x) => x.id === created.id)) {
        intentions = [created, ...intentions];
      }
      resetForm();
      formOpen = false;
      toast.success('intention added');
      await loadAll();
    } catch (err) {
      toast.error('save failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }

  async function setStatus(p: PrayerIntention, status: 'praying' | 'answered' | 'archived') {
    try {
      await api.patchPrayer(p.id, { status });
      await loadAll();
      if (status === 'answered') toast.success('marked as answered 🙏');
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function deleteIntention(p: PrayerIntention) {
    if (!confirm(`Delete intention "${p.text}"?`)) return;
    try {
      await api.deletePrayer(p.id);
      intentions = intentions.filter((x) => x.id !== p.id);
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Active intentions only on the main view; answered + archived live
  // under a collapsed section so the page leads with what's still being
  // prayed for. Within each linkage group, newest first.
  let active = $derived(intentions.filter((p) => p.status === 'praying'));
  let answered = $derived(intentions.filter((p) => p.status === 'answered'));
  let archived = $derived(intentions.filter((p) => p.status === 'archived'));

  // Grouping. The render walks: Ventures (alphabetic), then Projects
  // (alphabetic), then Goals, then People, then "general" (no
  // linkage). An intention attached to multiple entities surfaces in
  // the FIRST matching group only — ventures > project > goal >
  // person > general — to avoid duplicate cards. Surfacing duplicates
  // would feel like clutter.
  type Group = { kind: 'venture' | 'project' | 'goal' | 'person' | 'general'; key: string; label: string; intentions: PrayerIntention[] };

  let groups = $derived.by((): Group[] => {
    const byVenture = new Map<string, PrayerIntention[]>();
    const byProject = new Map<string, PrayerIntention[]>();
    const byGoal = new Map<string, PrayerIntention[]>();
    const byPerson = new Map<string, PrayerIntention[]>();
    const general: PrayerIntention[] = [];

    for (const p of active) {
      if (p.venture) {
        const arr = byVenture.get(p.venture) ?? [];
        arr.push(p);
        byVenture.set(p.venture, arr);
      } else if (p.project) {
        const arr = byProject.get(p.project) ?? [];
        arr.push(p);
        byProject.set(p.project, arr);
      } else if (p.goal) {
        const arr = byGoal.get(p.goal) ?? [];
        arr.push(p);
        byGoal.set(p.goal, arr);
      } else if (p.person) {
        const arr = byPerson.get(p.person) ?? [];
        arr.push(p);
        byPerson.set(p.person, arr);
      } else {
        general.push(p);
      }
    }

    const out: Group[] = [];
    const sortKeys = (m: Map<string, PrayerIntention[]>): string[] =>
      [...m.keys()].sort((a, b) => a.localeCompare(b));

    for (const k of sortKeys(byVenture)) out.push({ kind: 'venture', key: k, label: k, intentions: byVenture.get(k)! });
    for (const k of sortKeys(byProject)) out.push({ kind: 'project', key: k, label: k, intentions: byProject.get(k)! });
    for (const k of sortKeys(byGoal)) {
      const g = goals.find((x) => x.id === k);
      out.push({ kind: 'goal', key: k, label: g?.title ?? k, intentions: byGoal.get(k)! });
    }
    for (const k of sortKeys(byPerson)) out.push({ kind: 'person', key: k, label: k, intentions: byPerson.get(k)! });
    if (general.length > 0) out.push({ kind: 'general', key: '__general__', label: 'General', intentions: general });
    return out;
  });

  // Per-group icon + accent — keeps the grouped page scannable at a
  // glance. Tones come from the existing palette so they harmonize
  // with the venture / project / goal cards on their respective pages.
  function groupIcon(kind: Group['kind']): string {
    switch (kind) {
      case 'venture': return '🏢';
      case 'project': return '📁';
      case 'goal': return '🎯';
      case 'person': return '👤';
      default: return '🙏';
    }
  }
  function groupHref(g: Group): string | null {
    switch (g.kind) {
      case 'venture': return `/projects?venture=${encodeURIComponent(g.key)}`;
      case 'project': return `/projects?p=${encodeURIComponent(g.key)}`;
      case 'goal': return `/goals?focus=${encodeURIComponent(g.key)}`;
      case 'person': return null;  // people page is search-driven
      default: return null;
    }
  }

  // Show all answered intentions but keep them collapsed by default so
  // the active list isn't crowded. The collapse toggle is per-section.
  let showAnswered = $state(false);
  let showArchived = $state(false);

  // Bible verse jump: passage_ref is free-text. We do a best-effort
  // parse to /bible/<book>/<chapter> when the format matches; otherwise
  // we link to /scripture which has the bible reader.
  function passageHref(ref: string): string {
    // Match "Book Chapter[:Verse[-Verse]]" loosely. e.g. "John 3:16",
    // "Phil 4:6-7", "Psalm 23". Unmatched → /scripture as the fallback.
    const m = ref.trim().match(/^(\d?\s?[A-Za-z]+)\s+(\d+)(?::|$)/);
    if (!m) return '/scripture';
    const book = m[1].trim();
    const chapter = m[2];
    return `/scripture?book=${encodeURIComponent(book)}&chapter=${encodeURIComponent(chapter)}`;
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <header class="mb-6">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Prayer</h1>
      <p class="text-sm text-dim mt-1">
        {active.length} active · what you're bringing before God for the work, the people, and your life
      </p>
    </header>

    <!-- Verse of the day. Sets the tone above the prayer list — God's
         word frames the requests. Links to the full /scripture surface. -->
    {#if verseToday}
      <a
        href="/scripture"
        class="block bg-surface0 border-l-4 border-primary rounded-r-lg p-4 mb-6 hover:bg-surface1 transition-colors"
      >
        <p class="text-sm text-text leading-relaxed italic">"{verseToday.text}"</p>
        {#if verseToday.source}
          <p class="text-xs text-primary mt-2 font-medium">— {verseToday.source}</p>
        {/if}
      </a>
    {/if}

    <!-- Add intention. Collapsed by default so the page leads with the
         existing list; the form appears expanded when arriving with a
         linkage prefilled (from a "Pray for this" button on a detail
         page). -->
    {#if !formOpen}
      <button
        onclick={() => (formOpen = true)}
        class="w-full mb-6 px-4 py-3 bg-surface0 border border-dashed border-surface1 rounded text-sm text-subtext hover:border-primary hover:text-primary transition-colors"
      >+ New intention</button>
    {:else}
      <form onsubmit={submitCreate} class="bg-surface0 border border-surface1 rounded-lg p-4 mb-6 space-y-3">
        <div class="flex items-center gap-2">
          <h2 class="text-sm font-medium text-text flex-1">New prayer intention</h2>
          <button
            type="button"
            onclick={() => { resetForm(); formOpen = false; }}
            aria-label="cancel"
            class="text-dim hover:text-text"
          >×</button>
        </div>
        <textarea
          bind:value={nText}
          required
          autofocus
          rows="2"
          placeholder="What are you bringing before God?"
          class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        ></textarea>

        <!-- Linkage. Show all four selectors so the user can attach
             without leaving the form. Free-text inputs with a
             datalist of existing entities — typing autocomplete or
             a brand-new value both work. -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div>
            <label for="np-venture" class="text-[11px] uppercase tracking-wider text-dim block mb-1">For venture</label>
            <input
              id="np-venture"
              bind:value={nVenture}
              list="np-ventures-list"
              placeholder="venture name"
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            />
            <datalist id="np-ventures-list">
              {#each ventures as v}<option value={v.name}></option>{/each}
            </datalist>
          </div>
          <div>
            <label for="np-project" class="text-[11px] uppercase tracking-wider text-dim block mb-1">For project</label>
            <input
              id="np-project"
              bind:value={nProject}
              list="np-projects-list"
              placeholder="project name"
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            />
            <datalist id="np-projects-list">
              {#each projects as p}<option value={p.name}></option>{/each}
            </datalist>
          </div>
          <div>
            <label for="np-goal" class="text-[11px] uppercase tracking-wider text-dim block mb-1">For goal</label>
            <select
              id="np-goal"
              bind:value={nGoal}
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            >
              <option value="">—</option>
              {#each goals as g}<option value={g.id}>{g.title}</option>{/each}
            </select>
          </div>
          <div>
            <label for="np-person" class="text-[11px] uppercase tracking-wider text-dim block mb-1">For person</label>
            <input
              id="np-person"
              bind:value={nPerson}
              placeholder="name"
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div>
            <label for="np-passage" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Scripture (optional)</label>
            <input
              id="np-passage"
              bind:value={nPassageRef}
              placeholder='e.g. "Phil 4:6-7"'
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div>
            <label for="np-cat" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Category (optional)</label>
            <select
              id="np-cat"
              bind:value={nCategory}
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            >
              <option value="">—</option>
              {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
            </select>
          </div>
        </div>

        <button
          type="submit"
          disabled={!nText.trim() || saving}
          class="w-full px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
        >{saving ? 'saving…' : 'Add intention'}</button>
      </form>
    {/if}

    {#if loading && intentions.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if active.length === 0 && answered.length === 0 && archived.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-1">Your prayer list is empty.</p>
        <p class="text-xs text-dim">
          Add what's on your heart above, or visit any
          <a href="/projects" class="text-primary hover:underline">project</a>,
          <a href="/goals" class="text-primary hover:underline">goal</a>, or
          <a href="/ventures" class="text-primary hover:underline">venture</a>
          and tap "Pray for this".
        </p>
      </div>
    {:else}
      <!-- Active intentions, grouped by what they're tied to. -->
      <div class="space-y-6">
        {#each groups as g (g.kind + ':' + g.key)}
          <section>
            <header class="flex items-baseline gap-2 mb-2 pb-1 border-b border-surface1">
              <span class="text-base flex-shrink-0">{groupIcon(g.kind)}</span>
              {#if groupHref(g)}
                <a href={groupHref(g)} class="text-sm font-medium text-text hover:text-primary truncate">{g.label}</a>
              {:else}
                <span class="text-sm font-medium text-text truncate">{g.label}</span>
              {/if}
              <span class="text-[10px] uppercase tracking-wider text-dim">{g.kind}</span>
              <span class="text-xs text-dim ml-auto flex-shrink-0">{g.intentions.length}</span>
            </header>
            <ul class="space-y-2">
              {#each g.intentions as p (p.id)}
                <li class="bg-surface0 border border-surface1 rounded p-3 group">
                  <div class="flex items-start gap-2">
                    <p class="text-sm text-text flex-1 break-words">{p.text}</p>
                    <div class="flex items-center gap-1 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onclick={() => setStatus(p, 'answered')}
                        title="mark answered"
                        class="text-xs text-dim hover:text-success px-1.5 py-0.5 rounded"
                      >✓</button>
                      <button
                        onclick={() => setStatus(p, 'archived')}
                        title="archive"
                        class="text-xs text-dim hover:text-text px-1.5 py-0.5 rounded"
                      >⌽</button>
                      <button
                        onclick={() => deleteIntention(p)}
                        title="delete"
                        class="text-xs text-dim hover:text-error px-1.5 py-0.5 rounded"
                      >×</button>
                    </div>
                  </div>
                  <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-dim mt-1.5">
                    {#if p.passage_ref}
                      <a
                        href={passageHref(p.passage_ref)}
                        class="text-primary hover:underline"
                        title="open this passage"
                      >📖 {p.passage_ref}</a>
                    {/if}
                    {#if p.category}<span>· {p.category}</span>{/if}
                    {#if p.started_at}<span>· since {p.started_at}</span>{/if}
                    {#if p.note_path}
                      <a href={`/notes/${encodeURIComponent(p.note_path)}`} class="hover:text-primary truncate font-mono">↗ {p.note_path}</a>
                    {/if}
                  </div>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>

      <!-- Answered: collapsed by default. Reading these is the "answered prayer
           journal" view — a record of how God has shown up. -->
      {#if answered.length > 0}
        <section class="mt-8">
          <button
            onclick={() => (showAnswered = !showAnswered)}
            class="w-full flex items-baseline gap-2 pb-1 border-b border-success/30 text-left"
          >
            <span class="text-base">🙌</span>
            <span class="text-sm font-medium text-success">Answered</span>
            <span class="text-[10px] uppercase tracking-wider text-dim">{answered.length}</span>
            <span class="ml-auto text-xs text-dim">{showAnswered ? '▾' : '▸'}</span>
          </button>
          {#if showAnswered}
            <ul class="space-y-2 mt-2">
              {#each answered as p (p.id)}
                <li class="bg-success/5 border border-success/20 rounded p-3">
                  <p class="text-sm text-text">{p.text}</p>
                  {#if p.answer}
                    <p class="text-xs text-success mt-1.5 italic">{p.answer}</p>
                  {/if}
                  <div class="text-[11px] text-dim mt-1.5">
                    {#if p.answered_at}answered {p.answered_at}{/if}
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}

      <!-- Archived. Hidden by default; shown on demand. -->
      {#if archived.length > 0}
        <section class="mt-6">
          <button
            onclick={() => (showArchived = !showArchived)}
            class="w-full flex items-baseline gap-2 pb-1 border-b border-surface1 text-left text-dim"
          >
            <span class="text-base">⌽</span>
            <span class="text-sm">Archived</span>
            <span class="text-[10px] uppercase tracking-wider">{archived.length}</span>
            <span class="ml-auto text-xs">{showArchived ? '▾' : '▸'}</span>
          </button>
          {#if showArchived}
            <ul class="space-y-1 mt-2">
              {#each archived as p (p.id)}
                <li class="text-sm text-dim px-2 py-1">{p.text}</li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}
    {/if}

    <footer class="mt-10 pt-4 border-t border-surface1 text-[11px] text-dim">
      Synced via <code class="font-mono">.granit/prayer/intentions.json</code> — the same file the granit TUI reads.
    </footer>
  </div>
</div>
