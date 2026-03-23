import { writable } from 'svelte/store'
import type { NoteDetail, NoteInfo, FolderNode } from './types'

// Vault state
export const vaultOpen = writable(false)
export const vaultPath = writable('')
export const notes = writable<NoteInfo[]>([])
export const tree = writable<FolderNode | null>(null)

// Navigation
export type ViewType = 'journal' | 'page' | 'graph' | 'allPages' | 'dashboard'
export const currentView = writable<ViewType>('journal')
export const currentPagePath = writable('')
export const activeNote = writable<NoteDetail | null>(null)

// Navigation history
const navHistory: string[] = []
let navIndex = -1
let navLock = false

// UI state
export const showLeftSidebar = writable(true)
export const showRightSidebar = writable(false)
export const rightSidebarPage = writable('')

// Overlay state
export const overlays = writable<Record<string, boolean>>({})

export function openOverlay(name: string) {
  overlays.update(o => {
    const cleared: Record<string, boolean> = {}
    for (const k in o) cleared[k] = false
    cleared[name] = true
    return cleared
  })
}

export function closeOverlay(name: string) {
  overlays.update(o => ({ ...o, [name]: false }))
}

export function closeAllOverlays() {
  overlays.update(o => {
    const cleared: Record<string, boolean> = {}
    for (const k in o) cleared[k] = false
    return cleared
  })
}

// Navigation helpers
export const favorites = writable<string[]>([])
export const recentPages = writable<string[]>([])

export function navigateToPage(relPath: string) {
  if (!relPath) return
  currentView.set('page')
  currentPagePath.set(relPath)
  if (!navLock) {
    if (navIndex < navHistory.length - 1) navHistory.splice(navIndex + 1)
    navHistory.push(relPath)
    navIndex = navHistory.length - 1
  }
  recentPages.update(r => {
    const filtered = r.filter(p => p !== relPath)
    return [relPath, ...filtered].slice(0, 20)
  })
}

export function navigateToJournal() {
  currentView.set('journal')
}

export function navigateBack(): boolean {
  if (navIndex > 0) {
    navIndex--
    navLock = true
    navigateToPage(navHistory[navIndex])
    navLock = false
    return true
  }
  return false
}

export function navigateForward(): boolean {
  if (navIndex < navHistory.length - 1) {
    navIndex++
    navLock = true
    navigateToPage(navHistory[navIndex])
    navLock = false
    return true
  }
  return false
}

// Focus mode
export const focusMode = writable(false)
