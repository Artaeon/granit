<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { navigateToPage, notes } from '../stores'
  import { getJournalNotes, ensureJournalNote, saveNote } from '../api'
  import { parseMarkdown, serializeBlocks } from '../blocks'
  import BlockEditor from '../editor/BlockEditor.svelte'
  import type { NoteDetail, Block } from '../types'

  interface JournalDay {
    note: NoteDetail
    blocks: Block[]
    dirty: boolean
  }

  let days: JournalDay[] = []
  let loading = true
  let saveTimeouts: Record<string, ReturnType<typeof setTimeout>> = {}

  onMount(async () => {
    try {
      // Ensure today's note exists
      const today = new Date().toISOString().split('T')[0]
      await ensureJournalNote(today)

      // Load recent journal notes
      const journalNotes = await getJournalNotes(8) || []
      days = journalNotes.map(note => ({
        note,
        blocks: parseMarkdown(note.content),
        dirty: false,
      }))
    } catch (e) {
      console.error('Failed to load journal:', e)
    }
    loading = false
  })

  function extractDate(relPath: string): string {
    // Strip folder path and .md extension to get YYYY-MM-DD
    const base = relPath.split('/').pop() || relPath
    return base.replace('.md', '')
  }

  function formatDate(relPath: string): string {
    const dateStr = extractDate(relPath)
    // Validate it's a date
    if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) return dateStr

    const today = new Date()
    const todayStr = today.toISOString().split('T')[0]
    const yesterday = new Date(today)
    yesterday.setDate(yesterday.getDate() - 1)
    const yesterdayStr = yesterday.toISOString().split('T')[0]

    if (dateStr === todayStr) return 'Today'
    if (dateStr === yesterdayStr) return 'Yesterday'

    const [year, month, day] = dateStr.split('-').map(Number)
    const date = new Date(year, month - 1, day)
    return date.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })
  }

  function formatDateSub(relPath: string): string {
    return extractDate(relPath)
  }

  function handleBlockChange(index: number) {
    days[index].dirty = true
    const relPath = days[index].note.relPath

    clearTimeout(saveTimeouts[relPath])
    saveTimeouts[relPath] = setTimeout(async () => {
      const content = serializeBlocks(days[index].blocks)
      try {
        await saveNote(relPath, content)
        days[index].dirty = false
        days = [...days]
      } catch (e) {
        console.error('Failed to save:', e)
      }
    }, 1500)
  }

  function handleSave(index: number) {
    const relPath = days[index].note.relPath
    clearTimeout(saveTimeouts[relPath])
    const content = serializeBlocks(days[index].blocks)
    saveNote(relPath, content).then(() => {
      days[index].dirty = false
      days = [...days]
    }).catch(e => console.error('Failed to save:', e))
  }

  onDestroy(() => {
    // Clear all pending save timeouts
    for (const key of Object.keys(saveTimeouts)) {
      clearTimeout(saveTimeouts[key])
    }
    // Flush any dirty entries
    for (let i = 0; i < days.length; i++) {
      if (days[i].dirty) {
        const content = serializeBlocks(days[i].blocks)
        saveNote(days[i].note.relPath, content).catch(e => console.error('Failed to save on destroy:', e))
        days[i].dirty = false
      }
    }
  })

  function handleWikilinkNav(text: string) {
    const target = text.replace(/^\[\[|\]\]$/g, '')
    const match = $notes.find(n =>
      n.title.toLowerCase() === target.toLowerCase() ||
      n.relPath.toLowerCase() === target.toLowerCase() + '.md'
    )
    if (match) navigateToPage(match.relPath)
  }
</script>

<div class="journal-view h-full overflow-y-auto">
  <div class="max-w-[780px] mx-auto px-8 py-8">
    {#if loading}
      <div class="flex items-center justify-center py-20">
        <div class="text-[14px] text-ctp-overlay0">Loading journal...</div>
      </div>
    {:else if days.length === 0}
      <div class="flex flex-col items-center justify-center py-20 gap-4">
        <p class="text-ctp-overlay0 text-[14px]">No journal entries yet.</p>
        <p class="text-ctp-overlay0 text-[13px]">Start writing and your daily notes will appear here.</p>
      </div>
    {:else}
      {#each days as day, i}
        <div class="journal-day mb-10" class:is-today={i === 0}>
          <!-- Date header -->
          <div class="flex items-baseline gap-3 mb-4 pb-2"
            style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 25%, transparent)">
            <h2 class="text-[20px] font-semibold tracking-tight"
              class:text-ctp-blue={i === 0}
              class:text-ctp-subtext1={i > 0}>
              {formatDate(day.note.relPath)}
            </h2>
            <span class="text-[11px] text-ctp-overlay0 font-mono">{formatDateSub(day.note.relPath)}</span>
            {#if day.dirty}
              <span class="w-1.5 h-1.5 rounded-full bg-ctp-peach animate-pulse"></span>
            {/if}
          </div>

          <!-- Block editor for this day -->
          <div class="pl-2">
            <BlockEditor
              blocks={day.blocks}
              on:change={() => handleBlockChange(i)}
              on:save={() => handleSave(i)}
              on:wikilink={(e) => handleWikilinkNav(e.detail)}
            />
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .journal-view {
    background: var(--ctp-base);
  }
</style>
