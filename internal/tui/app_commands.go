package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

func (m *Model) executeCommand(action CommandAction) (tea.Model, tea.Cmd) {
	switch action {
	case CmdOpenFile:
		m.searchMode = true
		m.searchQuery = ""
		m.searchResults = m.vault.SortedPaths()
		m.searchCursor = 0
	case CmdNewNote:
		m.templates.SetSize(m.width, m.height)
		m.templates.OpenWithVault(m.vault.Root)
	case CmdSaveNote:
		cmd := m.saveCurrentNote()
		hookCmd := m.runPluginSaveHooks()
		syncCmd := m.autoSync.CommitAndPush()
		// Auto-tag if enabled
		var tagCmd tea.Cmd
		if m.autoTagger != nil && m.autoTagger.IsEnabled() {
			m.autoTagger.ai = m.aiConfig()
			// Collect existing vault tags for consistency
			var existingTags []string
			tagSet := make(map[string]bool)
			for _, p := range m.vault.SortedPaths() {
				if note := m.vault.GetNote(p); note != nil {
					if tags, ok := note.Frontmatter["tags"]; ok {
						if tagList, ok := tags.([]interface{}); ok {
							for _, t := range tagList {
								if s, ok := t.(string); ok && !tagSet[s] {
									existingTags = append(existingTags, s)
									tagSet[s] = true
								}
							}
						}
					}
				}
			}
			m.autoTagger.SetVaultTags(existingTags)
			tagCmd = m.autoTagger.TagNote(m.editor.GetContent())
		}
		// AI link suggestions on save
		var linkCmd tea.Cmd
		if m.autoLinkSuggest != nil && m.autoLinkSuggest.IsEnabled() {
			m.autoLinkSuggest.ai = m.aiConfig()
			linkCmd = m.autoLinkSuggest.SuggestLinks(
				m.editor.GetContent(), m.vault.SortedPaths(), m.activeNote)
		}
		return m, tea.Batch(cmd, hookCmd, syncCmd, tagCmd, linkCmd)
	case CmdDailyNote:
		m.openDailyNote()
	case CmdPrevDailyNote:
		m.navigateDailyNote(-1)
	case CmdNextDailyNote:
		m.navigateDailyNote(1)
	case CmdWeeklyNote:
		year, week := time.Now().ISOWeek()
		wName := fmt.Sprintf("%d-W%02d.md", year, week)
		wFolder := m.config.WeeklyNotesFolder
		if wFolder != "" {
			wName = filepath.Join(wFolder, wName)
		}
		wPath := filepath.Join(m.vault.Root, wName)
		if _, err := os.Stat(wPath); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(wPath), 0755); err != nil {
				m.reportError("create weekly note folder", err)
				return m, m.clearMessageAfter(5 * time.Second)
			}
			content := m.weeklyNoteContent(year, week)
			if err := atomicWriteNote(wPath, content); err != nil {
				m.reportError("create weekly note", err)
				return m, m.clearMessageAfter(5 * time.Second)
			}
			_ = m.vault.Scan()
			m.index = vault.NewIndex(m.vault)
			m.index.Build()
			m.sidebar.SetFiles(m.vault.SortedPaths())
			m.statusbar.SetNoteCount(m.vault.NoteCount())
			m.statusbar.SetMessage("Created weekly note: " + wName)
		}
		m.loadNote(wName)
		m.setSidebarCursorToFile(wName)
		m.setFocus(focusEditor)
	case CmdToggleView:
		m.viewMode = !m.viewMode
		if m.viewMode {
			m.statusbar.SetMode("VIEW")
			m.statusbar.SetViewMode(true)
			m.viewScroll = 0
			m.updateReadingProgress()
		} else {
			m.statusbar.SetMode("EDIT")
			m.statusbar.SetViewMode(false)
		}
	case CmdSettings:
		m.settings.SetConfig(m.config)
		m.settings.SetSize(m.width, m.height)
		m.settings.Toggle()
	case CmdFocusEditor:
		m.setFocus(focusEditor)
	case CmdFocusSidebar:
		m.setFocus(focusSidebar)
	case CmdFocusBacklinks:
		m.setFocus(focusBacklinks)
	case CmdRefreshVault:
		_ = m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Vault refreshed")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdDeleteNote:
		if m.activeNote != "" {
			if m.config.ConfirmDelete {
				m.confirmDelete = true
				m.confirmDeleteNote = m.activeNote
			} else {
				if err := m.trash.MoveToTrash(m.activeNote); err == nil {
					_ = m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.statusbar.SetMessage("Moved to trash: " + m.activeNote)
					if len(paths) > 0 {
						m.loadNote(paths[0])
					} else {
						m.activeNote = ""
						m.editor.SetContent("")
						m.statusbar.SetActiveNote("")
					}
				}
				return m, m.clearMessageAfter(2 * time.Second)
			}
		}
	case CmdShowTrash:
		m.trash.SetSize(m.width, m.height)
		m.trash.Open()
	case CmdRenameNote:
		if m.activeNote != "" {
			m.newNoteMode = true
			m.newNoteName = strings.TrimSuffix(m.activeNote, ".md")
		}
	case CmdShowGraph:
		m.graphView.SetSize(m.width, m.height)
		m.graphView.Open(m.activeNote)
	case CmdShowTags:
		m.tagBrowser.SetSize(m.width, m.height)
		m.tagBrowser.Open()
	case CmdShowHelp:
		m.helpOverlay.SetSize(m.width, m.height)
		m.helpOverlay.Toggle()
	case CmdShowOutline:
		m.outline.SetSize(m.width, m.height)
		m.outline.Open(m.editor.GetContent())
	case CmdShowBookmarks:
		m.bookmarks.SetSize(m.width, m.height)
		m.bookmarks.Open()
	case CmdToggleBookmark:
		if m.activeNote != "" {
			m.bookmarks.ToggleStar(m.activeNote)
			m.reportError("persist bookmarks", m.bookmarks.ConsumeSaveError())
			if m.bookmarks.IsStarred(m.activeNote) {
				m.statusbar.SetMessage("Starred " + m.activeNote)
			} else {
				m.statusbar.SetMessage("Unstarred " + m.activeNote)
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdFindInFile:
		m.findReplace.SetSize(m.width, m.height)
		m.findReplace.OpenFind(m.vault.Root)
		m.findReplace.UpdateMatches(m.editor.content)
	case CmdReplaceInFile:
		m.findReplace.SetSize(m.width, m.height)
		m.findReplace.OpenReplace(m.vault.Root)
		m.findReplace.UpdateMatches(m.editor.content)
	case CmdShowStats:
		m.vaultStats.SetSize(m.width, m.height)
		m.vaultStats.Open()
	case CmdNewFromTemplate:
		m.templates.SetSize(m.width, m.height)
		m.templates.OpenWithVault(m.vault.Root)
	case CmdFocusMode:
		if m.focusMode.IsActive() {
			m.focusMode.Close()
		} else {
			m.focusMode.SetSize(m.width, m.height)
			m.focusMode.Open(m.editor.GetWordCount())
			m.setFocus(focusEditor)
		}
	case CmdQuickSwitch:
		m.quickSwitch.SetSize(m.width, m.height)
		m.quickSwitch.Open(
			m.bookmarks.data.Recent,
			m.bookmarks.data.Starred,
			m.vault.SortedPaths(),
			func(path string) time.Time {
				if note := m.vault.GetNote(path); note != nil {
					return note.ModTime
				}
				return time.Time{}
			},
		)
	case CmdShowCanvas:
		m.canvas.SetSize(m.width, m.height)
		m.canvas.Open()
	case CmdShowCalendar:
		m.calendar.SetSize(m.width, m.height)
		m.calendar.SetVaultRoot(m.vault.Root)
		m.calendar.SetActiveGoals(loadActiveGoals(m.vault.Root))
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.calendar.SetNoteContents(noteContents)
		plannerBlocks, dailyFocus := loadPlannerBlocks(m.vault.Root)
		m.calendar.SetPlannerBlocks(plannerBlocks)
		m.calendar.SetAllDailyFocus(dailyFocus)
		m.loadCalendarEvents()
		m.refreshCalendarPanel()
		// Load habit data for calendar views
		ht := NewHabitTracker()
		ht.Open(m.vault.Root)
		m.calendar.SetHabitData(ht.habits, ht.logs)
		m.calendar.Open()
	case CmdShowBots:
		m.bots.SetSize(m.width, m.height)
		m.bots.SetAIConfig(m.aiConfig())
		m.bots.SetVaultRoot(m.vault.Root)
		noteContents := make(map[string]string)
		tagMap := make(map[string][]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
				if tags, ok := note.Frontmatter["tags"]; ok {
					if tagList, ok := tags.([]interface{}); ok {
						for _, t := range tagList {
							if s, ok := t.(string); ok {
								tagMap[s] = append(tagMap[s], p)
							}
						}
					}
				}
			}
		}
		m.bots.SetVaultData(noteContents, tagMap)
		m.bots.SetCurrentNote(m.activeNote, m.editor.GetContent())
		m.bots.Open()
	case CmdNewFolder:
		m.newFolderMode = true
		m.newFolderName = ""
	case CmdMoveFile:
		if m.activeNote != "" {
			m.moveFileMode = true
			m.moveFileCursor = 0
			m.moveFileDirs = m.getVaultDirs()
		}
	case CmdExportNote:
		m.export.SetSize(m.width, m.height)
		m.export.Open(m.activeNote, m.editor.GetContent(), m.vault.Root)
	case CmdGitOverlay:
		m.git.SetSize(m.width, m.height)
		return m, m.git.Open(m.vault.Root)
	case CmdStatusTray:
		m.statusTray.SetSize(m.width, m.height)
		m.statusTray.Open(m.statusbar, m.research.IsRunning(), m.research.StatusText())
	case CmdPluginManager:
		m.plugins.SetSize(m.width, m.height)
		m.plugins.Open()
	case CmdContentSearch:
		m.contentSearch.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.contentSearch.Open(noteContents, m.vault.SearchIndex, m.vault.Root, m.config.SearchContentByDefault, m.config.MaxSearchResults)
	case CmdGlobalReplace:
		m.globalReplace.SetSize(m.width, m.height)
		m.globalReplace.Open(m.vault)
	case CmdToggleRegex:
		// Toggle regex mode on whichever search overlay is currently active
		if m.contentSearch.IsActive() {
			m.contentSearch.ToggleRegex()
		} else if m.findReplace.IsActive() {
			m.findReplace.ToggleRegex()
			m.findReplace.UpdateMatches(m.editor.content)
		} else if m.globalReplace.IsActive() {
			m.globalReplace.ToggleRegex()
		} else {
			m.statusbar.SetWarning("Open a search overlay first, then toggle regex with Alt+R")
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdSpellCheck:
		if m.spellcheck.IsAvailable() {
			m.spellcheck.SetSize(m.width, m.height)
			m.spellcheck.Open(m.editor.GetContent())
		} else {
			m.statusbar.SetWarning("Spell check unavailable (install aspell/hunspell or /usr/share/dict/words)")
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdToggleSpellCheck:
		m.config.SpellCheck = !m.config.SpellCheck
		m.spellcheck.SetInlineEnabled(m.config.SpellCheck)
		if m.config.SpellCheck {
			m.statusbar.SetMessage("Inline spell check enabled (" + m.spellcheck.BackendName() + ")")
			// Trigger an immediate check
			if m.activeNote != "" && m.spellcheck.IsAvailable() {
				now := time.Now()
				m.lastSpellEditTime = now
				return m, tea.Batch(m.clearMessageAfter(2*time.Second), ScheduleInlineCheck(now))
			}
		} else {
			m.editor.SetSpellPositions(nil)
			m.statusbar.SetMessage("Inline spell check disabled")
		}
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdPublishSite:
		m.publisher.SetSize(m.width, m.height)
		m.publisher.Open()
	case CmdBlogPublish:
		if !m.registry.Enabled("blog_publisher") {
			break
		}
		if m.activeNote != "" {
			note := m.vault.GetNote(m.activeNote)
			title := strings.TrimSuffix(filepath.Base(m.activeNote), ".md")
			content := ""
			if note != nil {
				content = note.Content
			}
			m.blogPublisher.SetSize(m.width, m.height)
			m.blogPublisher.PreFill(
				m.config.MediumToken,
				m.config.GitHubToken,
				m.config.GitHubRepo,
				m.config.GitHubBranch,
			)
			m.blogPublisher.SetConfigSave(func(target, mediumToken, ghToken, ghRepo, ghBranch string) {
				if target == "medium" {
					m.config.MediumToken = mediumToken
				} else {
					m.config.GitHubToken = ghToken
					m.config.GitHubRepo = ghRepo
					m.config.GitHubBranch = ghBranch
				}
				_ = m.config.Save()
			})
			m.blogPublisher.Open(title, content)
		} else {
			m.statusbar.SetWarning("Open a note first to publish")
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdSplitPane:
		m.splitPane.SetSize(m.width, m.height)
		m.splitPane.SetNotes(m.vault.SortedPaths())
		m.splitPane.Open()
		// Load current note into the left pane; right pane opens the picker
		if m.activeNote != "" {
			content := strings.Split(m.editor.GetContent(), "\n")
			m.splitPane.SetLeftContent(m.activeNote, content)
		}
	case CmdRunLuaScript:
		m.luaOverlay.SetSize(m.width, m.height)
		m.luaOverlay.Open(m.activeNote, m.editor.GetContent(), nil)
	case CmdFlashcards:
		if !m.registry.Enabled("flashcards") {
			break
		}
		m.flashcards.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.flashcards.LoadCards(noteContents)
		m.flashcards.Open()
	case CmdQuizMode:
		if !m.registry.Enabled("quiz_mode") {
			break
		}
		m.quizMode.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.quizMode.SetNoteContents(noteContents)
		m.quizMode.Open()
	case CmdLearnDashboard:
		m.learnDash.SetSize(m.width, m.height)
		// Update mastery stats from flashcards
		fc := &m.flashcards
		total, due, newC, mastered := fc.GetStats()
		m.learnDash.SetCardStats(total, due, newC, mastered)
		m.learnDash.Open()
	case CmdAIChat:
		m.aiChat.SetSize(m.width, m.height)
		m.aiChat.ai = m.aiConfig()
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.aiChat.SetNotes(noteContents)
		m.aiChat.Open()
	case CmdComposer:
		m.composer.SetSize(m.width, m.height)
		m.composer.ai = m.aiConfig()
		m.composer.SetExistingNotes(m.vault.SortedPaths())
		composerContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				composerContents[p] = note.Content
			}
		}
		m.composer.SetNoteContents(composerContents)
		m.composer.Open()
	case CmdKnowledgeGraph:
		m.knowledgeGraph.SetSize(m.width, m.height)
		allNotes := m.vault.SortedPaths()
		noteLinks := make(map[string][]string)
		backlinks := make(map[string][]string)
		for _, p := range allNotes {
			noteLinks[p] = m.index.GetOutgoingLinks(p)
			backlinks[p] = m.index.GetBacklinks(p)
		}
		m.knowledgeGraph.SetGraphData(allNotes, noteLinks, backlinks)
		m.knowledgeGraph.Open()
	case CmdAutoLink:
		if m.activeNote != "" {
			m.autoLinker.SetNotes(m.vault.SortedPaths())
			suggestions := m.autoLinker.FindUnlinkedMentions(m.editor.GetContent(), m.activeNote)
			if len(suggestions) > 0 {
				m.reportInfo("Found %d unlinked mentions", len(suggestions))
				// Apply first suggestion as demo, or show count
			} else {
				m.statusbar.SetMessage("No unlinked mentions found")
			}
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdSimilarNotes:
		if m.activeNote != "" {
			// Build TF-IDF index if needed (rebuild when dirty or first time)
			if m.tfidfIndex == nil || m.tfidfDirty {
				noteContents := make(map[string]string)
				for _, p := range m.vault.SortedPaths() {
					if note := m.vault.GetNote(p); note != nil {
						noteContents[p] = note.Content
					}
				}
				m.tfidfIndex = BuildTFIDF(noteContents)
				m.tfidfDirty = false
			}
			similar := FindSimilar(m.tfidfIndex, m.activeNote, 5)
			if len(similar) > 0 {
				names := make([]string, len(similar))
				for i, s := range similar {
					names[i] = fmt.Sprintf("%s (%.0f%%)", strings.TrimSuffix(filepath.Base(s.Path), ".md"), s.Score*100)
				}
				m.statusbar.SetMessage("Similar: " + strings.Join(names, ", "))
			} else {
				m.statusbar.SetMessage("No similar notes found")
			}
			return m, m.clearMessageAfter(5 * time.Second)
		}
	case CmdTableEditor:
		if m.activeNote != "" {
			m.tableEditor.SetSize(m.width, m.height)
			m.tableEditor.Open(m.editor.content, m.editor.cursor)
			if !m.tableEditor.IsActive() {
				m.tableEditor.OpenNew(m.editor.cursor)
			}
		}
	case CmdSemanticSearch:
		if m.config.AIProvider == "local" {
			m.statusbar.SetMessage("Semantic search requires Ollama or OpenAI — configure in Settings (Ctrl+,)")
			return m, m.clearMessageAfter(4 * time.Second)
		}
		m.semanticSearch.SetSize(m.width, m.height)
		cfg := m.aiConfig()
		m.semanticSearch.SetConfig(cfg)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.semanticSearch.SetNotes(noteContents)
		m.semanticSearch.Open()
		// Auto-build if enabled and index needs updating.
		if m.config.SemanticSearchEnabled && m.semanticSearch.needsRebuild() {
			m.semanticSearch.building = true
			m.semanticSearch.buildProgress = 0
			m.semanticSearch.buildTotal = len(noteContents)
			m.semanticSearch.loadingTick = 0
			return m, tea.Batch(m.semanticSearch.startBuild(), semanticTick())
		}
	case CmdThreadWeaver:
		m.threadWeaver.SetSize(m.width, m.height)
		m.threadWeaver.ai = m.aiConfig()
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.threadWeaver.SetNotes(m.vault.SortedPaths(), noteContents)
		m.threadWeaver.Open()
	case CmdNoteChat:
		if m.activeNote != "" {
			m.noteChat.SetSize(m.width, m.height)
			m.noteChat.ai = m.aiConfig()
			m.noteChat.Open(m.activeNote, m.editor.GetContent())
		}
	case CmdToggleGhostWriter:
		if m.ghostWriter != nil {
			m.ghostWriter.SetEnabled(!m.ghostWriter.IsEnabled())
			if m.ghostWriter.IsEnabled() {
				m.ghostWriter.SetAI(m.aiConfig())
				m.statusbar.SetMessage("Ghost Writer enabled")
			} else {
				m.statusbar.SetMessage("Ghost Writer disabled")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdClockIn:
		if m.clockIn.IsActive() {
			m.statusbar.SetMessage("Already clocked in — clock out first")
			return m, m.clearMessageAfter(2 * time.Second)
		}
		project := ""
		if m.activeNote != "" {
			// Use current note name as project context
			project = m.activeNote
		}
		cmd := m.clockIn.ClockInCmd(project)
		if err := m.clockIn.ConsumeSaveError(); err != nil {
			m.reportError("save clock-in", err)
		} else {
			m.statusbar.SetMessage("Clocked in")
		}
		return m, tea.Batch(cmd, m.clearMessageAfter(2*time.Second))
	case CmdClockOut:
		if !m.clockIn.IsActive() {
			m.statusbar.SetMessage("Not clocked in")
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.clockIn.ClockOutCmd()
		if err := m.clockIn.ConsumeSaveError(); err != nil {
			m.reportError("save clock-out", err)
		} else {
			m.statusbar.SetMessage("Clocked out — session saved to Timetracking/")
		}
		return m, m.clearMessageAfter(3 * time.Second)
	case CmdPomodoro:
		if !m.registry.Enabled("pomodoro") {
			break
		}
		m.pomodoro.SetSize(m.width, m.height)
		m.pomodoro.SetVaultRoot(m.vault.Root)
		m.pomodoro.Open()
	case CmdPomodoroNow:
		if !m.registry.Enabled("pomodoro") {
			break
		}
		m.pomodoro.SetSize(m.width, m.height)
		m.pomodoro.SetVaultRoot(m.vault.Root)
		m.pomodoro.Open()
		alreadyInWork := m.pomodoro.IsInWork()
		switch task := m.pomodoro.StartForCurrentBlock(m.vault.Root); {
		case task != "" && alreadyInWork:
			m.statusbar.SetMessage("⏸ Pomodoro already running for: " + task)
		case task != "":
			m.statusbar.SetMessage("▶ Pomodoro started for: " + task)
		default:
			m.statusbar.SetMessage("No block scheduled right now — queue is empty")
		}
		return m, m.clearMessageAfter(3 * time.Second)
	case CmdWebClip:
		m.webClipper.SetSize(m.width, m.height)
		// Prompt user for URL — for now open with empty URL (they type in overlay)
		m.webClipper.Open("")
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return webClipTickMsg{}
		})
	case CmdToggleVim:
		if m.vimState != nil {
			m.vimState.SetEnabled(!m.vimState.IsEnabled())
			m.config.VimMode = m.vimState.IsEnabled()
			_ = m.config.Save()
			if m.vimState.IsEnabled() {
				m.statusbar.SetMode("VIM:NORMAL")
				m.statusbar.SetMessage("Vim mode enabled")
			} else {
				m.statusbar.SetMode("EDIT")
				m.statusbar.SetMessage("Vim mode disabled")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdToggleWordWrap:
		m.config.WordWrap = !m.config.WordWrap
		m.editor.SetWordWrap(m.config.WordWrap)
		_ = m.config.Save()
		if m.config.WordWrap {
			m.statusbar.SetMessage("Word wrap enabled")
		} else {
			m.statusbar.SetMessage("Word wrap disabled")
		}
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdMacroRecord:
		if m.vimState != nil && m.vimState.IsEnabled() {
			if m.vimState.IsRecording() {
				m.vimState.StopRecording()
				m.statusbar.SetMode("VIM:" + m.vimState.ModeString())
				m.statusbar.SetMessage("Macro recording stopped")
			} else {
				m.vimState.StartRecording('a')
				mode := "VIM:" + m.vimState.ModeString() + " [" + m.vimState.RecordingStatus() + "]"
				m.statusbar.SetMode(mode)
				m.statusbar.SetMessage("Recording macro into @a (press q to stop)")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.statusbar.SetWarning("Vim mode must be enabled for macros")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdMacroPlay:
		if m.vimState != nil && m.vimState.IsEnabled() {
			reg := m.vimState.LastMacroRegister()
			if reg == 0 {
				reg = 'a'
			}
			keys := m.vimState.GetMacro(reg)
			if len(keys) > 0 {
				m.vimState.SetLastMacroRegister(reg)
				m.vimState.SetPlayingMacro(true)
				return m, func() tea.Msg {
					return vimMacroReplayMsg{keys: keys, idx: 0}
				}
			}
			m.statusbar.SetMessage("No macro recorded in @" + string(reg))
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.statusbar.SetWarning("Vim mode must be enabled for macros")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdPinNote:
		if m.activeNote != "" && m.breadcrumb != nil {
			m.breadcrumb.Pin(m.activeNote)
			m.statusbar.SetMessage("Pinned: " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdUnpinNote:
		if m.activeNote != "" && m.breadcrumb != nil {
			m.breadcrumb.Unpin(m.activeNote)
			m.statusbar.SetMessage("Unpinned: " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdNavBack:
		if m.breadcrumb != nil {
			if nav := m.breadcrumb.Back(); nav != "" {
				m.loadNoteWithoutBreadcrumb(nav)
				m.setSidebarCursorToFile(nav)
			}
		}
	case CmdNavForward:
		if m.breadcrumb != nil {
			if nav := m.breadcrumb.Forward(); nav != "" {
				m.loadNoteWithoutBreadcrumb(nav)
				m.setSidebarCursorToFile(nav)
			}
		}
	case CmdKanban:
		m.kanban.SetSize(m.width, m.height)
		if len(m.config.KanbanColumns) > 0 {
			m.kanban.Configure(m.config.KanbanColumns, m.config.KanbanColumnTags)
		}
		// Load saved state BEFORE distributing cards so positions are restored
		m.kanban.Open(m.vault.Root)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.kanban.SetTasks(noteContents)
		// Enrich kanban cards with project info
		allTasks := m.currentTasks()
		pm := NewProjectMode()
		pm.vaultRoot = m.vault.Root
		pm.loadProjects()
		MatchTasksToProjects(allTasks, pm.projects)
		m.kanban.SetTaskProjects(allTasks)
	case CmdZettelNote:
		if m.zettelkasten != nil {
			name := m.zettelkasten.GenerateNoteName("Untitled")
			title := strings.TrimSuffix(name, ".md")
			content := m.zettelkasten.GenerateTemplate(title)
			path := filepath.Join(m.vault.Root, name)
			if err := atomicWriteNote(path, content); err == nil {
				_ = m.vault.Scan()
				m.index = vault.NewIndex(m.vault)
				m.index.Build()
				paths := m.vault.SortedPaths()
				m.sidebar.SetFiles(paths)
				m.autocomplete.SetNotes(paths)
				m.statusbar.SetNoteCount(m.vault.NoteCount())
				m.loadNote(name)
				m.setSidebarCursorToFile(name)
				m.setFocus(focusEditor)
				m.statusbar.SetMessage("Created Zettelkasten note: " + name)
				return m, m.clearMessageAfter(3 * time.Second)
			}
		}
	case CmdImportObsidian:
		imported := config.ImportObsidianConfig(m.vault.Root)
		if imported != nil {
			m.config = *imported
			_ = m.config.Save()
			ApplyTheme(m.config.Theme)
			m.syncConfigToComponents()
			report := config.ImportReport(m.vault.Root)
			m.statusbar.SetMessage(report)
		} else {
			m.statusbar.SetMessage("No .obsidian/ directory found")
		}
		return m, m.clearMessageAfter(5 * time.Second)
	case CmdVaultRefactor:
		m.vaultRefactor.SetSize(m.width, m.height)
		m.vaultRefactor.ai = m.aiConfig()
		noteContents := make(map[string]string)
		tagMap := make(map[string][]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
				if tags, ok := note.Frontmatter["tags"]; ok {
					if tagList, ok := tags.([]interface{}); ok {
						for _, t := range tagList {
							if s, ok := t.(string); ok {
								tagMap[s] = append(tagMap[s], p)
							}
						}
					}
				}
			}
		}
		m.vaultRefactor.SetVaultData(noteContents, tagMap, m.vault.SortedPaths())
		m.vaultRefactor.Open()

	case CmdDailyBriefing:
		m.dailyBriefing.SetSize(m.width, m.height)
		m.dailyBriefing.ai = m.aiConfig()
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		today := time.Now().Format("2006-01-02")
		todayPath := today + ".md"
		if m.config.DailyNotesFolder != "" {
			todayPath = filepath.Join(m.config.DailyNotesFolder, todayPath)
		}
		m.dailyBriefing.SetVaultData(noteContents, m.vault.SortedPaths(), todayPath)
		m.dailyBriefing.Open()

	case CmdDevotional:
		m.devotional.SetSize(m.width, m.height)
		goals := m.goalsMode.GetGoals()
		cmd := m.devotional.Open(m.vault.Root, m.aiConfig(), goals)
		return m, cmd

	case CmdEncryptNote:
		if m.activeNote != "" {
			m.encryption.SetSize(m.width, m.height)
			m.encryption.Open()
		}

	case CmdGitHistory:
		if m.activeNote != "" {
			m.gitHistory.SetSize(m.width, m.height)
			cmd := m.gitHistory.Open(m.activeNote, m.vault.Root)
			return m, cmd
		}

	case CmdWorkspaces:
		m.workspace.SetSize(m.width, m.height)
		m.workspace.Open()

	case CmdTimeline:
		m.timeline.SetSize(m.width, m.height)
		notes := make(map[string]TimelineEntry)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				title := strings.TrimSuffix(filepath.Base(p), ".md")
				var tags []string
				if t, ok := note.Frontmatter["tags"]; ok {
					if tagList, ok := t.([]interface{}); ok {
						for _, tg := range tagList {
							if s, ok := tg.(string); ok {
								tags = append(tags, s)
							}
						}
					}
				}
				preview := note.Content
				// Strip frontmatter for preview
				if strings.HasPrefix(preview, "---") {
					if idx := strings.Index(preview[3:], "---"); idx >= 0 {
						preview = strings.TrimSpace(preview[3+idx+3:])
					}
				}
				if len(preview) > 80 {
					preview = preview[:80]
				}
				// Remove newlines from preview
				preview = strings.ReplaceAll(preview, "\n", " ")
				notes[p] = TimelineEntry{
					Path:    p,
					Title:   title,
					Date:    note.ModTime,
					Tags:    tags,
					Preview: preview,
				}
			}
		}
		m.timeline.Open(notes)

	case CmdVaultSwitch:
		m.vaultSwitch.SetSize(m.width, m.height)
		m.vaultSwitch.Open()

	case CmdFrontmatterEdit:
		if m.activeNote != "" {
			m.frontmatterEdit.SetSize(m.width, m.height)
			m.frontmatterEdit.Open(m.editor.GetContent())
		}

	case CmdFoldToggle:
		if m.activeNote != "" {
			m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
		}

	case CmdFoldAll:
		if m.activeNote != "" {
			m.foldState.FoldAll(m.editor.content)
		}

	case CmdUnfoldAll:
		m.foldState.UnfoldAll()

	case CmdTaskManager:
		if !m.registry.Enabled("task_manager") {
			break
		}
		m.taskManager.SetSize(m.width, m.height)
		m.taskManager.config = m.config
		m.taskManager.ai = m.aiConfig()
		m.taskManager.Open(m.vault)
		// Enrich tasks with project associations
		pm := NewProjectMode()
		pm.vaultRoot = m.vault.Root
		pm.loadProjects()
		MatchTasksToProjects(m.taskManager.allTasks, pm.projects)
		// Enrich tasks with actual time from time tracker
		timeMap := m.timeTracker.TaskTimeMap()
		for i := range m.taskManager.allTasks {
			t := &m.taskManager.allTasks[i]
			if mins, ok := timeMap[tmCleanText(t.Text)]; ok {
				t.ActualMinutes = mins
			}
		}
		m.taskManager.rebuildFiltered()

	case CmdLinkAssist:
		if m.activeNote != "" {
			m.linkAssist.SetSize(m.width, m.height)
			m.linkAssist.Open(m.editor.GetContent(), m.vault.SortedPaths(), m.activeNote)
		}

	case CmdImageManager:
		m.imageManager.SetSize(m.width, m.height)
		m.imageManager.Open(m.vault.Root)

	case CmdThemeEditor:
		m.themeEditor.SetSize(m.width, m.height)
		m.themeEditor.Open(m.config.Theme)

	case CmdLayoutDefault, CmdLayoutWriter, CmdLayoutMinimal, CmdLayoutReading, CmdLayoutDashboard, CmdLayoutZen, CmdLayoutResearch, CmdLayoutCornell, CmdLayoutFocus, CmdLayoutCockpit, CmdLayoutStacked, CmdLayoutPreview, CmdLayoutPresenter, CmdLayoutKanban, CmdLayoutWidescreen:
		switch action {
		case CmdLayoutDefault:
			m.config.Layout = LayoutDefault
			m.statusbar.SetMessage("Layout: Default (3-panel)")
		case CmdLayoutWriter:
			m.config.Layout = LayoutWriter
			m.statusbar.SetMessage("Layout: Writer (2-panel)")
		case CmdLayoutMinimal:
			m.config.Layout = LayoutZen
			m.statusbar.SetMessage("Layout: Zen (distraction-free)")
		case CmdLayoutReading:
			m.config.Layout = LayoutReading
			m.statusbar.SetMessage("Layout: Reading (editor + backlinks)")
		case CmdLayoutDashboard:
			m.config.Layout = LayoutDashboard
			m.statusbar.SetMessage("Layout: Dashboard (4-panel)")
		case CmdLayoutZen:
			m.config.Layout = LayoutZen
			m.statusbar.SetMessage("Layout: Zen (distraction-free)")
		case CmdLayoutResearch:
			m.config.Layout = LayoutDefault
			m.statusbar.SetMessage("Layout: Default (3-panel)")
		case CmdLayoutCornell:
			m.config.Layout = LayoutCornell
			m.statusbar.SetMessage("Layout: Cornell (editor + notes study layout)")
		case CmdLayoutFocus:
			m.config.Layout = LayoutFocus
			m.statusbar.SetMessage("Layout: Focus (sidebar + wide centered editor)")
		case CmdLayoutCockpit:
			m.config.Layout = LayoutCockpit
			m.statusbar.SetMessage("Layout: Cockpit (sidebar + editor + calendar & tasks)")
		case CmdLayoutStacked:
			m.config.Layout = LayoutStacked
			m.statusbar.SetMessage("Layout: Stacked (editor over outline & backlinks)")
		case CmdLayoutPreview:
			m.config.Layout = LayoutPreview
			m.statusbar.SetMessage("Layout: Preview (editor + rendered)")
		case CmdLayoutPresenter:
			m.config.Layout = LayoutPresenter
			m.statusbar.SetMessage("Layout: Presenter (full-screen view)")
		case CmdLayoutKanban:
			m.config.Layout = LayoutKanban
			m.statusbar.SetMessage("Layout: Kanban (sidebar + editor + board)")
		case CmdLayoutWidescreen:
			m.config.Layout = LayoutWidescreen
			m.statusbar.SetMessage("Layout: Widescreen (5-panel ultra-wide)")
		}
		// Fix focus if current panel is hidden in new layout
		if !LayoutHasSidebar(m.config.Layout) && m.focus == focusSidebar {
			m.setFocus(focusEditor)
		}
		if !LayoutHasBacklinks(m.config.Layout) && m.focus == focusBacklinks {
			m.setFocus(focusEditor)
		}
		m.updateLayout()
		_ = m.config.Save()

	case CmdCycleLayout:
		layouts := AllLayouts()
		current := m.config.Layout
		if current == "" {
			current = LayoutDefault
		}
		nextIdx := 0
		for i, l := range layouts {
			if l == current {
				nextIdx = (i + 1) % len(layouts)
				break
			}
		}
		m.config.Layout = layouts[nextIdx]
		m.statusbar.SetMessage("Layout: " + LayoutDescription(m.config.Layout))
		// Safety: move focus if current panel is hidden
		if !LayoutHasSidebar(m.config.Layout) && m.focus == focusSidebar {
			m.setFocus(focusEditor)
		}
		if !LayoutHasBacklinks(m.config.Layout) && m.focus == focusBacklinks {
			m.setFocus(focusEditor)
		}
		m.updateLayout()
		_ = m.config.Save()

	case CmdLayoutPicker:
		m.layoutPicker.SetSize(m.width, m.height)
		m.layoutPicker.Open(m.config.Layout)

	case CmdToggleSidebar:
		if LayoutHasSidebar(m.config.Layout) {
			// Hide sidebar: switch to a layout without it
			switch m.config.Layout {
			case LayoutWriter:
				m.config.Layout = LayoutMinimal
			case LayoutDashboard:
				m.config.Layout = LayoutReading
			default:
				m.config.Layout = LayoutReading
			}
			if m.focus == focusSidebar {
				m.setFocus(focusEditor)
			}
			m.statusbar.SetMessage("Sidebar hidden")
		} else {
			// Show sidebar: switch to a layout with it
			switch m.config.Layout {
			case LayoutMinimal:
				m.config.Layout = LayoutWriter
			case LayoutReading:
				m.config.Layout = LayoutDefault
			default:
				m.config.Layout = LayoutDefault
			}
			m.statusbar.SetMessage("Sidebar shown")
		}
		m.updateLayout()
		_ = m.config.Save()

	case CmdResearchAgent:
		if !m.registry.Enabled("research_agent") {
			break
		}
		m.research.SetSize(m.width, m.height)
		if m.research.IsRunning() {
			// Reopen to show progress
			m.research.Reopen()
		} else {
			m.research.Open(m.vault.Root, m.vault.SortedPaths(), m.activeNote)
		}

	case CmdResearchFollowUp:
		if m.activeNote != "" && !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenFollowUp(m.vault.Root, m.activeNote, m.editor.GetContent(), m.vault.SortedPaths())
		}

	case CmdAITemplate:
		if !m.registry.Enabled("ai_templates") {
			break
		}
		m.aiTemplates.SetSize(m.width, m.height)
		m.aiTemplates.OpenWithAI(m.aiConfig())

	case CmdVaultAnalyzer:
		if !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenVaultAnalyzer(m.vault.Root, m.vault.SortedPaths())
		}

	case CmdNoteEnhancer:
		if m.activeNote != "" && !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenNoteEnhancer(m.vault.Root, m.activeNote, m.editor.GetContent(), m.vault.SortedPaths())
		}

	case CmdDailyDigest:
		if !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			recentNotes := make(map[string]string)
			cutoff := time.Now().AddDate(0, 0, -7)
			for _, p := range m.vault.SortedPaths() {
				note := m.vault.GetNote(p)
				if note != nil && note.ModTime.After(cutoff) {
					recentNotes[p] = note.Content
				}
			}
			m.research.OpenDailyDigest(m.vault.Root, recentNotes)
		}

	case CmdLanguageLearning:
		if m.registry.Enabled("language_learning") {
			m.languageLearning.SetSize(m.width, m.height)
			m.languageLearning.Open(m.vault.Root)
		}

	case CmdHabitTracker:
		if m.registry.Enabled("habit_tracker") {
			m.habitTracker.ai = m.aiConfig()
			m.habitTracker.SetSize(m.width, m.height)
			m.habitTracker.vault = m.vault
			m.habitTracker.dailyNotesFolder = m.config.DailyNotesFolder
			m.habitTracker.Open(m.vault.Root)
		}

	case CmdFocusSession:
		m.focusSession.SetSize(m.width, m.height)
		m.focusSession.Open(m.vault.Root)

	case CmdStandupGenerator:
		m.standupGen.SetSize(m.width, m.height)
		m.standupGen.Open(m.vault.Root)

	case CmdDailyReview:
		m.dailyReview.ai = m.aiConfig()
		m.dailyReview.SetSize(m.width, m.height)
		m.dailyReview.Open(m.vault.Root, m.vault, m.currentTasks())

	case CmdDailyJot:
		m.dailyJot.SetSize(m.width, m.height)
		noteNames := make([]string, 0, len(m.vault.Notes))
		for k := range m.vault.Notes {
			noteNames = append(noteNames, strings.TrimSuffix(filepath.Base(k), ".md"))
		}
		m.dailyJot.Open(m.vault.Root, "Jots", noteNames, 14)

	case CmdMorningRoutine:
		m.morningRoutine.SetSize(m.width, m.height)
		m.morningRoutine.dailyNotesFolder = m.config.DailyNotesFolder
		m.loadCalendarEvents() // ensure calendar data is fresh
		tasks, events, habits, projects, yesterdayTasks := m.gatherPlanMyDayData()
		goals := m.goalsMode.GetGoals()
		// Load existing planner blocks and daily focus so morning routine can see
		// what's already scheduled and pre-populate the daily goal.
		plannerBlocks, dailyFocus := loadPlannerBlocks(m.vault.Root)
		today := time.Now().Format("2006-01-02")
		todayBlocks := plannerBlocks[today]
		todayFocus := dailyFocus[today]
		cmd := m.morningRoutine.Open(m.vault.Root, goals, tasks, events, habits, projects, yesterdayTasks, todayBlocks, todayFocus)
		return m, cmd

	case CmdNoteHistory:
		m.noteHistory.SetSize(m.width, m.height)
		m.noteHistory.OpenForNote(m.vault.Root, m.activeNote)

	case CmdSmartConnections:
		m.smartConnect.SetSize(m.width, m.height)
		content := ""
		if n, ok := m.vault.Notes[m.activeNote]; ok {
			content = n.Content
		}
		m.smartConnect.OpenForNote(m.vault.Root, m.activeNote, content)

	case CmdWritingStats:
		m.writingStats.SetSize(m.width, m.height)
		m.writingStats.Open(m.vault.Root)

	case CmdQuickCapture:
		m.quickCapture.SetSize(m.width, m.height)
		m.quickCapture.Open(m.vault.Root)

	case CmdDashboard:
		// Phase 3: route Alt+H to the new Daily Hub when the
		// flag is on. Falls through to the legacy dashboard
		// otherwise so users on the old flag see no change.
		if m.config.UseProfiles && m.profileRegistry != nil {
			m.dailyHub.SetSize(m.width, m.height)
			m.dailyHub.Open(m.profileRegistry.Active())
		} else {
			m.dashboard.SetSize(m.width, m.height)
			m.dashboard.Open(m.vault.Root, m.projectMode.GetProjects(), m.goalsMode.GetGoals())
		}

	case CmdMindMap:
		m.mindMap.SetSize(m.width, m.height)
		content := m.editor.GetContent()
		m.mindMap.OpenForNote(m.vault.Root, m.activeNote, content)

	case CmdJournalPrompts:
		m.journalPrompts.SetSize(m.width, m.height)
		m.journalPrompts.Open(m.vault.Root)

	case CmdClipManager:
		// Load current system clipboard into clipboard manager
		if text, err := ClipboardPaste(); err == nil && text != "" {
			m.clipManager.AddClip(text, "(system clipboard)")
		}
		m.clipManager.SetSize(m.width, m.height)
		m.clipManager.Open()

	case CmdSmartPaste:
		if m.viewMode {
			m.statusbar.SetMessage("Cannot paste in view mode")
		} else {
			text, err := ClipboardPaste()
			if err != nil {
				m.statusbar.SetMessage("Clipboard error: " + err.Error())
			} else if text != "" {
				if m.editor.HasSelection() {
					m.editor.DeleteSelection()
				}
				if m.editor.SmartPaste(text) {
					m.statusbar.SetMessage("Smart paste: created markdown link")
				} else {
					m.editor.InsertText(text)
					m.statusbar.SetMessage("Pasted from clipboard")
				}
				m.clipManager.AddClip(text, m.activeNote)
				line, col := m.editor.GetCursor()
				m.statusbar.SetCursor(line, col)
				m.statusbar.SetWordCount(m.editor.GetWordCount())
			}
		}

	case CmdDailyPlanner:
		m.dailyPlanner.SetSize(m.width, m.height)
		m.loadCalendarEvents()
		tasks, events, habits := m.gatherPlannerData()
		m.dailyPlanner.Open(m.vault.Root, tasks, events, habits)
		// Load active goals for display
		gm := NewGoalsMode()
		gm.vaultRoot = m.vault.Root
		gm.loadGoals()
		var activeGoals []Goal
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive {
				activeGoals = append(activeGoals, g)
			}
		}
		m.dailyPlanner.activeGoals = activeGoals

	case CmdPlanMyDay:
		m.planMyDay.SetSize(m.width, m.height)
		m.loadCalendarEvents() // ensure calendar data is fresh
		tasks, events, habits, projects, yesterdayTasks := m.gatherPlanMyDayData()
		m.planMyDay.SetClockedSessions(m.clockIn.SessionsForPlan())
		// Load active goals for AI context
		gm := NewGoalsMode()
		gm.vaultRoot = m.vault.Root
		gm.loadGoals()
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive {
				m.planMyDay.goals = append(m.planMyDay.goals, g)
			}
		}
		cmd := m.planMyDay.Open(m.vault.Root, tasks, events, habits, projects, yesterdayTasks,
			m.aiConfig())
		return m, cmd

	case CmdAIProjectPlanner:
		m.aiProjectPlanner.SetSize(m.width, m.height)
		titles := make([]string, 0, len(m.vault.Notes))
		for k := range m.vault.Notes {
			titles = append(titles, strings.TrimSuffix(filepath.Base(k), ".md"))
		}
		m.aiProjectPlanner.Open(m.vault.Root, titles,
			m.aiConfig(), m.projectMode.GetProjects(), m.goalsMode.GetGoals())

	case CmdGoalsMode:
		m.goalsMode.SetSize(m.width, m.height)
		m.goalsMode.ai = m.aiConfig()
		allTasks := m.currentTasks()
		m.goalsMode.Open(m.vault.Root, allTasks)

	case CmdIdeasBoard:
		m.ideasBoard.SetSize(m.width, m.height)
		m.ideasBoard.Open(m.vault.Root)

	case CmdUniversalSearch:
		m.universalSearch.SetSize(m.width, m.height)
		allTasks := m.currentTasks()
		gm := NewGoalsMode()
		gm.vaultRoot = m.vault.Root
		gm.loadGoals()
		var habits []habitEntry
		if m.habitTracker.habits != nil {
			habits = m.habitTracker.habits
		}
		m.universalSearch.Open(m.vault.Notes, allTasks, gm.goals, habits)

	case CmdCopyDailyPlan:
		// Build a plan summary using the planner's data without opening it
		dp := NewDailyPlanner()
		dp.SetSize(m.width, m.height)
		tasks, events, habits := m.gatherPlannerData()
		dp.Open(m.vault.Root, tasks, events, habits)
		summary := dp.buildPlanSummary()
		// Also append active goals
		gm := NewGoalsMode()
		gm.vaultRoot = m.vault.Root
		gm.loadGoals()
		var goalLines []string
		for _, g := range gm.goals {
			if g.Status == GoalStatusActive {
				line := "  " + g.Title
				if len(g.Milestones) > 0 {
					line += fmt.Sprintf(" (%d%%)", g.Progress())
				}
				goalLines = append(goalLines, line)
			}
		}
		if len(goalLines) > 0 {
			summary += "\nGoals:\n" + strings.Join(goalLines, "\n") + "\n"
		}
		_ = ClipboardCopy(summary)
		m.toast.Show("Daily plan copied to clipboard!", ToastSuccess)

	case CmdProjectDashboard:
		m.projectDashboard.SetSize(m.width, m.height)
		m.projectDashboard.Open(m.vault.Root, m.vault, m.currentTasks())

	case CmdRecurringTasks:
		m.recurringTasks.SetSize(m.width, m.height)
		m.recurringTasks.Open(m.vault.Root)

	case CmdNotePreview:
		if m.activeNote != "" {
			if n, ok := m.vault.Notes[m.activeNote]; ok {
				m.notePreview.SetSize(m.width, m.height)
				m.notePreview.Open(m.activeNote, m.activeNote, n.Content)
			}
		}

	case CmdScratchpad:
		m.scratchpad.SetSize(m.width, m.height)
		m.scratchpad.Open(m.vault.Root)

	case CmdProjectMode:
		m.projectMode.ai = m.aiConfig()
		m.projectMode.SetSize(m.width, m.height)
		m.projectMode.Open(m.vault.Root)

	case CmdCommandCenter:
		m.commandCenter.SetSize(m.width, m.height)
		// Gather data from all productivity systems.
		allTasks := m.currentTasks()
		// Load projects and match tasks to projects.
		pm := NewProjectMode()
		pm.Open(m.vault.Root)
		pm.Close()
		var projects []Project
		projects = append(projects, pm.projects...)
		MatchTasksToProjects(allTasks, projects)
		// Compute task counts for each project.
		for i := range projects {
			projects[i].ComputeTaskCounts(allTasks)
		}
		// Load habits.
		ht := NewHabitTracker()
		ht.Open(m.vault.Root)
		ht.Close()
		// Gather calendar events for today.
		_, plannerEvents, _ := m.gatherPlannerData()
		m.commandCenter.LoadData(allTasks, projects, ht.habits, ht.logs, plannerEvents)
		m.commandCenter.Open()

	case CmdNLSearch:
		m.nlSearch.SetSize(m.width, m.height)
		m.nlSearch.Open(m.vault.Root, m.aiConfig())

	case CmdWritingCoach:
		m.writingCoach.SetSize(m.width, m.height)
		content := m.editor.GetContent()
		m.writingCoach.Open(m.vault.Root, content, m.activeNote, m.aiConfig())

	case CmdDataview:
		m.dataview.SetSize(m.width, m.height)
		m.dataview.Open(m.vault)

	case CmdTimeTracker:
		m.timeTracker.SetSize(m.width, m.height)
		m.timeTracker.Open(m.vault.Root)

	case CmdBackup:
		m.backup.SetSize(m.width, m.height)
		m.backup.Open(m.vault.Root)

	case CmdKnowledgeGaps:
		m.knowledgeGaps.SetSize(m.width, m.height)
		m.knowledgeGaps.Open(m.vault.Root)

	case CmdShowTutorial:
		m.tutorial.SetSize(m.width, m.height)
		m.tutorial.Open()

	case CmdCloseTab:
		if m.tabBar != nil {
			next := m.tabBar.CloseActive()
			if next != "" {
				m.loadNote(next)
				m.setSidebarCursorToFile(next)
			} else {
				m.activeNote = ""
				m.editor.SetContent("")
				m.editor.filePath = ""
				m.statusbar.SetActiveNote("")
			}
			return m, nil
		}

	case CmdCloseAllTabs:
		if m.tabBar != nil {
			m.tabBar.CloseAll()
			m.activeNote = ""
			m.editor.SetContent("")
			m.editor.filePath = ""
			m.statusbar.SetActiveNote("")
			return m, nil
		}

	case CmdCloseOtherTabs:
		if m.tabBar != nil {
			m.tabBar.CloseOthers()
			m.statusbar.SetMessage("Closed other tabs")
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdCloseTabsToRight:
		if m.tabBar != nil {
			m.tabBar.CloseToRight()
			m.statusbar.SetMessage("Closed tabs to the right")
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdTogglePinTab:
		if m.tabBar != nil && m.activeNote != "" {
			m.tabBar.TogglePin()
			if m.tabBar.IsActiveTabPinned() {
				m.statusbar.SetMessage("Pinned tab: " + m.activeNote)
			} else {
				m.statusbar.SetMessage("Unpinned tab: " + m.activeNote)
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdReopenClosedTab:
		if m.tabBar != nil {
			if path := m.tabBar.ReopenLast(); path != "" {
				if m.vault.GetNote(path) != nil {
					m.tabBar.AddTab(path)
					m.loadNote(path)
					m.setSidebarCursorToFile(path)
					m.statusbar.SetMessage("Reopened: " + path)
					return m, m.clearMessageAfter(2 * time.Second)
				}
			} else {
				m.statusbar.SetMessage("No closed tabs to reopen")
				return m, m.clearMessageAfter(2 * time.Second)
			}
		}

	case CmdExtractToNote:
		sel := m.editor.GetSelectedText()
		if sel == "" {
			m.statusbar.SetMessage("No text selected")
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.extractMode = true
		m.extractName = ""

	case CmdNextcloudSync:
		m.nextcloudOverlay.SetSize(m.width, m.height)
		m.nextcloudOverlay.Open(m.config, m.vault.Root)

	case CmdNousStatus:
		if m.config.AIProvider != "nous" {
			m.statusbar.SetMessage("Nous is not the active AI provider. Set AI Provider to 'nous' in Settings.")
			return m, m.clearMessageAfter(3 * time.Second)
		}
		client := NewNousClient(m.config.NousURL, m.config.NousAPIKey)
		if err := client.TestConnection(); err != nil {
			m.statusbar.SetMessage("Nous: " + err.Error())
			return m, m.clearMessageAfter(3 * time.Second)
		}
		status, err := client.GetStatus()
		if err != nil {
			m.statusbar.SetMessage("Nous connected but status unavailable: " + err.Error())
		} else {
			m.statusbar.SetMessage("Nous: " + status)
		}
		return m, m.clearMessageAfter(5 * time.Second)

	case CmdWeeklyReview:
		m.weeklyReview.ai = m.aiConfig()
		m.weeklyReview.SetSize(m.width, m.height)
		m.weeklyReview.Open(m.vault.Root, m.vault, m.currentTasks())

	case CmdReadingList:
		m.readingList.SetSize(m.width, m.height)
		m.readingList.Open(m.vault.Root)

	case CmdBlogDraft:
		m.blogDraft.SetSize(m.width, m.height)
		m.blogDraft.Open(m.vault.Root, m.aiConfig())

	case CmdTaskTriage:
		m.taskTriage.SetSize(m.width, m.height)
		allTasks := m.currentTasks()
		var activeGoals []Goal
		for _, g := range m.goalsMode.goals {
			if g.Status == GoalStatusActive {
				activeGoals = append(activeGoals, g)
			}
		}
		cmd := m.taskTriage.Open(m.vault.Root, allTasks, activeGoals, m.aiConfig())
		return m, cmd

	case CmdQuit:
		return m, m.triggerExitSplash()
	}
	return m, nil
}

// gatherPlannerData collects tasks, events, and habits for the daily planner.
func (m *Model) gatherPlannerData() ([]PlannerTask, []PlannerEvent, []PlannerHabit) {
	today := time.Now().Format("2006-01-02")
	var tasks []PlannerTask
	var events []PlannerEvent
	var habits []PlannerHabit

	// Scan all vault notes for tasks due today, overdue, or from today's daily note
	for _, p := range m.vault.SortedPaths() {
		note := m.vault.GetNote(p)
		if note == nil {
			continue
		}
		// Check if this is today's daily note (tasks without dates should be included)
		base := strings.TrimSuffix(filepath.Base(p), ".md")
		isTodayNote := base == today
		for lineNum, line := range strings.Split(note.Content, "\n") {
			if !tmTaskRe.MatchString(line) {
				continue
			}
			m2 := tmTaskRe.FindStringSubmatch(line)
			done := m2[2] != " "
			// m2[3] is "] task text" — strip leading "] "
			text := m2[3]
			if len(text) > 2 {
				text = text[2:]
			}
			dueDate := ""
			if dm := tmDueDateRe.FindStringSubmatch(text); dm != nil {
				dueDate = dm[1]
			}
			isDueToday := dueDate == today
			isOverdue := dueDate != "" && dueDate < today && !done
			isFromTodayNote := isTodayNote && dueDate == ""
			if isDueToday || isOverdue || isFromTodayNote {
				tasks = append(tasks, PlannerTask{
					Text:     text,
					Done:     done,
					Priority: taskPriority(text),
					DueDate:  dueDate,
					Source:   filepath.Base(p),
					NotePath: p,
					LineNum:  lineNum,
				})
			}
		}
	}

	// Calendar events — use shared helper for today's events (handles multi-day)
	events = gatherTodayEvents(m.calendar.GetEvents())

	// Scan habits with completion status — use app's tracker, refreshed from disk
	m.habitTracker.vaultRoot = m.vault.Root
	m.habitTracker.loadHabits()
	todayCompleted := make(map[string]bool)
	for _, log := range m.habitTracker.logs {
		if log.Date == today {
			for _, name := range log.Completed {
				todayCompleted[name] = true
			}
		}
	}
	for _, h := range m.habitTracker.habits {
		habits = append(habits, PlannerHabit{
			Name:   h.Name,
			Done:   todayCompleted[h.Name],
			Streak: h.Streak,
		})
	}

	return tasks, events, habits
}

// gatherPlanMyDayData collects all data needed by the Plan My Day overlay.
func (m *Model) gatherPlanMyDayData() ([]Task, []PlannerEvent, []habitEntry, []Project, []string) {
	// Use the canonical task parser — same as TaskManager uses.
	// Refresh the cache so we have the latest from disk.
	m.cachedTasks = m.currentTasks()
	tasks := m.cachedTasks

	// Calendar events — use shared helper (handles multi-day events)
	events := gatherTodayEvents(m.calendar.GetEvents())

	// Habits — use app's tracker, refreshed from disk
	m.habitTracker.vaultRoot = m.vault.Root
	m.habitTracker.loadHabits()
	habits := m.habitTracker.habits

	// Projects — use app's project mode, refreshed from disk
	m.projectMode.vaultRoot = m.vault.Root
	m.projectMode.loadProjects()
	var projects []Project
	for _, proj := range m.projectMode.projects {
		if proj.Status == "active" {
			projects = append(projects, proj)
		}
	}

	// Yesterday's incomplete tasks
	yesterdayTasks := m.yesterdayIncompleteTasks()

	return tasks, events, habits, projects, yesterdayTasks
}

// writePlanMyDayToDailyNote writes the AI-generated day plan to today's daily note.
func (m *Model) writePlanMyDayToDailyNote(schedule []daySlot, topGoal string, focusOrder []string, advice string) {
	today := time.Now().Format("2006-01-02")
	dailyName := today + ".md"
	folder := m.config.DailyNotesFolder
	if folder != "" {
		dailyName = filepath.Join(folder, dailyName)
	}
	dailyPath := filepath.Join(m.vault.Root, dailyName)

	planContent := FormatDayPlanMarkdown(schedule, topGoal, focusOrder, advice)

	existing, err := os.ReadFile(dailyPath)
	var writeErr error
	if err != nil {
		// Create new daily note with plan
		if err := os.MkdirAll(filepath.Dir(dailyPath), 0755); err != nil {
			m.reportError("create daily note folder", err)
			return
		}
		fallback := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s\n\n%s", today, today, planContent)
		content := m.dailyNoteContent(today, fallback)
		writeErr = atomicWriteNote(dailyPath, content)
	} else {
		// Replace existing "## Day Plan" section or append if not present
		newContent := replaceDailySection(string(existing), planContent, "## Day Plan")
		writeErr = atomicWriteNote(dailyPath, newContent)
	}
	if writeErr != nil {
		m.reportError("write day plan", writeErr)
		return
	}

	// Write schedule blocks through the unified schedule layer. For task-like
	// slots, locate the source Task (by text) so the ⏰ marker lands on the
	// same line TaskManager reads from — otherwise the task's Plan view
	// would still show it as unscheduled after the AI ran.
	matcher := newTaskTextMatcher(m.cachedTasks)
	for _, slot := range schedule {
		ref := matcher.Find(slot.Task)
		kind := NormaliseBlockType(slot.Type)
		block := PlannerBlock{
			Date: today, StartTime: slot.Start, EndTime: slot.End,
			Text: slot.Task, BlockType: kind, SourceRef: ref,
		}
		if kind.IsTaskLike() && ref.hasLocation() {
			_ = SetTaskSchedule(m.vault.Root, today, ref, slot.Start, slot.End, kind)
		} else {
			// Non-task blocks (Lunch, Break, Review, Meeting) only live on the
			// planner side — there's no source task line to annotate.
			_ = UpsertPlannerBlock(m.vault.Root, today, ScheduleRef{Text: slot.Task}, block)
		}
	}

	// Write focus data to planner file
	m.reportError("write planner focus", writePlannerFocus(m.vault.Root, today, topGoal, focusOrder))

	_ = m.vault.Scan()
	m.index.Build()
	paths := m.vault.SortedPaths()
	m.sidebar.SetFiles(paths)
	m.autocomplete.SetNotes(paths)
	m.statusbar.SetNoteCount(m.vault.NoteCount())
	m.loadNote(dailyName)
	m.setSidebarCursorToFile(dailyName)
	m.setFocus(focusEditor)
}

// Planner I/O lives in planner_io.go; the unified schedule-write API
// (SetTaskSchedule / UpsertPlannerBlock / …) lives in schedulesync.go.
