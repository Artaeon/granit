<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Scripture } from '$lib/api';

  // Verse-for-mood — dashboard companion to ScriptureWidget. Today's
  // verse is deterministic (the date picks it); this widget answers
  // the *opposite* question: "I'm feeling X — what does scripture
  // say?". The user taps a mood chip; we resolve it to a topic in the
  // catalogue and show 1–2 verses, with a shuffle action to walk
  // within the same topic without rolling all the way back to
  // /scripture.
  //
  // No AI call: the topical index is already on disk (Scripture.Topics
  // for the bundled catalogue). Chips that don't map to a present
  // topic are silently hidden so a user-edited scriptures.md with no
  // topic metadata degrades to "verse only" — better than an empty
  // chip strip the chips don't drive anything.

  // Curated mood → topic mapping. Every topic on the right MUST be
  // present in scripture.Defaults (internal/scripture/scripture.go)
  // so the chip strip drives at least one verse out of the box. If a
  // user-edited scriptures.md replaces the defaults and drops topic
  // metadata, scriptureTopics() returns empty and the chip strip
  // falls through to "show all" (see availableTopics handling below).
  //
  // Order is hand-tuned: high-frequency moods first so the strip
  // reads top-to-bottom in order of how often the user is likely to
  // need it.
  const MOOD_TOPICS: Array<{ label: string; topic: string }> = [
    { label: 'Anxious', topic: 'anxiety' },
    { label: 'Fearful', topic: 'fear' },
    { label: 'Hopeful', topic: 'hope' },
    { label: 'Patient', topic: 'patience' },
    { label: 'Grateful', topic: 'gratitude' },
    { label: 'Joyful', topic: 'joy' },
    { label: 'Grieving', topic: 'grief' },
    { label: 'Angry', topic: 'anger' },
    { label: 'Suffering', topic: 'suffering' },
    { label: 'Weary', topic: 'rest' },
    { label: 'Restless', topic: 'peace' },
    { label: 'Lost', topic: 'guidance' }
  ];

  let availableTopics = $state<Set<string>>(new Set());
  let activeMood = $state<{ label: string; topic: string } | null>(null);
  let verses = $state<Scripture[]>([]);
  let cursor = $state(0); // index into `verses` for the shuffle action
  let loading = $state(false);
  let error = $state<string | null>(null);

  // The visible chip set — only moods whose topic is actually present
  // in the catalogue, so a bare vault doesn't show dead chips.
  let chips = $derived(MOOD_TOPICS.filter((m) => availableTopics.size === 0 || availableTopics.has(m.topic)));

  onMount(async () => {
    try {
      const r = await api.scriptureTopics();
      availableTopics = new Set(r.topics.map((t) => t.topic.toLowerCase()));
    } catch {
      // Empty set → render every chip rather than none; the per-chip
      // fetch will say "no verses" if the topic is missing.
      availableTopics = new Set();
    }
  });

  async function selectMood(m: { label: string; topic: string }) {
    activeMood = m;
    loading = true;
    error = null;
    cursor = 0;
    try {
      const r = await api.listScriptures(m.topic);
      verses = r.scriptures;
      if (verses.length === 0) error = 'No verses tagged with this topic.';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load verses.';
      verses = [];
    } finally {
      loading = false;
    }
  }

  function shuffle() {
    if (verses.length <= 1) return;
    // Step forward deterministically rather than re-randomising — the
    // user wants "another one", not "any one", and a stable walk
    // through the topic is more pleasant than re-rolling the same
    // verse twice.
    cursor = (cursor + 1) % verses.length;
  }

  function clear() {
    activeMood = null;
    verses = [];
    cursor = 0;
    error = null;
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Verse for…</h2>
    <a href="/scripture" class="text-xs text-secondary hover:underline">all →</a>
  </div>

  <div class="flex flex-wrap gap-1 mb-2">
    {#each chips as m (m.topic)}
      <button
        onclick={() => selectMood(m)}
        class="text-xs px-2 py-0.5 rounded-full border transition-colors {activeMood?.topic === m.topic ? 'bg-primary/15 border-primary text-primary' : 'bg-base border-surface1 text-subtext hover:text-text hover:border-overlay0'}"
      >
        {m.label}
      </button>
    {/each}
  </div>

  {#if !activeMood}
    <p class="text-xs text-dim italic">Tap a mood for a verse on that theme.</p>
  {:else if loading}
    <p class="text-xs text-dim">Loading…</p>
  {:else if error}
    <p class="text-xs text-warning">{error}</p>
  {:else if verses.length > 0}
    {@const v = verses[cursor]}
    <blockquote class="text-sm text-text leading-relaxed font-serif italic">
      "{v.text}"
    </blockquote>
    <div class="flex items-baseline justify-between mt-2 gap-3">
      {#if v.source}
        <cite class="text-xs text-subtext not-italic">— {v.source}</cite>
      {:else}
        <span></span>
      {/if}
      <div class="flex items-center gap-2 text-xs">
        <span class="text-dim">{cursor + 1}/{verses.length}</span>
        <button onclick={shuffle} disabled={verses.length <= 1} class="text-secondary hover:underline disabled:text-dim disabled:no-underline">next ↻</button>
        <button onclick={clear} class="text-dim hover:text-subtext">clear</button>
      </div>
    </div>
  {/if}
</section>
