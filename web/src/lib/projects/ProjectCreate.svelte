<script lang="ts">
  import { api, type Project, type ProjectKind } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';

  let {
    open = $bindable(false),
    ventures = [],
    onCreated
  }: {
    open?: boolean;
    /** Existing venture names so the input can autocomplete instead of
     *  forcing the user to retype. Optional — passed from the list page
     *  where we already have the project list. */
    ventures?: string[];
    onCreated: (p: Project) => void | Promise<void>;
  } = $props();

  let name = $state('');
  let description = $state('');
  let folder = $state('');
  let color = $state('blue');
  let category = $state('');
  let kind = $state<ProjectKind | ''>('');
  let venture = $state('');
  let repoUrl = $state('');
  let saving = $state(false);

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire', 'flamingo'];
  const categoryOptions = ['development', 'social-media', 'personal', 'business', 'writing', 'research', 'health', 'finance', 'other'];
  const kindOptions: { value: ProjectKind; label: string; hint: string }[] = [
    { value: 'software', label: 'Software', hint: 'app, library, service' },
    { value: 'content', label: 'Content', hint: 'writing, video, audio' },
    { value: 'research', label: 'Research', hint: 'investigation, learning' },
    { value: 'business', label: 'Business', hint: 'ops, sales, strategy' },
    { value: 'creative', label: 'Creative', hint: 'art, music, design' },
    { value: 'client', label: 'Client work', hint: 'paid engagement' },
    { value: 'personal', label: 'Personal', hint: 'private / household' },
    { value: 'other', label: 'Other', hint: 'unclassified' }
  ];

  function reset() {
    name = '';
    description = '';
    folder = '';
    color = 'blue';
    category = '';
    kind = '';
    venture = '';
    repoUrl = '';
  }

  function colorVar(c: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c] ?? 'secondary'})`;
  }

  async function submit(e?: SubmitEvent) {
    e?.preventDefault();
    if (!name.trim()) return;
    saving = true;
    try {
      const p = await api.createProject({
        name: name.trim(),
        description: description.trim(),
        folder: folder.trim(),
        color,
        category: category || undefined,
        kind: kind || undefined,
        venture: venture.trim() || undefined,
        repo_url: kind === 'software' ? repoUrl.trim() || undefined : undefined,
        status: 'active',
        tags: []
      });
      reset();
      open = false;
      await onCreated(p);
      toast.success(`project "${p.name}" created`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  <form onsubmit={submit} class="p-4 space-y-4 h-full overflow-y-auto">
    <div class="flex items-center gap-2">
      <h2 class="text-base font-semibold text-text flex-1">New project</h2>
      <button type="button" onclick={() => (open = false)} class="text-dim hover:text-text" aria-label="close">×</button>
    </div>

    <div>
      <label for="np-name" class="text-xs uppercase tracking-wider text-dim block mb-1">Name</label>
      <input
        id="np-name"
        bind:value={name}
        autofocus
        required
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      />
    </div>

    <div>
      <label for="np-desc" class="text-xs uppercase tracking-wider text-dim block mb-1">Description</label>
      <textarea
        id="np-desc"
        bind:value={description}
        rows="3"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      ></textarea>
    </div>

    <div>
      <label for="np-folder" class="text-xs uppercase tracking-wider text-dim block mb-1">Folder (vault path)</label>
      <input
        id="np-folder"
        bind:value={folder}
        placeholder="e.g. Projects/site-redesign"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      />
      <p class="text-[11px] text-dim mt-1">Tasks created under this folder will auto-link to the project.</p>
    </div>

    <div>
      <span class="text-xs uppercase tracking-wider text-dim block mb-1">Kind</span>
      <!-- Kind tiles: more glanceable than a dropdown for ~8 options.
           Picking software unlocks the repo URL field below. -->
      <div class="grid grid-cols-2 gap-1.5">
        {#each kindOptions as k}
          <button
            type="button"
            onclick={() => (kind = kind === k.value ? '' : k.value)}
            class="text-left px-2.5 py-2 rounded border text-sm transition-colors
              {kind === k.value
                ? 'bg-primary/15 border-primary text-primary'
                : 'bg-surface0 border-surface1 text-subtext hover:border-primary/40 hover:text-text'}"
          >
            <div class="font-medium">{k.label}</div>
            <div class="text-[10px] text-dim mt-0.5">{k.hint}</div>
          </button>
        {/each}
      </div>
    </div>

    {#if kind === 'software'}
      <div>
        <label for="np-repo" class="text-xs uppercase tracking-wider text-dim block mb-1">Repo URL</label>
        <input
          id="np-repo"
          type="url"
          bind:value={repoUrl}
          placeholder="https://github.com/you/repo"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary"
        />
      </div>
    {/if}

    <div>
      <label for="np-venture" class="text-xs uppercase tracking-wider text-dim block mb-1">Venture / Company</label>
      <input
        id="np-venture"
        bind:value={venture}
        list="np-venture-list"
        placeholder="e.g. Stoicera, Personal, Side Hustle"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      />
      {#if ventures.length > 0}
        <datalist id="np-venture-list">
          {#each ventures as v}<option value={v}></option>{/each}
        </datalist>
      {/if}
      <p class="text-[11px] text-dim mt-1">Free-text — projects sharing a venture name group together in the list.</p>
    </div>

    <div>
      <label for="np-cat" class="text-xs uppercase tracking-wider text-dim block mb-1">Category (legacy)</label>
      <select
        id="np-cat"
        bind:value={category}
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text"
      >
        <option value="">— none —</option>
        {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
      </select>
    </div>

    <div>
      <span class="text-xs uppercase tracking-wider text-dim block mb-1">Color</span>
      <div class="flex gap-2 flex-wrap">
        {#each colorOptions as c}
          <button
            type="button"
            onclick={() => (color = c)}
            aria-label="color {c}"
            class="w-8 h-8 rounded-full border-2 {color === c ? 'border-text' : 'border-surface1'}"
            style="background: {colorVar(c)}"
          ></button>
        {/each}
      </div>
    </div>

    <button
      type="submit"
      disabled={!name.trim() || saving}
      class="w-full px-4 py-2.5 bg-primary text-on-primary rounded font-medium disabled:opacity-50"
    >{saving ? 'creating…' : 'Create project'}</button>
  </form>
</Drawer>
