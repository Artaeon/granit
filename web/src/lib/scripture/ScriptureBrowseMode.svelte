<!--
  ScriptureBrowseMode — paginated catalogue list + topical filter
  chip strip + AI semantic search. Used to live inline in
  /scripture as the {:else if mode === 'browse'} branch.

  The filter input drives two parallel surfaces:
    1. Local substring filter (instantaneous, scope-of-`all`)
    2. AI "Ask AI" sibling that hands the same string to the
       semantic-search endpoint — picks 1-3 catalogue topics +
       returns their verses. Rendered above the substring list so
       the user can compare both.

  Parent owns `all` / `topics` / `activeTopic` (it pulls them from
  the server) and exposes `onSelectTopic` so a chip-tap can re-
  request a scoped list.
-->
<script lang="ts">
  import { api, type Scripture, type ScriptureTopic } from '$lib/api';
  import { toast } from '$lib/components/toast';

  let {
    all,
    topics,
    activeTopic,
    onSelectTopic,
    onUseAsTodayVerse
  }: {
    all: Scripture[];
    topics: ScriptureTopic[];
    activeTopic: string;
    onSelectTopic: (topic: string) => void | Promise<void>;
    onUseAsTodayVerse: (s: Scripture) => void;
  } = $props();

  let q = $state('');
  let aiSearching = $state(false);
  let aiSearchResults = $state<Scripture[]>([]);
  let aiSearchTopics = $state<string[]>([]);
  let aiSearchQuery = $state('');

  async function runSemanticSearch() {
    const query = q.trim();
    if (!query || aiSearching) return;
    aiSearching = true;
    try {
      const r = await api.scriptureSemanticSearch({ query });
      aiSearchResults = r.scriptures;
      aiSearchTopics = r.topics;
      aiSearchQuery = r.query;
      if (r.scriptures.length === 0 && r.topics.length === 0) {
        toast.info(topics.length === 0
          ? 'Topical search needs the bundled catalogue — add topics to your scriptures.md or revert to defaults.'
          : 'No matching topics — try a different query.');
      }
    } catch (e) {
      toast.error('AI search failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiSearching = false;
    }
  }

  function clearSemanticSearch() {
    aiSearchResults = [];
    aiSearchTopics = [];
    aiSearchQuery = '';
  }

  let filteredAll = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return all;
    return all.filter((v) =>
      v.text.toLowerCase().includes(term) ||
      (v.source ?? '').toLowerCase().includes(term)
    );
  });
</script>

<!-- Filter input + Ask AI sibling. Cmd-Enter on the input fires
     the semantic search so a keyboard-first user doesn't need to
     reach for the button. -->
<div class="flex items-center gap-2 mb-3">
  <input
    bind:value={q}
    onkeydown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); runSemanticSearch(); } }}
    placeholder="filter substring, or describe what you want and click Ask AI…"
    class="flex-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
  />
  <button
    type="button"
    onclick={runSemanticSearch}
    disabled={!q.trim() || aiSearching}
    class="px-3 py-2 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 flex-shrink-0"
    title="Find verses by meaning (Cmd-Enter) — AI maps your query to catalogue topics"
  >{aiSearching ? 'asking…' : 'Ask AI'}</button>
</div>

{#if aiSearchResults.length > 0 || aiSearchTopics.length > 0}
  <!-- AI matches sit above the catalogue list so the user can
       compare both. Topics chip strip surfaces what the model
       picked, so the user can refine into a single-topic view. -->
  <div class="mb-4 bg-surface0 border border-primary/40 rounded-lg p-3">
    <div class="flex items-baseline justify-between mb-2 gap-2">
      <h3 class="text-xs uppercase tracking-wider text-primary font-medium">AI matches for "{aiSearchQuery}"</h3>
      <button
        type="button"
        onclick={clearSemanticSearch}
        class="text-[11px] text-dim hover:text-text flex-shrink-0"
      >clear</button>
    </div>
    {#if aiSearchTopics.length > 0}
      <div class="flex flex-wrap gap-1 mb-2">
        {#each aiSearchTopics as t (t)}
          <button
            type="button"
            onclick={() => { clearSemanticSearch(); void onSelectTopic(t); }}
            class="text-[11px] px-2 py-0.5 rounded-full border border-primary/40 bg-mantle text-subtext hover:border-primary hover:text-text"
            title="Focus catalogue list on this topic"
          >{t}</button>
        {/each}
      </div>
    {/if}
    {#if aiSearchResults.length > 0}
      <ul class="space-y-2">
        {#each aiSearchResults as v}
          <li>
            <p class="text-sm text-text font-serif italic leading-relaxed">"{v.text}"</p>
            {#if v.source}
              <p class="text-xs text-subtext mt-0.5">— {v.source}</p>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

{#if topics.length > 0}
  <!-- Topical chip strip — click a theme to scope the list. -->
  <div class="flex flex-wrap gap-1.5 mb-4">
    <button
      type="button"
      onclick={() => void onSelectTopic('')}
      class="text-xs px-2 py-1 rounded-full border transition-colors {activeTopic === '' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
    >All</button>
    {#each topics as t (t.topic)}
      <button
        type="button"
        onclick={() => void onSelectTopic(t.topic)}
        class="text-xs px-2 py-1 rounded-full border transition-colors {activeTopic === t.topic ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
        title="{t.count} verses"
      >{t.topic} <span class="opacity-70">{t.count}</span></button>
    {/each}
  </div>
{/if}

<ul class="divide-y divide-surface1 bg-surface0 border border-surface1 rounded-lg">
  {#each filteredAll as v}
    <li class="px-4 py-3 group">
      <p class="text-sm text-text font-serif italic leading-relaxed">"{v.text}"</p>
      <div class="flex items-baseline gap-2 mt-1.5 flex-wrap">
        {#if v.source}
          <p class="text-xs text-subtext">— {v.source}</p>
        {/if}
        {#if v.topics && v.topics.length > 0}
          <div class="flex flex-wrap gap-1">
            {#each v.topics as tag (tag)}
              <button
                type="button"
                onclick={() => void onSelectTopic(tag)}
                class="text-[10px] px-1.5 py-0.5 rounded bg-mantle border border-surface1 text-dim hover:border-primary hover:text-text"
                title="Filter by {tag}"
              >{tag}</button>
            {/each}
          </div>
        {/if}
        <span class="flex-1"></span>
        <button
          type="button"
          onclick={() => onUseAsTodayVerse(v)}
          class="text-[11px] text-dim hover:text-primary opacity-0 group-hover:opacity-100 transition-opacity"
          title="Promote to verse view"
        >read →</button>
      </div>
    </li>
  {/each}
</ul>
{#if filteredAll.length === 0}
  <p class="text-sm text-dim italic mt-4 text-center">
    {activeTopic ? `No verses tagged "${activeTopic}".` : 'No matches.'}
  </p>
{/if}
<p class="text-[11px] text-dim italic mt-3">
  Edit <code>.granit/scriptures.md</code> to add your own — same file the granit TUI reads.
</p>
