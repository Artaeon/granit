<!--
  AI link suggester — sits in the note editor's right info panel.
  Fires /ai/suggest-links with the current buffer + cap'd candidate
  pool. Renders accept/dismiss chips for proposed tags + outbound
  links. The vault graph stays sparse because typing [[…]] is friction;
  this surface lowers the friction without committing anything until
  the user clicks accept.

  UX rules:
   - Manual fire only (no auto-fire on every keystroke / save —
     would burn tokens for trivial edits). The button gates it so
     the user knows when an AI call goes out.
   - Each chip has accept (insert) / dismiss (forget for this
     session). Once accepted, the chip clears so it doesn't keep
     suggesting the same link.
   - Cancellable via AbortController; rerun replaces the result.
   - Empty + error states are explicit; no "ghost" loading shimmer
     that could be mistaken for a stuck request.
-->
<script lang="ts">
  import { api } from '$lib/api';

  let {
    notePath,
    body,
    existingTags = [],
    onAddTag,
    onInsertLink
  }: {
    notePath: string;
    body: string;
    existingTags?: string[];
    onAddTag: (tag: string) => void;
    onInsertLink: (markup: string) => void;
  } = $props();

  type Tag = { name: string; rationale?: string };
  type Link = {
    type: 'note' | 'project' | 'goal' | 'venture';
    ref: string;
    title?: string;
    rationale?: string;
  };

  let tags = $state<Tag[]>([]);
  let links = $state<Link[]>([]);
  let dismissed = $state<Set<string>>(new Set());
  let loading = $state(false);
  let error = $state<string | null>(null);
  let warning = $state<string | null>(null);
  let lastFiredAt = $state<number | null>(null);
  let abort: AbortController | null = null;

  async function fire() {
    if (loading) return;
    if (!notePath || body.trim().length < 30) {
      error = body.trim().length < 30 ? 'Note is too short to suggest links from.' : 'No note loaded.';
      return;
    }
    abort?.abort();
    abort = new AbortController();
    loading = true;
    error = null;
    warning = null;
    try {
      const res = await api.aiSuggestLinks(
        { note_path: notePath, content: body, existing_tags: existingTags },
        abort.signal
      );
      tags = res.tags ?? [];
      links = res.links ?? [];
      warning = res.warning ?? null;
      lastFiredAt = Date.now();
    } catch (e) {
      if ((e as Error).name === 'AbortError') return;
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
      abort = null;
    }
  }

  function cancel() {
    abort?.abort();
    abort = null;
    loading = false;
  }

  function acceptTag(tag: Tag) {
    onAddTag(tag.name);
    tags = tags.filter((t) => t.name !== tag.name);
  }

  function dismissTag(tag: Tag) {
    const next = new Set(dismissed);
    next.add('tag|' + tag.name);
    dismissed = next;
    tags = tags.filter((t) => t.name !== tag.name);
  }

  function acceptLink(link: Link) {
    // For notes: [[path|title]] if title differs, else [[path]].
    // For projects/goals/ventures: surface as a frontmatter-friendly
    // markup ([[Project: name]] doesn't index well; we use the bare
    // [[name]] form and let the user retype if they prefer prefixed).
    let markup: string;
    if (link.type === 'note') {
      const ref = link.ref.replace(/\.md$/, '');
      const title = link.title?.trim();
      markup = title && title !== ref ? `[[${ref}|${title}]]` : `[[${ref}]]`;
    } else {
      markup = `[[${link.ref}]]`;
    }
    onInsertLink(markup);
    links = links.filter((l) => !(l.type === link.type && l.ref === link.ref));
  }

  function dismissLink(link: Link) {
    const next = new Set(dismissed);
    next.add(link.type + '|' + link.ref);
    dismissed = next;
    links = links.filter((l) => !(l.type === link.type && l.ref === link.ref));
  }

  const linkIcon: Record<Link['type'], string> = {
    note: '📄',
    project: '🛠',
    goal: '🎯',
    venture: '🌱'
  };

  const isEmpty = $derived(tags.length === 0 && links.length === 0);
</script>

<div class="space-y-2">
  <div class="flex items-center gap-2">
    {#if loading}
      <button
        type="button"
        onclick={cancel}
        class="text-xs px-2 py-1 rounded border border-surface1 bg-surface0 text-subtext hover:text-text"
        title="Cancel current request"
      >
        Cancel
      </button>
      <span class="text-[11px] text-dim italic">analyzing note…</span>
    {:else}
      <button
        type="button"
        onclick={fire}
        class="text-xs px-2 py-1 rounded bg-surface1 hover:bg-surface1 text-primary border border-surface2 inline-flex items-center gap-1.5"
        title="Ask AI to suggest tags + outbound links from your vault"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3.5 h-3.5">
          <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round" />
        </svg>
        {lastFiredAt ? 'Suggest again' : 'Suggest links'}
      </button>
      {#if lastFiredAt}
        <span class="text-[10px] text-dim">fired {new Date(lastFiredAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
      {/if}
    {/if}
  </div>

  {#if error}
    <div class="text-xs text-error bg-surface0 border border-error rounded px-2 py-1.5">
      {error}
    </div>
  {/if}
  {#if warning}
    <div class="text-xs text-warning bg-surface0 border border-warning rounded px-2 py-1.5">
      {warning}
    </div>
  {/if}

  {#if !loading && lastFiredAt && isEmpty && !error}
    <div class="text-xs text-dim italic">No suggestions — the note doesn't match anything new.</div>
  {/if}

  {#if tags.length > 0}
    <div>
      <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Tags</div>
      <div class="flex flex-wrap gap-1">
        {#each tags as tag (tag.name)}
          <span
            class="group inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-xs bg-surface0 border border-surface1 hover:border-primary"
            title={tag.rationale ?? ''}
          >
            <button
              type="button"
              onclick={() => acceptTag(tag)}
              class="text-text hover:text-primary"
            >
              #{tag.name}
            </button>
            <button
              type="button"
              onclick={() => dismissTag(tag)}
              aria-label="dismiss suggestion"
              class="text-dim hover:text-error"
            >
              ×
            </button>
          </span>
        {/each}
      </div>
    </div>
  {/if}

  {#if links.length > 0}
    <div>
      <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Links</div>
      <div class="space-y-1">
        {#each links as link (link.type + ':' + link.ref)}
          <div
            class="flex items-start gap-1.5 group rounded border border-surface1 bg-surface0 px-2 py-1.5 hover:border-primary"
            title={link.rationale ?? ''}
          >
            <span class="text-xs leading-5">{linkIcon[link.type]}</span>
            <button
              type="button"
              onclick={() => acceptLink(link)}
              class="flex-1 text-left text-xs text-text hover:text-primary leading-5 truncate"
            >
              {link.title ?? link.ref}
              {#if link.rationale}
                <span class="block text-[10px] text-dim group-hover:text-subtext truncate">{link.rationale}</span>
              {/if}
            </button>
            <button
              type="button"
              onclick={() => dismissLink(link)}
              aria-label="dismiss suggestion"
              class="text-dim hover:text-error text-sm leading-none px-1"
            >
              ×
            </button>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>
