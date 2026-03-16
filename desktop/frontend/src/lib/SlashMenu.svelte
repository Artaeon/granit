<script lang="ts">
  import { createEventDispatcher, tick } from 'svelte'

  export let x: number = 0
  export let y: number = 0
  export let query: string = ''

  const dispatch = createEventDispatcher()
  const maxVisible = 10

  interface SlashItem {
    command: string
    label: string
    icon: string
    description: string
    insert: string
  }

  const items: SlashItem[] = [
    { command: 'h1', label: 'Heading 1', icon: 'H1', description: 'Large heading', insert: '# ' },
    { command: 'h2', label: 'Heading 2', icon: 'H2', description: 'Medium heading', insert: '## ' },
    { command: 'h3', label: 'Heading 3', icon: 'H3', description: 'Small heading', insert: '### ' },
    { command: 'bullet', label: 'Bullet List', icon: '\u2022', description: 'Unordered list item', insert: '- ' },
    { command: 'number', label: 'Numbered List', icon: '1.', description: 'Ordered list item', insert: '1. ' },
    { command: 'todo', label: 'To-do', icon: '\u2610', description: 'Checkbox task', insert: '- [ ] ' },
    { command: 'quote', label: 'Quote', icon: '\u275D', description: 'Block quote', insert: '> ' },
    { command: 'code', label: 'Code Block', icon: '{ }', description: 'Fenced code block', insert: '```\n\n```' },
    { command: 'table', label: 'Table', icon: '\u2261', description: 'Markdown table', insert: '| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n|          |          |          |' },
    { command: 'divider', label: 'Divider', icon: '\u2014', description: 'Horizontal rule', insert: '\n---\n' },
    { command: 'callout', label: 'Callout', icon: '!', description: 'Callout block', insert: '> [!note]\n> ' },
    { command: 'image', label: 'Image', icon: '\u25A3', description: 'Image embed', insert: '![alt text](url)' },
    { command: 'link', label: 'Wiki Link', icon: '[[', description: 'Internal note link', insert: '[[]]' },
    { command: 'date', label: "Today's Date", icon: '\u2636', description: 'Insert current date', insert: '{{date}}' },
    { command: 'time', label: 'Current Time', icon: '\u231A', description: 'Insert current time', insert: '{{time}}' },
    { command: 'frontmatter', label: 'Frontmatter', icon: '---', description: 'YAML front matter', insert: '---\ntitle: \ndate: {{date}}\ntags: []\n---\n' },
    { command: 'meeting', label: 'Meeting Notes', icon: '\u2637', description: 'Meeting template', insert: '## Meeting Notes\n\n**Date:** {{date}}\n**Attendees:**\n-\n\n**Agenda:**\n1.\n\n**Notes:**\n\n**Action Items:**\n- [ ] ' },
    { command: 'daily', label: 'Daily Note', icon: '\u263C', description: 'Daily template', insert: '# {{date}}\n\n## Tasks\n- [ ] \n\n## Notes\n\n## Reflection\n' },
    { command: 'bold', label: 'Bold', icon: 'B', description: 'Bold text', insert: '****' },
    { command: 'italic', label: 'Italic', icon: 'I', description: 'Italic text', insert: '**' },
    { command: 'highlight', label: 'Highlight', icon: '==', description: 'Highlighted text', insert: '====' },
    { command: 'strikethrough', label: 'Strikethrough', icon: '~~', description: 'Strikethrough text', insert: '~~~~' },
  ]

  let selectedIndex = 0
  let listEl: HTMLDivElement

  function fuzzyMatch(text: string, q: string): boolean {
    return text.toLowerCase().includes(q.toLowerCase())
  }

  $: filtered = query
    ? items.filter(i => fuzzyMatch(i.command, query) || fuzzyMatch(i.label, query) || fuzzyMatch(i.description, query))
    : items

  $: selectedIndex = Math.min(selectedIndex, Math.max(0, filtered.length - 1))

  function expandPlaceholders(text: string): string {
    const now = new Date()
    const date = now.toISOString().slice(0, 10)
    const time = now.toTimeString().slice(0, 5)
    return text
      .replace(/\{\{date\}\}/g, date)
      .replace(/\{\{time\}\}/g, time)
      .replace(/\{\{datetime\}\}/g, `${date} ${time}`)
  }

  export function handleKeydown(e: KeyboardEvent): boolean {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        selectedIndex = Math.min(selectedIndex + 1, filtered.length - 1)
        scrollToSelected()
        return true
      case 'ArrowUp':
        e.preventDefault()
        selectedIndex = Math.max(selectedIndex - 1, 0)
        scrollToSelected()
        return true
      case 'Enter':
        e.preventDefault()
        if (filtered[selectedIndex]) {
          dispatch('select', expandPlaceholders(filtered[selectedIndex].insert))
        }
        return true
      case 'Tab':
        e.preventDefault()
        if (filtered[selectedIndex]) {
          dispatch('select', expandPlaceholders(filtered[selectedIndex].insert))
        }
        return true
      case 'Escape':
        e.preventDefault()
        dispatch('close')
        return true
      case ' ':
        dispatch('close')
        return false
    }
    return false
  }

  function scrollToSelected() {
    tick().then(() => {
      const el = listEl?.querySelector(`[data-index="${selectedIndex}"]`)
      el?.scrollIntoView({ block: 'nearest' })
    })
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
{#if filtered.length > 0}
  <div class="fixed z-50" style="left: {x}px; top: {y}px;">
    <div class="bg-ctp-surface0 border border-ctp-surface1 rounded-lg shadow-xl overflow-hidden w-[280px]">

      <!-- Header -->
      <div class="px-3 py-1.5 border-b border-ctp-surface1">
        <div class="flex items-center gap-1">
          <span class="text-ctp-mauve font-bold text-sm">/</span>
          {#if query}
            <span class="text-[12px] text-ctp-text font-medium">{query}</span>
          {/if}
          <span class="text-[12px] text-ctp-mauve animate-pulse">|</span>
        </div>
      </div>

      <!-- Items -->
      <div bind:this={listEl} class="max-h-[320px] overflow-y-auto py-1">
        {#each filtered.slice(0, maxVisible) as item, i}
          <div
            data-index={i}
            class="flex items-center gap-2.5 px-3 py-1.5 cursor-pointer transition-colors duration-75
              {i === selectedIndex ? 'bg-ctp-blue/15' : 'hover:bg-ctp-surface1/50'}"
            on:click={() => dispatch('select', expandPlaceholders(item.insert))}
            on:mouseenter={() => selectedIndex = i}
          >
            <div class="w-7 h-7 rounded-md flex items-center justify-center flex-shrink-0 text-[11px] font-mono
              {i === selectedIndex ? 'bg-ctp-mauve/20 text-ctp-mauve' : 'bg-ctp-crust text-ctp-overlay1'}">
              {item.icon}
            </div>
            <div class="min-w-0 flex-1">
              <div class="text-[12px] {i === selectedIndex ? 'text-ctp-text font-medium' : 'text-ctp-subtext1'}">{item.label}</div>
              <div class="text-[9px] text-ctp-overlay0 truncate">{item.description}</div>
            </div>
          </div>
        {/each}
      </div>

      <!-- Scroll hint -->
      {#if filtered.length > maxVisible}
        <div class="px-3 py-1 border-t border-ctp-surface1 text-[9px] text-ctp-overlay0 text-right">
          +{filtered.length - maxVisible} more
        </div>
      {/if}

      <!-- Footer -->
      <div class="px-3 py-1 border-t border-ctp-surface1 text-[9px] text-ctp-overlay0 flex gap-3">
        <span><kbd class="bg-ctp-crust px-1 py-px rounded">&uarr;&darr;</kbd> navigate</span>
        <span><kbd class="bg-ctp-crust px-1 py-px rounded">Enter</kbd> insert</span>
        <span><kbd class="bg-ctp-crust px-1 py-px rounded">Esc</kbd> close</span>
      </div>
    </div>
  </div>
{/if}
