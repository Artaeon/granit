<script lang="ts">
  import { api, type Goal } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';

  let {
    open = $bindable(false),
    onCreated
  }: {
    open?: boolean;
    onCreated: (g: Goal) => void | Promise<void>;
  } = $props();

  let title = $state('');
  let description = $state('');
  let targetDate = $state('');
  let category = $state('');
  let project = $state('');
  let color = $state('blue');
  let reviewFrequency = $state<'' | 'weekly' | 'monthly' | 'quarterly'>('');
  let saving = $state(false);

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire'];
  const categoryOptions = ['career', 'health', 'learning', 'relationships', 'finance', 'creative', 'spiritual', 'other'];

  function reset() {
    title = '';
    description = '';
    targetDate = '';
    category = '';
    project = '';
    color = 'blue';
    reviewFrequency = '';
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
    if (!title.trim()) return;
    saving = true;
    try {
      const g = await api.createGoal({
        title: title.trim(),
        description: description.trim() || undefined,
        target_date: targetDate || undefined,
        category: category || undefined,
        project: project.trim() || undefined,
        color,
        status: 'active',
        review_frequency: reviewFrequency || undefined
      });
      reset();
      open = false;
      await onCreated(g);
      toast.success(`goal "${g.title}" created`);
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
      <h2 class="text-base font-semibold text-text flex-1">New goal</h2>
      <button type="button" onclick={() => (open = false)} class="text-dim hover:text-text" aria-label="close">×</button>
    </div>

    <div>
      <label for="ng-title" class="text-xs uppercase tracking-wider text-dim block mb-1">Title</label>
      <input
        id="ng-title"
        bind:value={title}
        autofocus
        required
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      />
    </div>

    <div>
      <label for="ng-desc" class="text-xs uppercase tracking-wider text-dim block mb-1">Description</label>
      <textarea
        id="ng-desc"
        bind:value={description}
        rows="3"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      ></textarea>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="ng-target" class="text-xs uppercase tracking-wider text-dim block mb-1">Target date</label>
        <input
          id="ng-target"
          type="date"
          bind:value={targetDate}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text"
        />
      </div>
      <div>
        <label for="ng-cat" class="text-xs uppercase tracking-wider text-dim block mb-1">Category</label>
        <select
          id="ng-cat"
          bind:value={category}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text"
        >
          <option value="">—</option>
          {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
        </select>
      </div>
    </div>

    <div>
      <label for="ng-project" class="text-xs uppercase tracking-wider text-dim block mb-1">Project (optional)</label>
      <input
        id="ng-project"
        bind:value={project}
        placeholder="link to a project name"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
      />
    </div>

    <div>
      <label for="ng-rev" class="text-xs uppercase tracking-wider text-dim block mb-1">Review frequency</label>
      <select
        id="ng-rev"
        bind:value={reviewFrequency}
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text"
      >
        <option value="">— none —</option>
        <option value="weekly">weekly</option>
        <option value="monthly">monthly</option>
        <option value="quarterly">quarterly</option>
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
      disabled={!title.trim() || saving}
      class="w-full px-4 py-2.5 bg-primary text-mantle rounded font-medium disabled:opacity-50"
    >{saving ? 'creating…' : 'Create goal'}</button>
  </form>
</Drawer>
