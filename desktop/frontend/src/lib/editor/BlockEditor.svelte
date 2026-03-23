<script lang="ts">
  import { createEventDispatcher, tick, onDestroy } from 'svelte'
  import BlockItem from './BlockItem.svelte'
  import type { Block } from '../types'
  import { createBlock, findBlock, removeBlock, insertAfter, flattenBlocks } from '../blocks'

  export let blocks: Block[] = []

  const dispatch = createEventDispatcher()

  let focusedBlockId: string = ''
  let focusPosition: 'start' | 'end' | number = 'end'
  let blockRefs: Record<string, BlockItem> = {}

  function setRef(id: string, ref: BlockItem) {
    blockRefs[id] = ref
  }

  function triggerSave() {
    dispatch('change', blocks)
  }

  async function handleSplit(blockId: string, before: string, after: string) {
    const found = findBlock(blocks, blockId)
    if (!found) return

    found.block.content = before
    const newBlock = createBlock(after)
    insertAfter(found.parent, blockId, newBlock)
    blocks = [...blocks]

    focusedBlockId = newBlock.id
    focusPosition = 'start'

    await tick()
    blockRefs[newBlock.id]?.focusAtStart()
    triggerSave()
  }

  async function handleMergeUp(blockId: string) {
    const flat = flattenBlocks(blocks)
    const idx = flat.findIndex(f => f.block.id === blockId)
    if (idx <= 0) return

    const current = flat[idx].block
    const prev = flat[idx - 1].block
    const prevLen = prev.content.length

    prev.content += current.content
    // Move children to prev
    prev.children.push(...current.children)

    removeBlock(blocks, blockId)
    blocks = [...blocks]

    focusedBlockId = prev.id
    focusPosition = prevLen

    await tick()
    blockRefs[prev.id]?.focusAtPos(prevLen)
    triggerSave()
  }

  async function handleIndent(blockId: string) {
    const found = findBlock(blocks, blockId)
    if (!found || found.index === 0) return

    const sibling = found.parent[found.index - 1]
    const removed = found.parent.splice(found.index, 1)[0]
    sibling.children.push(removed)
    blocks = [...blocks]

    focusedBlockId = blockId
    await tick()
    triggerSave()
  }

  async function handleOutdent(blockId: string) {
    // Find the block and its parent
    function findWithParent(blocks: Block[], id: string, parentArr: Block[] | null, parentBlock: Block | null): { block: Block, parentArr: Block[], index: number, grandparentArr: Block[] | null, parentBlock: Block | null } | null {
      for (let i = 0; i < blocks.length; i++) {
        if (blocks[i].id === id) {
          return { block: blocks[i], parentArr: blocks, index: i, grandparentArr: parentArr, parentBlock }
        }
        const found = findWithParent(blocks[i].children, id, blocks, blocks[i])
        if (found) return found
      }
      return null
    }

    const found = findWithParent(blocks, blockId, null, null)
    if (!found || !found.grandparentArr || !found.parentBlock) return

    // Remove from current parent's children
    found.parentArr.splice(found.index, 1)

    // Insert after the parent block in grandparent
    const parentIdx = found.grandparentArr.indexOf(found.parentBlock)
    found.grandparentArr.splice(parentIdx + 1, 0, found.block)

    blocks = [...blocks]
    focusedBlockId = blockId
    await tick()
    triggerSave()
  }

  async function handleFocusPrev(blockId: string) {
    const flat = flattenBlocks(blocks)
    const idx = flat.findIndex(f => f.block.id === blockId)
    if (idx > 0) {
      const prev = flat[idx - 1].block
      focusedBlockId = prev.id
      focusPosition = 'end'
      await tick()
      blockRefs[prev.id]?.focusAtEnd()
    }
  }

  async function handleFocusNext(blockId: string) {
    const flat = flattenBlocks(blocks)
    const idx = flat.findIndex(f => f.block.id === blockId)
    if (idx < flat.length - 1) {
      const next = flat[idx + 1].block
      focusedBlockId = next.id
      focusPosition = 'start'
      await tick()
      blockRefs[next.id]?.focusAtStart()
    }
  }

  function handleBlockChange(blockId: string, content: string) {
    const found = findBlock(blocks, blockId)
    if (found) {
      found.block.content = content
      triggerSave()
    }
  }

  onDestroy(() => {
    blockRefs = {}
  })

  function handleToggleCollapse(blockId: string) {
    const found = findBlock(blocks, blockId)
    if (found) {
      found.block.collapsed = !found.block.collapsed
      blocks = [...blocks]
    }
  }
</script>

<div class="block-editor-container">
  {#each blocks as block (block.id)}
    <BlockItem
      {block}
      depth={0}
      focused={focusedBlockId === block.id}
      bind:this={blockRefs[block.id]}
      on:split={(e) => handleSplit(block.id, e.detail.before, e.detail.after)}
      on:merge-up={() => handleMergeUp(block.id)}
      on:indent={() => handleIndent(block.id)}
      on:outdent={() => handleOutdent(block.id)}
      on:focus-prev={() => handleFocusPrev(block.id)}
      on:focus-next={() => handleFocusNext(block.id)}
      on:change={(e) => handleBlockChange(block.id, e.detail)}
      on:focused={() => focusedBlockId = block.id}
      on:toggle-collapse={() => handleToggleCollapse(block.id)}
      on:save
      on:wikilink
    />
    {#if !block.collapsed && block.children.length > 0}
      <svelte:self blocks={block.children} on:change on:save on:wikilink />
    {/if}
  {/each}
</div>
