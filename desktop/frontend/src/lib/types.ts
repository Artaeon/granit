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
  cursorPos: number
}

export interface BacklinkEntry {
  relPath: string
  title: string
  context: string
}

export interface Block {
  id: string
  content: string
  children: Block[]
  collapsed: boolean
}

export interface TaskItem {
  text: string
  done: boolean
  notePath: string
  lineNum: number
  priority: number           // 0=none, 1=low, 2=med, 3=high, 4=highest
  dueDate: string            // "YYYY-MM-DD" or ""
  tags: string[]             // e.g. ["work", "urgent"]
  estimatedMinutes: number   // from ~30m or ~2h
  scheduledTime: string      // "HH:MM-HH:MM" or ""
  recurrence: string         // "daily", "weekly", etc. or ""
  goalId: string             // "G001" or ""
  snoozedUntil: string       // "YYYY-MM-DDTHH:MM" or ""
}

export interface ProjectMilestone {
  text: string
  done: boolean
}

export interface ProjectGoal {
  title: string
  done: boolean
  milestones: ProjectMilestone[]
}

export interface Project {
  name: string
  description: string
  folder: string
  tags: string[]
  status: 'active' | 'paused' | 'completed' | 'archived'
  color: string
  createdAt: string
  notes: string[]
  taskFilter: string
  category: string
  goals: ProjectGoal[]
  nextAction: string
  priority: number
  dueDate: string
  timeSpent: number
}

export interface KanbanCard {
  id: string
  title: string
  notePath: string
  lineNum: number
  done: boolean
  manual: boolean
  priority: number
  dueDate: string
  tags: string[]
  columnId: string
}

export interface KanbanColumn {
  id: string
  title: string
  color: string
  cards: KanbanCard[]
}
