<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import QuickAddTask from '$lib/components/QuickAddTask.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  let daily = $state<Note | null>(null);
  let loading = $state(true);

  async function load() {
    try {
      daily = await api.daily('today');
    } finally {
      loading = false;
    }
  }
  onMount(load);
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Quick capture</h2>
  {#if loading}
    <Skeleton class="h-12 w-full" />
  {:else if daily}
    <QuickAddTask notePath={daily.path} section="## Tasks" />
  {/if}
</section>
