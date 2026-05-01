<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type NoteTemplate } from '$lib/api';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { toast } from '$lib/components/toast';

  let templates = $state<NoteTemplate[]>([]);
  let loading = $state(false);
  let selected = $state<NoteTemplate | null>(null);
  let title = $state('');
  let folder = $state('');
  let busy = $state(false);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listTemplates();
      templates = r.templates;
      // Auto-select first non-blank as a sensible default
      if (!selected && templates.length > 0) selected = templates[0];
    } finally {
      loading = false;
    }
  }
  onMount(load);

  let builtin = $derived(templates.filter((t) => !t.isUser));
  let userTpl = $derived(templates.filter((t) => t.isUser));

  function slugify(s: string): string {
    return s.trim().replace(/[\\/]/g, '-').replace(/\s+/g, ' ');
  }

  async function create() {
    if (!selected) return;
    const t = title.trim();
    if (!t && selected.name !== 'Blank Note (no template)') {
      toast.warning('add a title');
      return;
    }
    const finalTitle = t || `Untitled ${new Date().toISOString().slice(0, 10)}`;
    let path = slugify(finalTitle) + '.md';
    if (folder.trim()) {
      path = folder.trim().replace(/\/+$/, '') + '/' + path;
    }
    busy = true;
    try {
      const created = await api.createFromTemplate({
        templateName: selected.name,
        path,
        title: finalTitle
      });
      toast.success(`created ${created.path}`);
      goto(`/notes/${encodeURIComponent(created.path)}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      toast.error(msg);
    } finally {
      busy = false;
    }
  }

  // Live preview: substitute {{title}} / {{date}} just like the server will
  const today = new Date().toISOString().slice(0, 10);
  let previewTitle = $derived(title.trim() || `Untitled ${today}`);
  let preview = $derived.by(() => {
    if (!selected) return '';
    return selected.content
      .replace(/\{\{date\}\}/g, today)
      .replace(/\{\{title\}\}/g, previewTitle);
  });
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-6xl mx-auto">
    <PageHeader
      title="New from template"
      subtitle="Built-in templates ship with granit; user templates load from .granit/templates/"
    />

    {#if loading && templates.length === 0}
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div class="space-y-2">
          {#each Array(6) as _}<Skeleton class="h-9 w-full" />{/each}
        </div>
        <div class="lg:col-span-2 space-y-2">
          <Skeleton class="h-10 w-full" />
          <Skeleton class="h-64 w-full" />
        </div>
      </div>
    {:else if templates.length === 0}
      <EmptyState icon="✎" title="No templates" description="Built-in templates should always be present — check the server logs." />
    {:else}
      <div class="grid grid-cols-1 lg:grid-cols-[18rem_1fr] gap-4 sm:gap-6">
        <!-- Left: template list -->
        <aside class="space-y-4">
          <section>
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Built-in</h3>
            <ul class="space-y-1">
              {#each builtin as t (t.name)}
                <li>
                  <button
                    onclick={() => (selected = t)}
                    class="w-full text-left px-3 py-2 rounded text-sm
                      {selected === t ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface0'}"
                  >
                    {t.name}
                  </button>
                </li>
              {/each}
            </ul>
          </section>
          {#if userTpl.length > 0}
            <section>
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Your templates</h3>
              <ul class="space-y-1">
                {#each userTpl as t (t.name)}
                  <li>
                    <button
                      onclick={() => (selected = t)}
                      class="w-full text-left px-3 py-2 rounded text-sm
                        {selected === t ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface0'}"
                    >
                      <span class="text-success mr-1">★</span>{t.name}
                    </button>
                  </li>
                {/each}
              </ul>
            </section>
          {:else}
            <section class="text-xs text-dim leading-relaxed">
              <p>Drop <code>.md</code> files in <code class="text-[10px]">.granit/templates/</code> to add your own.</p>
            </section>
          {/if}
        </aside>

        <!-- Right: form + preview -->
        <main class="space-y-4">
          {#if selected}
            <div class="bg-surface0 border border-surface1 rounded-lg p-4 space-y-3">
              <div>
                <label class="block text-xs text-dim mb-1" for="t-title">Title</label>
                <input
                  id="t-title"
                  bind:value={title}
                  placeholder="my new note"
                  class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text focus:outline-none focus:border-primary"
                />
              </div>
              <div>
                <label class="block text-xs text-dim mb-1" for="t-folder">Folder (optional)</label>
                <input
                  id="t-folder"
                  bind:value={folder}
                  placeholder="Notes"
                  class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text font-mono focus:outline-none focus:border-primary"
                />
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-dim">Selected: <span class="text-text">{selected.name}</span></span>
                <button
                  onclick={create}
                  disabled={busy}
                  class="px-4 py-2 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50"
                >
                  {busy ? 'creating…' : 'create note'}
                </button>
              </div>
            </div>

            {#if preview}
              <section>
                <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Preview</h3>
                <pre class="bg-surface0 border border-surface1 rounded p-3 sm:p-4 text-xs sm:text-sm text-subtext font-mono whitespace-pre-wrap overflow-x-auto leading-relaxed">{preview}</pre>
              </section>
            {/if}
          {/if}
        </main>
      </div>
    {/if}
  </div>
</div>
