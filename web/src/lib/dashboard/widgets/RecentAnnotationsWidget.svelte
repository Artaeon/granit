<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type NoteAnnotation } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { relativeTime } from '$lib/util/relativeTime';
  import { annotationBarClass } from '$lib/notes/annotationColors';

  // Recent margin notes — the most recently touched annotations
  // across the vault. Surfaces the user's marginalia layer on the
  // home page so re-reads don't have to start by opening the
  // source note. Each row: anchor preview · annotation text ·
  // path · timestamp. Click → opens the note focused on the
  // annotated line.
  //
  // Distinct from RecentNotesWidget: that one is "what did I
  // edit"; this is "what did I think about". The daily review
  // surface, in widget form.

  let items = $state<NoteAnnotation[]>([]);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      // Empty notePath returns the full store, then we sort and
      // slice client-side. Cheap — even a vault with 1000+ rows
      // is kilobytes; the widget loads once per dashboard paint.
      const r = await api.listAnnotations();
      const sorted = [...r.annotations].sort((a, b) => {
        const at = Date.parse(a.updatedAt ?? a.createdAt);
        const bt = Date.parse(b.updatedAt ?? b.createdAt);
        return (Number.isNaN(bt) ? 0 : bt) - (Number.isNaN(at) ? 0 : at);
      });
      items = sorted.slice(0, 5);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/annotations.json') {
        void load();
      }
    });
  });
  onDestroy(() => {});

  function open(a: NoteAnnotation) {
    // Deep-link to the note; the editor's own ?line=… handling
    // would scroll on land if it existed — for v1 the user lands
    // on the note and can click the matching margin card to jump.
    goto(`/notes/${encodeURIComponent(a.notePath)}`);
  }

  function fileName(p: string): string {
    const base = p.split('/').pop() ?? p;
    return base.replace(/\.md$/, '');
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <header class="flex items-baseline justify-between mb-3">
    <h2 class="text-sm font-medium text-text">Margin notes</h2>
    <a href="/notes" class="text-xs text-dim hover:text-text">notes →</a>
  </header>

  {#if loading && items.length === 0}
    <div class="space-y-2">
      {#each Array(3) as _, i (i)}
        <div class="h-12 bg-surface1 rounded animate-pulse"></div>
      {/each}
    </div>
  {:else if items.length === 0}
    <p class="text-xs text-dim italic leading-relaxed">
      Margin notes are the marginalia layer for your notes — questions, counter-arguments, "this matters" markers anchored to a specific line. Add one from the right rail of any note.
    </p>
  {:else}
    <ul class="space-y-2">
      {#each items as a (a.id)}
        <li>
          <button
            type="button"
            onclick={() => open(a)}
            class="w-full text-left flex gap-2 p-2 rounded hover:bg-surface1 group"
          >
            <span class="w-1 self-stretch rounded-full {annotationBarClass(a.color)} flex-shrink-0"></span>
            <div class="flex-1 min-w-0">
              <p class="text-sm text-text line-clamp-2 leading-snug">{a.text}</p>
              <div class="flex items-baseline gap-1.5 mt-0.5 text-[11px] text-dim">
                <span class="truncate">{fileName(a.notePath)}</span>
                <span class="opacity-60">·</span>
                <span class="flex-shrink-0">L{a.lineNum}</span>
                <span class="opacity-60">·</span>
                <span class="flex-shrink-0">{relativeTime(a.updatedAt ?? a.createdAt)}</span>
              </div>
              {#if a.anchorText}
                <p class="text-[11px] text-dim italic line-clamp-1 mt-0.5">"{a.anchorText}"</p>
              {/if}
            </div>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>
