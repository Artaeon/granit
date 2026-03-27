import type { Block } from './types'

let idCounter = 0

export function newBlockId(): string {
  return 'b' + Date.now().toString(36) + (idCounter++).toString(36)
}

export function createBlock(content: string = '', children: Block[] = []): Block {
  return { id: newBlockId(), content, children, collapsed: false }
}

/**
 * Parse markdown content into a Block[] tree.
 * Lines starting with `- ` become blocks. Indentation (2 spaces) = nesting.
 * Non-list content becomes single top-level blocks.
 */
export function parseMarkdown(markdown: string): Block[] {
  if (!markdown || !markdown.trim()) {
    return [createBlock()]
  }

  const lines = markdown.split('\n')
  const root: Block[] = []
  const stack: { blocks: Block[], indent: number }[] = [{ blocks: root, indent: -1 }]

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // Count leading spaces
    const stripped = line.replace(/^\s*/, '')
    const indent = line.length - stripped.length

    // Check if this is a list item
    if (stripped.startsWith('- ')) {
      const content = stripped.slice(2)
      const block = createBlock(content)

      // Find the right parent based on indentation
      while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
        stack.pop()
      }

      stack[stack.length - 1].blocks.push(block)
      stack.push({ blocks: block.children, indent })
    } else if (stripped === '') {
      // Skip empty lines between blocks
      continue
    } else {
      // Non-list content: treat as a top-level block
      // Reset stack to root
      while (stack.length > 1) stack.pop()
      root.push(createBlock(line))
    }
  }

  if (root.length === 0) {
    return [createBlock()]
  }

  return root
}

/**
 * Serialize a Block[] tree back to markdown.
 */
export function serializeBlocks(blocks: Block[], indent: number = 0): string {
  const lines: string[] = []
  const prefix = '  '.repeat(indent)

  for (const block of blocks) {
    lines.push(`${prefix}- ${block.content}`)
    if (block.children.length > 0) {
      lines.push(serializeBlocks(block.children, indent + 1))
    }
  }

  return lines.join('\n')
}

/**
 * Flatten a block tree into an ordered list with depth info, for keyboard navigation.
 */
export function flattenBlocks(blocks: Block[], depth: number = 0): { block: Block, depth: number, parent: Block | null, index: number }[] {
  const result: { block: Block, depth: number, parent: Block | null, index: number }[] = []
  for (let i = 0; i < blocks.length; i++) {
    const block = blocks[i]
    result.push({ block, depth, parent: null, index: i })
    if (!block.collapsed && block.children.length > 0) {
      const childFlat = flattenBlocks(block.children, depth + 1)
      childFlat.forEach(c => { if (c.depth === depth + 1) c.parent = block })
      result.push(...childFlat)
    }
  }
  return result
}

/**
 * Find a block by ID in the tree.
 */
export function findBlock(blocks: Block[], id: string): { block: Block, parent: Block[], index: number } | null {
  for (let i = 0; i < blocks.length; i++) {
    if (blocks[i].id === id) {
      return { block: blocks[i], parent: blocks, index: i }
    }
    const found = findBlock(blocks[i].children, id)
    if (found) return found
  }
  return null
}

/**
 * Remove a block by ID from the tree.
 */
export function removeBlock(blocks: Block[], id: string): Block | null {
  for (let i = 0; i < blocks.length; i++) {
    if (blocks[i].id === id) {
      return blocks.splice(i, 1)[0]
    }
    const found = removeBlock(blocks[i].children, id)
    if (found) return found
  }
  return null
}

/**
 * Insert a block after another block (by ID) at the same level.
 */
export function insertAfter(blocks: Block[], afterId: string, newBlock: Block): boolean {
  for (let i = 0; i < blocks.length; i++) {
    if (blocks[i].id === afterId) {
      blocks.splice(i + 1, 0, newBlock)
      return true
    }
    if (insertAfter(blocks[i].children, afterId, newBlock)) return true
  }
  return false
}
