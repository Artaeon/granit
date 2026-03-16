<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  const dispatch = createEventDispatcher()

  const sections = [
    { title: 'Navigation', keys: [
      ['Tab / Shift+Tab', 'Cycle between panels'],
      ['Ctrl+P', 'Quick open (fuzzy search)'],
      ['Ctrl+J', 'Quick switch files'],
      ['Alt+1 / Alt+2 / Alt+3', 'Focus sidebar / editor / backlinks'],
      ['Esc', 'Close overlay / return to sidebar'],
    ]},
    { title: 'File Operations', keys: [
      ['Ctrl+N', 'Create new note'],
      ['Ctrl+S', 'Save current note'],
      ['F4', 'Rename current note'],
      ['Ctrl+X', 'Command palette'],
      ['Alt+D', 'Open today\'s daily note'],
      ['Alt+W', 'Open weekly note'],
    ]},
    { title: 'Editor', keys: [
      ['Ctrl+E', 'Toggle edit / preview / split'],
      ['Ctrl+B', 'Bold selection'],
      ['Ctrl+I', 'Italic selection'],
      ['Ctrl+U', 'Undo'],
      ['Ctrl+Y', 'Redo'],
      ['Ctrl+Z', 'Focus / zen mode'],
      ['Ctrl+F', 'Find in file'],
      ['Ctrl+H', 'Find and replace'],
    ]},
    { title: 'Views & Tools', keys: [
      ['Ctrl+G', 'Note graph'],
      ['Ctrl+T', 'Browse tags'],
      ['Ctrl+O', 'Note outline'],
      ['Ctrl+B', 'Bookmarks & recent notes'],
      ['Ctrl+L', 'Calendar'],
      ['Ctrl+R', 'AI bots'],
      ['Ctrl+W', 'Canvas'],
      ['Ctrl+,', 'Settings'],
      ['F5', 'Keyboard shortcuts'],
    ]},
    { title: 'Application', keys: [
      ['Ctrl+Q', 'Quit'],
      ['Alt+P', 'Plan My Day (AI)'],
      ['Alt+C', 'Command Center'],
    ]},
  ]
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[8%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[75vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">Keyboard Shortcuts</span>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex-1 overflow-y-auto p-4 space-y-5">
      {#each sections as section}
        <div>
          <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider mb-2">{section.title}</h3>
          <div class="space-y-1">
            {#each section.keys as [key, desc]}
              <div class="flex items-center justify-between py-1 px-2">
                <span class="text-sm text-ctp-text">{desc}</span>
                <kbd class="text-[12px] text-ctp-blue bg-ctp-surface0 px-2 py-0.5 rounded font-mono">{key}</kbd>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>
