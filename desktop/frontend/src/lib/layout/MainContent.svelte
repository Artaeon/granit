<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { currentView, currentPagePath } from '../stores'
  import JournalView from '../pages/JournalView.svelte'
  import PageView from '../pages/PageView.svelte'
  import AllPages from '../pages/AllPages.svelte'
  import GraphPage from '../pages/GraphPage.svelte'
  import DashboardView from '../pages/DashboardView.svelte'

  const dispatch = createEventDispatcher()
</script>

<div class="flex-1 h-full overflow-hidden">
  {#if $currentView === 'journal'}
    <JournalView />
  {:else if $currentView === 'page'}
    <PageView relPath={$currentPagePath} />
  {:else if $currentView === 'allPages'}
    <AllPages />
  {:else if $currentView === 'graph'}
    <GraphPage />
  {:else if $currentView === 'dashboard'}
    <DashboardView on:command />
  {/if}
</div>
