import type { NoteDetail, NoteInfo, FolderNode, SearchHit, TaskItem, Project } from './types'

// @ts-ignore - Wails binds Go methods to window.go.main.GranitApp
const api = () => (window as any).go?.main?.GranitApp

async function call<T>(fn: () => Promise<T>, fallback?: T): Promise<T> {
  try {
    return await fn()
  } catch (e) {
    console.error('[Granit API]', e)
    if (fallback !== undefined) return fallback
    throw e
  }
}

// Vault
export const openVault = (path: string): Promise<void> => call(() => api().OpenVault(path))
export const selectVaultDialog = (): Promise<string> => call(() => api().SelectVaultDialog())
export const isVaultOpen = (): Promise<boolean> => call(() => api().IsVaultOpen())
export const getVaultPath = (): Promise<string> => call(() => api().GetVaultPath())
export const refreshVault = (): Promise<void> => call(() => api().RefreshVault())

// Notes
export const getNotes = (): Promise<NoteInfo[]> => call(() => api().GetNotes(), [])
export const getNote = (relPath: string): Promise<NoteDetail> => call(() => api().GetNote(relPath))
export const saveNote = (relPath: string, content: string): Promise<void> => call(() => api().SaveNote(relPath, content))
export const createNote = (name: string, content: string): Promise<string> => call(() => api().CreateNote(name, content))
export const deleteNote = (relPath: string): Promise<void> => call(() => api().DeleteNote(relPath))
export const renameNote = (oldPath: string, newName: string): Promise<string> => call(() => api().RenameNote(oldPath, newName))
export const createFolder = (name: string): Promise<void> => call(() => api().CreateFolder(name))
export const moveFile = (path: string, dir: string): Promise<string> => call(() => api().MoveFile(path, dir))

// Tree
export const getFolderTree = (): Promise<FolderNode> => call(() => api().GetFolderTree())

// Search
export const search = (query: string): Promise<SearchHit[]> => call(() => api().Search(query), [])

// Journal
export const getJournalNotes = (count: number): Promise<NoteDetail[]> => call(() => api().GetJournalNotes(count), [])
export const ensureJournalNote = (date: string): Promise<NoteDetail> => call(() => api().EnsureJournalNote(date))

// Backlinks
export const getBacklinkContext = (relPath: string): Promise<any[]> => call(() => api().GetBacklinkContext(relPath), [])

// Graph
export const getGraphData = (centerPath: string): Promise<any> => call(() => api().GetGraphData(centerPath))

// Settings
export const getAllSettings = (): Promise<any[]> => call(() => api().GetAllSettings(), [])
export const updateSetting = (key: string, value: any): Promise<void> => call(() => api().UpdateSetting(key, value))
export const getTheme = (): Promise<string> => call(() => api().GetTheme())
export const setTheme = (theme: string): Promise<void> => call(() => api().SetTheme(theme))

// Bookmarks
export const getBookmarks = (): Promise<any> => call(() => api().GetBookmarks())
export const toggleBookmark = (relPath: string): Promise<boolean> => call(() => api().ToggleBookmark(relPath))
export const addRecent = (relPath: string): Promise<void> => call(() => api().AddRecent(relPath))

// Tags
export const getAllTags = (): Promise<any[]> => call(() => api().GetAllTags(), [])
export const getNotesForTag = (tag: string): Promise<NoteInfo[]> => call(() => api().GetNotesForTag(tag), [])

// Git
export const gitStatus = (): Promise<string[]> => call(() => api().GitStatus(), [])
export const gitLog = (): Promise<string[]> => call(() => api().GitLog(), [])
export const gitDiff = (): Promise<string> => call(() => api().GitDiff())
export const gitCommit = (msg: string): Promise<string> => call(() => api().GitCommit(msg))
export const gitPush = (): Promise<string> => call(() => api().GitPush())
export const gitPull = (): Promise<string> => call(() => api().GitPull())
export const getGitBranch = (): Promise<string> => call(() => api().GetGitBranch(), '')

// Outline
export const getOutline = (relPath: string): Promise<any[]> => call(() => api().GetOutline(relPath), [])

// Stats
export const getVaultStats = (): Promise<any> => call(() => api().GetVaultStats())

// Templates
export const getTemplates = (): Promise<any[]> => call(() => api().GetTemplates(), [])
export const createFromTemplate = (idx: number, name: string): Promise<string> => call(() => api().CreateFromTemplate(idx, name))

// Trash
export const getTrashItems = (): Promise<any[]> => call(() => api().GetTrashItems(), [])
export const restoreFromTrash = (item: string): Promise<void> => call(() => api().RestoreFromTrash(item))
export const purgeFromTrash = (item: string): Promise<void> => call(() => api().PurgeFromTrash(item))

// Calendar
export const getCalendarData = (year: number, month: number): Promise<any> => call(() => api().GetCalendarData(year, month))

// Bots
export const getBotList = (): Promise<any[]> => call(() => api().GetBotList(), [])
export const runBot = (kind: number, path: string, question: string): Promise<any> => call(() => api().RunBot(kind, path, question))

// Export
export const exportHTML = (path: string): Promise<string> => call(() => api().ExportHTML(path))
export const exportText = (path: string): Promise<string> => call(() => api().ExportText(path))
export const exportPDF = (path: string): Promise<string> => call(() => api().ExportPDF(path))
export const exportAll = (): Promise<string> => call(() => api().ExportAll())

// Plugins
export const getPlugins = (): Promise<any[]> => call(() => api().GetPlugins(), [])

// Auto-link
export const getAutoLinkSuggestions = (path: string): Promise<any[]> => call(() => api().GetAutoLinkSuggestions(path), [])

// Note history
export const getNoteHistory = (path: string): Promise<any[]> => call(() => api().GetNoteHistory(path), [])
export const getNoteAtVersion = (path: string, hash: string): Promise<string> => call(() => api().GetNoteAtVersion(path, hash))
export const getNoteDiff = (path: string, hash: string): Promise<string> => call(() => api().GetNoteDiff(path, hash))
export const restoreNoteVersion = (path: string, hash: string): Promise<void> => call(() => api().RestoreNoteVersion(path, hash))

// Smart connections
export const getSmartConnections = (path: string): Promise<any[]> => call(() => api().GetSmartConnections(path), [])

// AI Chat
export const chatWithAI = (msg: string): Promise<string> => call(() => api().ChatWithAI(msg))
export const getWritingFeedback = (content: string): Promise<string> => call(() => api().GetWritingFeedback(content))

// Tasks
export const getAllTasks = (): Promise<TaskItem[]> => call(() => api().GetAllTasks(), [])
export const toggleTask = (file: string, line: number): Promise<void> => call(() => api().ToggleTask(file, line))
export const getRecurringTasks = (): Promise<any[]> => call(() => api().GetRecurringTasks(), [])

// Flashcards
export const getFlashcards = (path: string): Promise<any[]> => call(() => api().GetFlashcards(path), [])
export const getFlashcardProgress = (): Promise<string> => call(() => api().GetFlashcardProgress(), '')
export const saveFlashcardProgress = (data: string): Promise<void> => call(() => api().SaveFlashcardProgress(data))
export const getQuizQuestions = (path: string): Promise<any[]> => call(() => api().GetQuizQuestions(path), [])

// Timeline & Mind Map
export const getTimeline = (): Promise<any[]> => call(() => api().GetTimeline(), [])
export const getMindMapData = (path: string): Promise<string> => call(() => api().GetMindMapData(path))

// Canvas
export const saveCanvas = (name: string, data: string): Promise<void> => call(() => api().SaveCanvas(name, data))
export const getCanvas = (name: string): Promise<string> => call(() => api().GetCanvas(name))
export const listCanvases = (): Promise<string[]> => call(() => api().ListCanvases(), [])
export const deleteCanvas = (name: string): Promise<void> => call(() => api().DeleteCanvas(name))

// Kanban
export const getKanban = (): Promise<string> => call(() => api().GetKanban(), '')
export const saveKanban = (data: string): Promise<void> => call(() => api().SaveKanban(data))

// Workspaces
export const saveWorkspace = (name: string, data: string): Promise<void> => call(() => api().SaveWorkspace(name, data))
export const loadWorkspace = (name: string): Promise<string> => call(() => api().LoadWorkspace(name))
export const listWorkspaces = (): Promise<string[]> => call(() => api().ListWorkspaces(), [])
export const deleteWorkspace = (name: string): Promise<void> => call(() => api().DeleteWorkspace(name))
export const renameWorkspace = (oldName: string, newName: string): Promise<void> => call(() => api().RenameWorkspace(oldName, newName))

// Backups
export const createBackup = (): Promise<string> => call(() => api().CreateBackup())
export const listBackups = (): Promise<any[]> => call(() => api().ListBackups(), [])
export const deleteBackup = (name: string): Promise<void> => call(() => api().DeleteBackup(name))

// Habits
export const getHabits = (): Promise<string> => call(() => api().GetHabits(), '')
export const saveHabits = (data: string): Promise<void> => call(() => api().SaveHabits(data))

// Briefing & Prompts
export const getDailyBriefing = (): Promise<any> => call(() => api().GetDailyBriefing())
export const getJournalPrompts = (): Promise<any[]> => call(() => api().GetJournalPrompts(), [])

// Encryption
export const encryptNote = (path: string, password: string): Promise<void> => call(() => api().EncryptNote(path, password))
export const decryptNote = (path: string, password: string): Promise<string> => call(() => api().DecryptNote(path, password))
export const isNoteEncrypted = (path: string): Promise<boolean> => call(() => api().IsNoteEncrypted(path))
export const saveDecryptedNote = (path: string, password: string): Promise<void> => call(() => api().SaveDecryptedNote(path, password))

// Snippets
export const getSnippets = (): Promise<any[]> => call(() => api().GetSnippets(), [])
export const saveSnippets = (data: string): Promise<void> => call(() => api().SaveSnippets(data))

// Table
export const parseMarkdownTable = (content: string, pos: number): Promise<any> => call(() => api().ParseMarkdownTable(content, pos))

// Dataview
export const runDataviewQuery = (query: string): Promise<any[]> => call(() => api().RunDataviewQuery(query), [])

// Blog publishing
export const publishToBlog = (path: string, platform: string): Promise<string> => call(() => api().PublishToBlog(path, platform))

// Commands
export const getCommands = (): Promise<any[]> => call(() => api().GetCommands(), [])

// Known vaults
export const getKnownVaults = (): Promise<any[]> => call(() => api().GetKnownVaults(), [])

// Plugins (extended)
export const togglePlugin = (name: string): Promise<void> => call(() => api().TogglePlugin(name))
export const runPluginCommand = (plugin: string, cmd: string): Promise<string> => call(() => api().RunPluginCommand(plugin, cmd))

// Platform
export const getPlatform = (): Promise<string> => call(() => api().GetPlatform())

// Projects
export const getProjects = (): Promise<Project[]> => call(() => api().GetProjects(), [])
export const saveProjectsJSON = (data: string): Promise<void> => call(() => api().SaveProjectsJSON(data))
export const createProject = (data: string): Promise<void> => call(() => api().CreateProject(data))
export const updateProject = (idx: number, data: string): Promise<void> => call(() => api().UpdateProject(idx, data))
export const deleteProject = (idx: number): Promise<void> => call(() => api().DeleteProject(idx))
export const getProjectTasks = (filter: string): Promise<TaskItem[]> => call(() => api().GetProjectTasks(filter), [])
