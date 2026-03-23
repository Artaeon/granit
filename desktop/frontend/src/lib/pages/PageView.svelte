<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { navigateToPage, activeNote, rightSidebarPage, notes } from '../stores'
  import { getNote, saveNote, addRecent } from '../api'
  import { parseMarkdown, serializeBlocks } from '../blocks'
  import BlockEditor from '../editor/BlockEditor.svelte'
  import PageReferences from '../shared/PageReferences.svelte'
  import type { NoteDetail, Block } from '../types'

  export let relPath: string

  let note: NoteDetail | null = null
  let blocks: Block[] = []
  let loading = true
  let dirty = false
  let saveTimeout: ReturnType<typeof setTimeout>
  let editingTitle = false
  let titleInput: HTMLInputElement

  $: if (relPath) loadPage(relPath)

  async function loadPage(path: string) {
    // Flush any pending save before loading new page
    if (dirty && note) {
      clearTimeout(saveTimeout)
      const content = serializeBlocks(blocks)
      try {
        await saveNote(note.relPath, content)
      } catch (e) {
        console.error('Failed to flush save on navigate:', e)
      }
    }
    loading = true
    dirty = false
    try {
      note = await getNote(path)
      activeNote.set(note)
      rightSidebarPage.set(path)
      blocks = parseMarkdown(note?.content || '')
      addRecent(path).catch(() => {})
    } catch (e) {
      console.error('Failed to load page:', e)
      note = null
    }
    loading = false
  }

  function handleBlockChange() {
    dirty = true
    clearTimeout(saveTimeout)
    saveTimeout = setTimeout(() => doSave(), 1500)
  }

  function doSave() {
    if (!note || !dirty) return
    clearTimeout(saveTimeout)
    const content = serializeBlocks(blocks)
    saveNote(note.relPath, content).then(() => {
      dirty = false
    }).catch(e => console.error('Save failed:', e))
  }

  function handleWikilinkNav(text: string) {
    const target = text.replace(/^\[\[|\]\]$/g, '')
    const match = $notes.find(n =>
      n.title.toLowerCase() === target.toLowerCase() ||
      n.relPath.toLowerCase() === target.toLowerCase() + '.md'
    )
    if (match) navigateToPage(match.relPath)
  }

  function pageTitle(path: string): string {
    return path.replace(/\.md$/, '').split('/').pop() || path
  }

  onDestroy(() => {
    clearTimeout(saveTimeout)
    if (dirty && note) doSave()
  })
</script>

<div class="page-view h-full overflow-y-auto bg-ctp-base">
  <div class="max-w-[780px] mx-auto px-8 py-8">
    {#if loading}
      <div class="flex items-center justify-center py-20">
        <div class="text-[14px] text-ctp-overlay0">Loading...</div>
      </div>
    {:else if !note}
      <div class="flex flex-col items-center justify-center py-20 gap-3">
        <p class="text-ctp-overlay0 text-[14px]">Page not found</p>
        <p class="text-ctp-overlay0 text-[12px]">{relPath}</p>
      </div>
    {:else}
      <!-- Page title -->
      <div class="mb-6">
        <h1 class="text-[26px] font-bold text-ctp-text leading-tight tracking-tight flex items-center gap-2">
          <span class="flex-1 min-w-0">{pageTitle(note.relPath)}</span>
          {#if dirty}
            <span class="w-2 h-2 rounded-full bg-ctp-peach animate-pulse flex-shrink-0"></span>
          {/if}
        </h1>
        <div class="flex items-center gap-3 mt-2 text-[11px] text-ctp-overlay0">
          <span>{note.wordCount} words</span>
          {#if note.links?.length}
            <span class="flex items-center gap-1">
              <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M6.5 7.5l3-3a2.1 2.1 0 0 1 3 3l-3 3"/></svg>
              {note.links.length} links
            </span>
          {/if}
          {#if note.backlinks?.length}
            <span class="flex items-center gap-1">
              <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M9.5 8.5l-3 3a2.1 2.1 0 0 1-3-3l3-3"/></svg>
              {note.backlinks.length} backlinks
            </span>
          {/if}
        </div>
      </div>

      <!-- Block editor -->
      <div class="pl-2">
        <BlockEditor
          {blocks}
          on:change={handleBlockChange}
          on:save={doSave}
          on:wikilink={(e) => handleWikilinkNav(e.detail)}
        />
      </div>

      <!-- Linked references -->
      <div class="mt-12 pt-6" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 25%, transparent)">
        <PageReferences relPath={note.relPath} on:navigate={(e) => navigateToPage(e.detail)} />
      </div>
    {/if}
  </div>
</div>
