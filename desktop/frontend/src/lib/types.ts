export interface NoteInfo {
  relPath: string
  title: string
  modTime: string
  size: number
}

export interface NoteDetail {
  relPath: string
  title: string
  content: string
  frontmatter: Record<string, any>
  links: string[]
  backlinks: string[]
  modTime: string
  wordCount: number
}

export interface FolderNode {
  name: string
  path: string
  isFolder: boolean
  children?: FolderNode[]
  expanded?: boolean
}

export interface SearchHit {
  relPath: string
  title: string
  line: number
  column: number
  matchLine: string
  score: number
}

export interface FlatTreeItem {
  name: string
  path: string
  isFolder: boolean
  depth: number
  expanded?: boolean
}

export interface Tab {
  relPath: string
  title: string
  dirty: boolean
  content: string
  scrollPos: number
}

export interface BacklinkEntry {
  relPath: string
  title: string
  context: string
}
