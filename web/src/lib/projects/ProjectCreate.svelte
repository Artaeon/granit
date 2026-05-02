<script lang="ts">
  import { api, type Project } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';

  let {
    open = $bindable(false),
    onCreated
  }: {
    open?: boolean;
    onCreated: (p: Project) => void | Promise<void>;
  } = $props();

  let name = $state('');
  let description = $state('');
  let folder = $state('');
  let color = $state('blue');
  let category = $state('');
  let saving = $state(false);

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire', 'flamingo'];
  const categoryOptions = ['development', 'social-media', 'personal', 'business', 'writing', 'research', 'health', 'finance', 'other'];

  function reset() {
    name = '';
    description = '';
    folder = '';
    color = 'blue';
    category = '';
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
      <label for="np-cat" class="text-xs uppercase tracking-wider text-dim block mb-1">Category</label>
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
      class="w-full px-4 py-2.5 bg-primary text-mantle rounded font-medium disabled:opacity-50"
    >{saving ? 'creating…' : 'Create project'}</button>
  </form>
</Drawer>
