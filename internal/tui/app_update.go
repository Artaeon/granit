package tui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/vault"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Splash screen phase
	if m.showSplash {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.splash.width = msg.Width
			m.splash.height = msg.Height
			m.updateLayout()
			return m, nil
		case tea.KeyMsg:
			// Any key press immediately dismisses the splash
			m.showSplash = false
			if !m.config.TutorialCompleted {
				m.tutorial.SetSize(m.width, m.height)
				m.tutorial.Open()
			}
			if m.pendingDailyNote {
				m.pendingDailyNote = false
				m.openDailyNote()
			}
			return m, m.autoSync.CheckStatus()
		case splashTickMsg:
			var cmd tea.Cmd
			m.splash, cmd = m.splash.Update(msg)
			if m.splash.IsDone() {
				m.showSplash = false
				if !m.config.TutorialCompleted {
					m.tutorial.SetSize(m.width, m.height)
					m.tutorial.Open()
				}
				if m.pendingDailyNote {
					m.pendingDailyNote = false
					m.openDailyNote()
				}
				return m, m.autoSync.CheckStatus()
			}
			return m, cmd
		}
		// Don't swallow other messages (e.g. gitStatusMsg, autoSyncResultMsg)
		// — let them fall through to the main switch so background commands
		// dispatched during Init() are processed while the splash is visible.
	}

	// Exit splash handling
	if m.showExitSplash {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.exitSplash.width = msg.Width
			m.exitSplash.height = msg.Height
		case exitTickMsg:
			var cmd tea.Cmd
			m.exitSplash, cmd = m.exitSplash.Update(msg)
			if m.exitSplash.IsDone() {
				m.quitting = true
				return m, tea.Quit
			}
			return m, cmd
		case tea.KeyMsg:
			m.quitting = true
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.exitSplash, cmd = m.exitSplash.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case clearMessageMsg:
		m.statusbar.SetMessage("")
		return m, nil

	case ollamaStatusMsg:
		if msg.ready {
			m.toast.Show("AI ready: "+msg.text, ToastSuccess)
		} else {
			m.toast.Show(msg.text, ToastWarning)
		}
		return m, nil

	case vimMacroReplayMsg:
		if msg.idx >= len(msg.keys) {
			if m.vimState != nil {
				m.vimState.SetPlayingMacro(false)
			}
			return m, nil
		}
		key := msg.keys[msg.idx]
		nextMsg := vimMacroReplayMsg{keys: msg.keys, idx: msg.idx + 1}
		updatedModel, cmd := m.Update(key)
		resultModel := updatedModel.(Model)
		nextCmd := func() tea.Msg { return nextMsg }
		if cmd != nil {
			return resultModel, tea.Batch(cmd, nextCmd)
		}
		return resultModel, nextCmd

	case toastExpireMsg:
		if m.toast != nil {
			m.toast.HandleExpire()
		}
		return m, nil

	case autoSaveTickMsg:
		// Only save if this tick matches the last edit time (debounce)
		if m.vault == nil {
			return m, nil
		}
		if m.config.AutoSave && msg.editTime.Equal(m.lastEditTime) && m.editor.modified && m.activeNote != "" {
			content := m.editor.GetContent()
			path := filepath.Join(m.vault.Root, m.activeNote)
			if err := atomicWriteNote(path, content); err != nil {
				m.statusbar.SetMessage("Autosave failed: " + err.Error())
				return m, m.clearMessageAfter(5 * time.Second)
			}
			m.editor.modified = false
			m.lastSaveTime = time.Now()
			// Incrementally update the search index for the saved file
			if m.vault.SearchIndex != nil {
				m.vault.SearchIndex.Update(m.activeNote, content)
			}
			m.statusbar.SetMessage("Auto-saved " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
		return m, nil

	case spellCheckTickMsg:
		// Debounced inline spell check: only run if this tick matches the last edit time
		if m.spellcheck.InlineEnabled() && msg.editTime.Equal(m.lastSpellEditTime) && m.activeNote != "" {
			content := m.editor.GetContent()
			return m, m.spellcheck.RunInlineCheck(content)
		}
		return m, nil

	case spellCheckDoneMsg:
		m.spellcheck.HandleInlineResult(msg.words)
		m.editor.SetSpellPositions(m.spellcheck.InlinePositions())
		return m, nil

	case gitCmdResultMsg:
		if m.git.IsActive() {
			var cmd tea.Cmd
			m.git, cmd = m.git.Update(msg)
			return m, cmd
		}
		return m, nil

	case ncTestResultMsg, ncPushResultMsg, ncPullResultMsg, ncSyncResultMsg:
		if m.nextcloudOverlay.IsActive() {
			var cmd tea.Cmd
			m.nextcloudOverlay, cmd = m.nextcloudOverlay.Update(msg)
			// After pull/sync, rescan vault for new files
			switch msg.(type) {
			case ncPullResultMsg, ncSyncResultMsg:
				if err := m.vault.Scan(); err != nil {
					log.Printf("warning: vault scan failed: %v", err)
				}
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
			}
			return m, cmd
		}
		return m, nil

	case autoSyncResultMsg:
		if msg.err != nil {
			m.statusbar.SetError("Git sync error: " + msg.err.Error())
		} else if msg.action == "pull" && msg.output != "" {
			trimmed := strings.TrimSpace(msg.output)
			if trimmed != "" && trimmed != "Already up to date." {
				// Rescan vault after pull brought changes
				if err := m.vault.Scan(); err != nil {
					log.Printf("warning: vault scan failed: %v", err)
				}
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
				m.statusbar.SetMessage("Git: pulled latest changes")
			}
		} else if msg.output == "synced" {
			m.statusbar.SetMessage("Git: auto-synced")
		}
		if msg.action != "" {
			// Refresh git status indicator after sync completes
			return m, tea.Batch(m.clearMessageAfter(3*time.Second), m.autoSync.CheckStatus())
		}
		return m, nil

	case gitStatusMsg:
		m.statusbar.SetGitInitialized(msg.isGitRepo)
		if !msg.isGitRepo {
			m.statusbar.SetGitStatus("no git")
		} else if msg.isSynced {
			m.statusbar.SetGitStatus("synced")
		} else {
			m.statusbar.SetGitStatus(fmt.Sprintf("%d changed", msg.changed))
		}
		return m, nil

	case saveResultMsg:
		if msg.err != nil {
			m.statusbar.SetMessage("Save failed: " + msg.err.Error())
			var toastCmd tea.Cmd
			if m.toast != nil {
				toastCmd = m.toast.ShowError("Save failed: " + msg.err.Error())
			}
			return m, tea.Batch(toastCmd, m.clearMessageAfter(5*time.Second))
		}
		m.statusbar.SetMessage("Saved " + m.activeNote)
		var toastCmd tea.Cmd
		if m.toast != nil {
			toastCmd = m.toast.ShowSuccess("Saved " + m.activeNote)
		}
		// Mark saved note's embedding as stale and reindex in background.
		var bgCmd tea.Cmd
		if m.config.SemanticSearchEnabled && m.config.AIProvider != "local" {
			m.semanticSearch.MarkNoteStale(m.activeNote)
			bgCmd = m.startSemanticBgIndex()
		}
		return m, tea.Batch(toastCmd, m.clearMessageAfter(2*time.Second), bgCmd)

	case splitPanePickMsg:
		if m.splitPane.IsActive() {
			note := m.vault.GetNote(msg.notePath)
			if note != nil {
				lines := strings.Split(note.Content, "\n")
				m.splitPane.SetRightContent(msg.notePath, lines)
			}
		}
		return m, nil

	case luaRunResultMsg:
		if m.luaOverlay.IsActive() {
			r := msg.result
			// Apply content/insert if provided
			if r.Content != "" {
				m.editor.SetContent(r.Content)
			}
			if r.Insert != "" {
				m.editor.InsertText(r.Insert)
			}
			var cmd tea.Cmd
			m.luaOverlay, cmd = m.luaOverlay.Update(msg)
			return m, cmd
		}
		return m, nil

	case publishResultMsg, publishProgressMsg:
		if m.publisher.IsActive() {
			var cmd tea.Cmd
			m.publisher, cmd = m.publisher.Update(msg)
			return m, cmd
		}
		return m, nil

	case blogPublishResultMsg:
		if m.blogPublisher.IsActive() {
			var cmd tea.Cmd
			m.blogPublisher, cmd = m.blogPublisher.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiChatResultMsg, aiChatTickMsg:
		if m.aiChat.IsActive() {
			var cmd tea.Cmd
			m.aiChat, cmd = m.aiChat.Update(msg)
			return m, cmd
		}
		return m, nil

	case streamChunkMsg, streamDoneMsg:
		// Route streaming messages by tag to the correct overlay.
		var tag string
		switch m := msg.(type) {
		case streamChunkMsg:
			tag = m.tag
		case streamDoneMsg:
			tag = m.tag
		}
		switch tag {
		case "planmyday":
			if m.planMyDay.IsActive() {
				var cmd tea.Cmd
				m.planMyDay, cmd = m.planMyDay.Update(msg)
				return m, cmd
			}
		case "tasktriage":
			if m.taskTriage.IsActive() {
				var cmd tea.Cmd
				m.taskTriage, cmd = m.taskTriage.Update(msg)
				return m, cmd
			}
		default: // "aichat" and any others
			if m.aiChat.IsActive() {
				var cmd tea.Cmd
				m.aiChat, cmd = m.aiChat.Update(msg)
				return m, cmd
			}
		}
		return m, nil

	case composerResultMsg, composerTickMsg:
		if m.composer.IsActive() {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			// Check if user accepted a generated note
			if !m.composer.IsActive() {
				if title, content, ok := m.composer.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("AI note created: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}
		return m, nil

	case semanticSearchMsg, semanticBuildMsg, semanticTickMsg:
		if m.semanticSearch.IsActive() {
			var cmd tea.Cmd
			m.semanticSearch, cmd = m.semanticSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case semanticBgIndexMsg:
		if msg.err != nil {
			m.statusbar.SetMessage("Embedding index error: " + msg.err.Error())
		} else if msg.statusText != "" {
			m.statusbar.SetMessage(msg.statusText)
			// Reload index so semantic search overlay can use it.
			if m.semanticSearch.vaultPath != "" {
				m.semanticSearch.index = LoadIndex(m.semanticSearch.vaultPath)
			}
		}
		return m, m.clearMessageAfter(3 * time.Second)

	case tutorialSaveErrMsg:
		m.statusbar.SetMessage("Failed to save tutorial state: " + msg.err.Error())
		return m, m.clearMessageAfter(3 * time.Second)

	case threadWeaverResultMsg, threadWeaverTickMsg:
		if m.threadWeaver.IsActive() {
			var cmd tea.Cmd
			m.threadWeaver, cmd = m.threadWeaver.Update(msg)
			if !m.threadWeaver.IsActive() {
				if title, content, ok := m.threadWeaver.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Thread woven: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}
		return m, nil

	case noteChatResultMsg, noteChatTickMsg:
		if m.noteChat.IsActive() {
			var cmd tea.Cmd
			m.noteChat, cmd = m.noteChat.Update(msg)
			return m, cmd
		}
		return m, nil

	case vaultRefactorResultMsg, vaultRefactorTickMsg:
		if m.vaultRefactor.IsActive() {
			var cmd tea.Cmd
			m.vaultRefactor, cmd = m.vaultRefactor.Update(msg)
			if !m.vaultRefactor.IsActive() {
				if plan, ok := m.vaultRefactor.GetResult(); ok {
					m.applyVaultRefactor(plan)
				}
			}
			return m, cmd
		}
		return m, nil

	case gitHistoryResultMsg:
		if m.gitHistory.IsActive() {
			var cmd tea.Cmd
			m.gitHistory, cmd = m.gitHistory.Update(msg)
			if !m.gitHistory.IsActive() {
				if hash, ok := m.gitHistory.GetRestoreResult(); ok {
					// Restore file content from git
					_ = hash
					content := msg.output
					if content != "" && m.activeNote != "" {
						path := filepath.Join(m.vault.Root, m.activeNote)
						if err := atomicWriteNote(path, content); err != nil {
							m.statusbar.SetMessage("Git restore failed: " + err.Error())
							return m, m.clearMessageAfter(5 * time.Second)
						}
						if err := m.vault.Scan(); err != nil {
							log.Printf("warning: vault scan failed: %v", err)
						}
						m.index = vault.NewIndex(m.vault)
						m.index.Build()
						m.loadNote(m.activeNote)
						m.statusbar.SetMessage("Restored from git history")
						return m, m.clearMessageAfter(3 * time.Second)
					}
				}
			}
			return m, cmd
		}
		return m, nil

	case gmAIResultMsg, gmAIReviewMsg, gmAIDecomposeMsg, gmAICoachMsg:
		if m.goalsMode.IsActive() {
			var cmd tea.Cmd
			m.goalsMode, cmd = m.goalsMode.Update(msg)
			return m, cmd
		}
		return m, nil

	case morningPlanSavedMsg:
		if m.morningRoutine.IsActive() {
			var cmd tea.Cmd
			m.morningRoutine, cmd = m.morningRoutine.Update(msg)
			return m, cmd
		}
		// Full refresh so calendar picks up new planner blocks, focus, and daily note
		m.refreshComponents("")
		return m, nil

	case autoLinkSuggestMsg:
		if m.autoLinkSuggest != nil {
			m.autoLinkSuggest.SetInFlight(false)
		}
		if msg.err == nil && len(msg.suggestions) > 0 {
			links := make([]string, len(msg.suggestions))
			for i, s := range msg.suggestions {
				links[i] = "[[" + s + "]]"
			}
			if m.toast != nil {
				toastCmd := m.toast.ShowInfo("Link suggestions: " + strings.Join(links, ", "))
				return m, toastCmd
			}
		}
		return m, nil

	case habitAICoachMsg:
		if m.habitTracker.IsActive() {
			var cmd tea.Cmd
			m.habitTracker, cmd = m.habitTracker.Update(msg)
			return m, cmd
		}
		return m, nil

	case devotionalResultMsg, devotionalTickMsg:
		if m.devotional.IsActive() {
			var cmd tea.Cmd
			m.devotional, cmd = m.devotional.Update(msg)
			return m, cmd
		}
		return m, nil

	case weeklyReviewAIMsg:
		if m.weeklyReview.IsActive() {
			m.weeklyReview, _ = m.weeklyReview.Update(msg)
			return m, nil
		}
		return m, nil

	case dailyReviewAIMsg:
		if m.dailyReview.IsActive() {
			m.dailyReview, _ = m.dailyReview.Update(msg)
			return m, nil
		}
		return m, nil

	case pmAIInsightMsg:
		if m.projectMode.IsActive() {
			var cmd tea.Cmd
			m.projectMode, cmd = m.projectMode.Update(msg)
			return m, cmd
		}
		return m, nil

	case tmAIResultMsg:
		if m.taskManager.IsActive() {
			var cmd tea.Cmd
			m.taskManager, cmd = m.taskManager.Update(msg)
			if m.taskManager.WasFileChanged() {
				m.refreshComponents(m.taskManager.ActiveNotePath())
			}
			return m, cmd
		}
		return m, nil

	case briefingResultMsg, briefingTickMsg:
		if m.dailyBriefing.IsActive() {
			var cmd tea.Cmd
			m.dailyBriefing, cmd = m.dailyBriefing.Update(msg)
			if !m.dailyBriefing.IsActive() {
				if content, ok := m.dailyBriefing.GetResult(); ok {
					m.writeBriefingToDailyNote(content)
					m.statusbar.SetDayPlanned(true)
				}
			}
			return m, cmd
		}
		return m, nil

	case aiTemplateResultMsg:
		if m.aiTemplates.IsActive() {
			var cmd tea.Cmd
			m.aiTemplates, cmd = m.aiTemplates.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiTemplateTickMsg:
		if m.aiTemplates.IsActive() {
			var cmd tea.Cmd
			m.aiTemplates, cmd = m.aiTemplates.Update(msg)
			return m, cmd
		}
		return m, nil

	case focusSessionTickMsg:
		if m.focusSession.IsActive() {
			var cmd tea.Cmd
			m.focusSession, cmd = m.focusSession.Update(msg)
			return m, cmd
		}
		return m, nil

	case noteHistoryResultMsg:
		if m.noteHistory.IsActive() {
			var cmd tea.Cmd
			m.noteHistory, cmd = m.noteHistory.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiSchedulerResultMsg:
		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiSchedulerTickMsg:
		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			return m, cmd
		}
		return m, nil

	case planMyDayResultMsg:
		if m.planMyDay.IsActive() {
			if msg.err == nil {
				m.statusbar.SetDayPlanned(true)
			}
			var cmd tea.Cmd
			m.planMyDay, cmd = m.planMyDay.Update(msg)
			return m, cmd
		}
		return m, nil

	case planMyDayTickMsg:
		if m.planMyDay.IsActive() {
			var cmd tea.Cmd
			m.planMyDay, cmd = m.planMyDay.Update(msg)
			return m, cmd
		}
		return m, nil

	case planMyDayGatherMsg:
		if m.planMyDay.IsActive() {
			var cmd tea.Cmd
			m.planMyDay, cmd = m.planMyDay.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiPlannerResultMsg:
		if m.aiProjectPlanner.IsActive() {
			var cmd tea.Cmd
			m.aiProjectPlanner, cmd = m.aiProjectPlanner.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiPlannerTickMsg:
		if m.aiProjectPlanner.IsActive() {
			var cmd tea.Cmd
			m.aiProjectPlanner, cmd = m.aiProjectPlanner.Update(msg)
			return m, cmd
		}
		return m, nil

	case blogOutlineResultMsg, blogDraftResultMsg, blogTickMsg:
		if m.blogDraft.IsActive() {
			var cmd tea.Cmd
			m.blogDraft, cmd = m.blogDraft.Update(msg)
			// Check if user saved a blog post
			if !m.blogDraft.IsActive() {
				if title, content, ok := m.blogDraft.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Blog post created: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}
		return m, nil

	case triageResultMsg, triageTickMsg:
		if m.taskTriage.IsActive() {
			var cmd tea.Cmd
			m.taskTriage, cmd = m.taskTriage.Update(msg)
			return m, cmd
		}
		return m, nil

	case nlSearchResultMsg:
		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case nlSearchTickMsg:
		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case writingCoachResultMsg:
		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			return m, cmd
		}
		return m, nil

	case writingCoachTickMsg:
		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			return m, cmd
		}
		return m, nil

	case timeTrackerTickMsg:
		if m.timeTracker.IsTimerRunning() {
			var cmd tea.Cmd
			m.timeTracker, cmd = m.timeTracker.Update(msg)
			return m, cmd
		}
		return m, nil

	case researchResultMsg:
		// Always handle — research runs in background even if overlay is closed
		if m.research.IsRunning() {
			m.research.running = false
			m.statusbar.SetResearchStatus("")
			if msg.err != nil {
				m.research.phase = researchError
				m.research.errorMsg = msg.err.Error()
				m.research.output = msg.output
				m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
				m.statusbar.SetError("Research failed: " + msg.err.Error())
			} else {
				m.research.phase = researchDone
				m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
				m.research.output = msg.output
				// Refresh vault to pick up new files
				if err := m.vault.Scan(); err != nil {
					log.Printf("warning: vault scan failed: %v", err)
				}
				m.index = vault.NewIndex(m.vault)
				m.index.Build()
				paths := m.vault.SortedPaths()
				m.sidebar.SetFiles(paths)
				m.autocomplete.SetNotes(paths)
				m.statusbar.SetNoteCount(m.vault.NoteCount())
				if len(msg.filesHint) > 0 {
					m.research.createdFiles = msg.filesHint
				} else {
					for _, p := range paths {
						if strings.HasPrefix(p, "Research/") {
							m.research.createdFiles = append(m.research.createdFiles, p)
						}
					}
				}
				m.statusbar.SetMessage(fmt.Sprintf("Research complete: %d notes created — open via command palette", len(m.research.createdFiles)))
				// Auto-open the overlay to show results if it was closed
				if !m.research.IsActive() {
					m.research.Reopen()
				}
			}
			return m, m.clearMessageAfter(5 * time.Second)
		}
		return m, nil

	case researchTickMsg:
		if m.research.IsRunning() {
			m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
			m.statusbar.SetResearchStatus(m.research.StatusText())
			return m, m.research.tickElapsed()
		}
		return m, nil

	case statusTrayActionMsg:
		switch msg.action {
		case trayActionGit:
			m.git.SetSize(m.width, m.height)
			return m, m.git.Open(m.vault.Root)
		case trayActionResearch:
			m.research.SetSize(m.width, m.height)
			if m.research.IsRunning() {
				m.research.Reopen()
			} else {
				m.research.Open(m.vault.Root, m.vault.SortedPaths(), m.activeNote)
			}
		}
		return m, nil

	case autoTagResultMsg:
		if m.autoTagger != nil {
			m.autoTagger.SetInFlight(false)
		}
		if msg.err == nil && len(msg.tags) > 0 {
			m.applyTagsToNote(msg.tags)
			m.statusbar.SetMessage("Auto-tagged: " + strings.Join(msg.tags, ", "))
			return m, m.clearMessageAfter(3 * time.Second)
		}
		return m, nil

	case ghostSuggestionMsg, ghostDebounceMsg:
		if m.ghostWriter != nil {
			cmd := m.ghostWriter.HandleMsg(msg)
			m.editor.SetGhostText(m.ghostWriter.GetSuggestion())
			if errMsg := m.ghostWriter.ConsumeError(); errMsg != "" {
				m.statusbar.SetWarning(errMsg)
			}
			return m, cmd
		}
		return m, nil

	case fileChangeMsg:
		if m.fileWatcher == nil || !m.fileWatcher.IsEnabled() {
			return m, nil
		}
		// Refresh all components (vault, sidebar, calendar, task manager, etc.)
		m.refreshComponents("")
		var toastCmd tea.Cmd
		// Check whether the currently open note was changed externally
		for _, p := range msg.paths {
			rel, _ := filepath.Rel(m.vault.Root, p)
			if rel == m.activeNote {
				// Ignore events caused by our own save
				if time.Since(m.lastSaveTime) < time.Second {
					break
				}
				if m.editor.modified {
					// Unsaved changes — ask user before overwriting
					m.pendingReload = true
					m.pendingReloadPath = m.activeNote
					if m.toast != nil {
						toastCmd = m.toast.ShowWarning("File modified externally. Reload? (y/n)")
					}
				} else {
					// No local changes — silently reload and preserve cursor
					curLine, curCol := m.editor.GetCursor()
					curScroll := m.editor.scroll
					if note := m.vault.GetNote(m.activeNote); note != nil {
						m.editor.LoadContent(note.Content, m.activeNote)
						m.editor.cursor = curLine
						m.editor.col = curCol
						// Clamp to new content bounds
						if m.editor.cursor >= len(m.editor.content) {
							m.editor.cursor = len(m.editor.content) - 1
						}
						if m.editor.cursor < 0 {
							m.editor.cursor = 0
						}
						if len(m.editor.content) > 0 {
							if m.editor.col > len(m.editor.content[m.editor.cursor]) {
								m.editor.col = len(m.editor.content[m.editor.cursor])
							}
						}
						m.editor.scroll = curScroll
					}
					baseName := filepath.Base(m.activeNote)
					if m.toast != nil {
						toastCmd = m.toast.ShowInfo("Reloaded: " + baseName)
					}
				}
				break
			}
		}
		return m, tea.Batch(toastCmd, m.fileWatcher.waitForEvent())

	case pomodoroTickMsg:
		var cmd tea.Cmd
		m.pomodoro, cmd = m.pomodoro.Update(msg)
		m.syncPomodoroCompletions()
		m.syncPomodoroTimeRecords()
		return m, cmd

	case clockInTickMsg:
		var cmd tea.Cmd
		m.clockIn, cmd = m.clockIn.Update(msg)
		return m, cmd

	case clockInReminderMsg:
		// Fire terminal bell and show toast notification
		reminderText := msg.Text
		m.statusbar.SetMessage(fmt.Sprintf("🔔 %s", reminderText))
		return m, tea.Printf("\a") // terminal bell

	case webClipResult, webClipTickMsg:
		if m.webClipper.IsActive() {
			var cmd tea.Cmd
			m.webClipper, cmd = m.webClipper.Update(msg)
			return m, cmd
		}
		return m, nil

	case botsTickMsg:
		if m.bots.IsActive() {
			var cmd tea.Cmd
			m.bots, cmd = m.bots.Update(msg)
			return m, cmd
		}
		return m, nil

	case botAIResultMsg:
		if m.bots.IsActive() {
			var cmd tea.Cmd
			m.bots, cmd = m.bots.Update(msg)
			return m, cmd
		}
		return m, nil

	case ollamaSetupMsg:
		if m.settings.IsActive() {
			m.settings, _ = m.settings.Update(msg)
			if !m.settings.setupRunning && msg.success {
				m.config = m.settings.GetConfig()
				_ = m.config.Save()
				m.renderer.SetViewStyle(m.config.ViewStyle)
			}
		}
		return m, nil

	case ollamaStartMsg:
		if m.settings.IsActive() {
			m.settings, _ = m.settings.Update(msg)
		}
		if msg.success {
			m.toast.Show(msg.message, ToastSuccess)
		} else {
			m.toast.Show(msg.message, ToastWarning)
		}
		return m, nil

	case pluginCmdResultMsg:
		if msg.err != nil {
			m.statusbar.SetError("Plugin error: " + msg.err.Error())
		} else {
			m.handlePluginOutput(msg.pluginName, msg.output)
		}
		return m, m.clearMessageAfter(3 * time.Second)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		// Auto-open daily note on first resize when splash is not shown
		if m.pendingDailyNote && !m.showSplash {
			m.pendingDailyNote = false
			m.openDailyNote()
		}
		return m, nil

	case tea.KeyMsg:
		// Handle overlay modes first (in priority order)
		if m.helpOverlay.IsActive() {
			m.helpOverlay, _ = m.helpOverlay.Update(msg)
			return m, nil
		}

		// Focus mode goal-setting prompt intercepts all keys while active.
		if m.focusMode.IsActive() && m.focusMode.IsSettingGoal() {
			m.focusMode, _ = m.focusMode.Update(msg)
			return m, nil
		}

		if m.settings.IsActive() {
			var settingsCmd tea.Cmd
			m.settings, settingsCmd = m.settings.Update(msg)
			if !m.settings.IsActive() {
				m.config = m.settings.GetConfig()
				_ = m.config.Save()
				m.syncConfigToComponents()
				m.sidebar.SetFiles(m.vault.SortedPaths())
				// Start background embedding indexing if semantic search was just enabled.
				if m.config.SemanticSearchEnabled && m.config.AIProvider != "local" {
					if bgCmd := m.startSemanticBgIndex(); bgCmd != nil {
						return m, tea.Batch(settingsCmd, bgCmd)
					}
				}
			}
			return m, settingsCmd
		}

		if m.git.IsActive() {
			var gitCmd tea.Cmd
			m.git, gitCmd = m.git.Update(msg)
			return m, gitCmd
		}

		if m.vaultStats.IsActive() {
			m.vaultStats, _ = m.vaultStats.Update(msg)
			return m, nil
		}

		if m.graphView.IsActive() {
			m.graphView, _ = m.graphView.Update(msg)
			if nav := m.graphView.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.setSidebarCursorToFile(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.tagBrowser.IsActive() {
			m.tagBrowser, _ = m.tagBrowser.Update(msg)
			if nav := m.tagBrowser.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.setSidebarCursorToFile(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.outline.IsActive() {
			m.outline, _ = m.outline.Update(msg)
			if jumpLine := m.outline.JumpToLine(); jumpLine >= 0 {
				m.editor.cursor = jumpLine
				m.editor.col = 0
				if jumpLine < m.editor.scroll || jumpLine >= m.editor.scroll+m.editor.height-4 {
					m.editor.scroll = jumpLine
				}
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.bookmarks.IsActive() {
			m.bookmarks, _ = m.bookmarks.Update(msg)
			if nav := m.bookmarks.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.setSidebarCursorToFile(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.findReplace.IsActive() {
			m.findReplace, _ = m.findReplace.Update(msg)
			m.findReplace.UpdateMatches(m.editor.content)
			if jumpLine := m.findReplace.GetJumpLine(); jumpLine >= 0 {
				m.editor.cursor = jumpLine
				m.editor.col = 0
				if jumpLine < m.editor.scroll || jumpLine >= m.editor.scroll+m.editor.height-4 {
					m.editor.scroll = jumpLine
				}
			}
			if m.findReplace.ShouldReplace() {
				m.doReplace()
			}
			if m.findReplace.ShouldReplaceAll() {
				m.doReplaceAll()
			}
			return m, nil
		}

		if m.templates.IsActive() {
			m.templates, _ = m.templates.Update(msg)
			if !m.templates.IsActive() && m.templates.WasSelected() {
				tmpl := m.templates.SelectedTemplate()
				m.newNoteMode = true
				m.newNoteName = ""
				m.pendingTemplate = tmpl // may be "" for blank note
			}
			return m, nil
		}

		if m.quickSwitch.IsActive() {
			m.quickSwitch, _ = m.quickSwitch.Update(msg)
			if nav := m.quickSwitch.SelectedFile(); nav != "" {
				m.loadNote(nav)
				m.setSidebarCursorToFile(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.trash.IsActive() {
			m.trash, _ = m.trash.Update(msg)
			if m.trash.ShouldRestore() {
				restored := m.trash.RestoreFile()
				if restored != "" {
					if err := m.vault.Scan(); err != nil {
						log.Printf("warning: vault scan failed: %v", err)
					}
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(restored)
					m.setSidebarCursorToFile(restored)
					m.statusbar.SetMessage("Restored: " + restored)
				}
			}
			return m, nil
		}

		if m.canvas.IsActive() {
			m.canvas, _ = m.canvas.Update(msg)
			if nav := m.canvas.SelectedNote(); nav != "" {
				resolved := m.resolveLink(nav)
				if resolved != "" {
					_, anchor := splitLinkAnchor(nav)
					m.loadNote(resolved)
					m.navigateToHeading(anchor)
					m.setSidebarCursorToFile(resolved)
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.calendar.IsActive() {
			// Clear refresh flag — calendar data is already updated by refreshComponents
			if m.needsRefresh {
				m.needsRefresh = false
			}
			m.calendar, _ = m.calendar.Update(msg)
			// Handle quick-add event: append task to daily note
			// Handle full event creation from wizard
			if ne := m.calendar.PendingNativeEvent(); ne != nil && m.eventStore != nil {
				m.eventStore.Add(ne.Title, ne.Date, ne.StartTime, ne.EndTime,
					ne.Location, ne.Description, ne.Color, ne.Recurrence, ne.AllDay)
				m.refreshComponents("")
				m.toast.Show("Event created: "+ne.Title, ToastSuccess)
			}
			// Handle event deletion
			if delID := m.calendar.PendingDeleteID(); delID != "" && m.eventStore != nil {
				if e := m.eventStore.Get(delID); e != nil {
					m.eventStore.Delete(delID)
					m.refreshComponents("")
					m.toast.Show("Event deleted: "+e.Title, ToastSuccess)
				}
			}
			if evDate, evText, ok := m.calendar.PendingEvent(); ok {
				name := evDate + ".md"
				folder := m.config.DailyNotesFolder
				if folder != "" {
					name = filepath.Join(folder, name)
				}
				path := filepath.Join(m.vault.Root, name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						m.statusbar.SetMessage("Error creating directory: " + err.Error())
						return m, nil
					}
					fallback := m.buildDailyFallback(evDate)
					content := m.dailyNoteContent(evDate, fallback)
					if err := atomicWriteNote(path, content); err != nil {
						m.statusbar.SetMessage("Error creating daily note: " + err.Error())
						return m, nil
					}
				}
				// Append the task line atomically.
				if existing, readErr := os.ReadFile(path); readErr == nil {
					_ = atomicWriteNote(path, string(existing)+"- [ ] "+evText+"\n")
				}
				// Save as native event in event store
				if m.eventStore != nil {
					m.eventStore.Add(evText, evDate, "", "", "", "", "", "", true)
				}
				// Also add to planner file for the date
				AddEventToPlannerFile(m.vault.Root, evDate, evText)
				m.refreshComponents(name)
				m.statusbar.SetMessage("Event added to " + evDate)
			}
			// Handle task toggles from the agenda view
			if toggles := m.calendar.GetTaskToggles(); len(toggles) > 0 {
				for _, toggle := range toggles {
					absPath := filepath.Join(m.vault.Root, toggle.NotePath)
					data, err := os.ReadFile(absPath)
					if err != nil {
						continue
					}
					lines := strings.Split(string(data), "\n")
					if toggle.LineNum < 1 || toggle.LineNum > len(lines) {
						continue
					}
					line := lines[toggle.LineNum-1]
					// Validate that the line still contains the expected task marker and text
					hasMarker := strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]") || strings.Contains(line, "- [X]")
					hasText := toggle.Text == "" || strings.Contains(line, toggle.Text)
					if !hasMarker || !hasText {
						continue
					}
					if toggle.Done {
						line = strings.Replace(line, "[ ]", "[x]", 1)
					} else {
						line = strings.Replace(line, "[x]", "[ ]", 1)
						line = strings.Replace(line, "[X]", "[ ]", 1)
					}
					lines[toggle.LineNum-1] = line
					if err := atomicWriteNote(absPath, strings.Join(lines, "\n")); err != nil {
						m.statusbar.SetError("Error syncing task: " + err.Error())
					}
				}
				// Refresh all components after toggling tasks
				m.refreshComponents("")
				m.statusbar.SetMessage("Task toggled")
			}
			if date := m.calendar.SelectedDate(); date != "" {
				// Open or create daily note for selected date
				name := date + ".md"
				folder := m.config.DailyNotesFolder
				if folder != "" {
					name = filepath.Join(folder, name)
				}
				path := filepath.Join(m.vault.Root, name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						m.statusbar.SetMessage("Error creating directory: " + err.Error())
						return m, nil
					}
					fallback := m.buildDailyFallback(date)
					content := m.dailyNoteContent(date, fallback)
					if err := atomicWriteNote(path, content); err != nil {
						m.statusbar.SetMessage("Error creating daily note: " + err.Error())
						return m, nil
					}
					m.refreshComponents(name)
				}
				m.loadNote(name)
				m.setSidebarCursorToFile(name)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.bots.IsActive() {
			var botCmd tea.Cmd
			m.bots, botCmd = m.bots.Update(msg)
			// Only process result when user closed the overlay (Enter on results)
			if !m.bots.IsActive() {
				if result := m.bots.GetResult(); result.Action != "none" {
					switch result.Action {
					case "tag":
						if m.activeNote != "" && len(result.Tags) > 0 {
							m.applyTagsToNote(result.Tags)
							tagStr := strings.Join(result.Tags, ", ")
							m.statusbar.SetMessage("Applied tags: " + tagStr)
						}
					case "link":
						if len(result.Links) > 0 {
							m.statusbar.SetMessage("Found " + fmt.Sprintf("%d", len(result.Links)) + " related notes")
						}
					case "summary":
						if result.Summary != "" {
							m.statusbar.SetMessage("Summary generated")
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, botCmd
		}

		if m.export.IsActive() {
			m.export, _ = m.export.Update(msg)
			return m, nil
		}

		if m.plugins.IsActive() {
			var pluginCmd tea.Cmd
			m.plugins, pluginCmd = m.plugins.Update(msg)
			if !m.plugins.IsActive() {
				if pCmd := m.plugins.PendingCommand(); pCmd != nil {
					return m, RunPluginCommand(pCmd.plugin, pCmd.cmdDef, filepath.Join(m.vault.Root, m.activeNote), m.editor.GetContent(), m.vault.Root)
				}
			}
			return m, pluginCmd
		}

		if m.contentSearch.IsActive() {
			m.contentSearch, _ = m.contentSearch.Update(msg)
			if !m.contentSearch.IsActive() {
				if result := m.contentSearch.SelectedResult(); result != nil {
					m.loadNote(result.FilePath)
					m.setSidebarCursorToFile(result.FilePath)
					m.editor.cursor = result.Line
					m.editor.col = result.Col
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.globalReplace.IsActive() {
			m.globalReplace, _ = m.globalReplace.Update(msg)
			if !m.globalReplace.IsActive() {
				// Reload editor if active note was modified
				if m.globalReplace.ModifiedFiles()[m.activeNote] {
					if note := m.vault.GetNote(m.activeNote); note != nil {
						m.editor.LoadContent(note.Content, m.editor.filePath)
					}
				}
				if file, line, ok := m.globalReplace.GetJumpResult(); ok {
					m.loadNote(file)
					m.setSidebarCursorToFile(file)
					m.editor.cursor = line
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.aiTemplates.IsActive() {
			m.aiTemplates, _ = m.aiTemplates.Update(msg)
			if !m.aiTemplates.IsActive() {
				if title, content, ok := m.aiTemplates.GetResult(); ok {
					// Create the new note file
					fileName := title + ".md"
					relPath := fileName
					absPath := filepath.Join(m.vault.Root, relPath)
					if err := atomicWriteNote(absPath, content); err == nil {
						if err := m.vault.Scan(); err != nil {
							log.Printf("warning: vault scan failed: %v", err)
						}
						m.index.Build()
						m.sidebar.SetFiles(m.vault.SortedPaths())
						m.loadNote(relPath)
						m.setSidebarCursorToFile(relPath)
						m.setFocus(focusEditor)
						m.statusbar.SetMessage("Created " + fileName)
					}
				}
			}
			return m, nil
		}

		if m.languageLearning.IsActive() {
			m.languageLearning, _ = m.languageLearning.Update(msg)
			return m, nil
		}

		if m.habitTracker.IsActive() {
			m.habitTracker, _ = m.habitTracker.Update(msg)
			return m, nil
		}

		if m.universalSearch.IsActive() {
			m.universalSearch, _ = m.universalSearch.Update(msg)
			if nav := m.universalSearch.NavResult(); nav != nil {
				switch nav.Type {
				case usResultNote:
					m.loadNote(nav.NotePath)
				case usResultTask:
					m.loadNote(nav.NotePath)
				case usResultGoal:
					allTasks := ParseAllTasks(m.vault.Notes)
					m.goalsMode.SetSize(m.width, m.height)
					m.goalsMode.ai = m.aiConfig()
					m.goalsMode.Open(m.vault.Root, allTasks)
				case usResultHabit:
					m.habitTracker.dailyNotesFolder = m.config.DailyNotesFolder
					m.habitTracker.Open(m.vault.Root)
					m.habitTracker.vault = m.vault
				}
			}
			return m, nil
		}

		if m.ideasBoard.IsActive() {
			m.ideasBoard, _ = m.ideasBoard.Update(msg)
			if !m.ideasBoard.IsActive() && m.ideasBoard.WasFileChanged() {
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.goalsMode.IsActive() {
			m.goalsMode, _ = m.goalsMode.Update(msg)
			if !m.goalsMode.IsActive() && m.goalsMode.WasFileChanged() {
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.focusSession.IsActive() {
			var cmd tea.Cmd
			m.focusSession, cmd = m.focusSession.Update(msg)
			return m, cmd
		}

		if m.standupGen.IsActive() {
			m.standupGen, _ = m.standupGen.Update(msg)
			if !m.standupGen.IsActive() {
				if err := m.vault.Scan(); err != nil {
					log.Printf("warning: vault scan failed: %v", err)
				}
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
			}
			return m, nil
		}

		if m.dailyReview.IsActive() {
			m.dailyReview, _ = m.dailyReview.Update(msg)
			if m.dailyReview.WasFileChanged() {
				m.refreshComponents("")
			}
			if !m.dailyReview.IsActive() {
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.dailyJot.IsActive() {
			m.dailyJot, _ = m.dailyJot.Update(msg)
			if !m.dailyJot.IsActive() {
				if notePath := m.dailyJot.GetPromotedNote(); notePath != "" {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.noteHistory.IsActive() {
			m.noteHistory, _ = m.noteHistory.Update(msg)
			return m, nil
		}

		if m.smartConnect.IsActive() {
			m.smartConnect, _ = m.smartConnect.Update(msg)
			if !m.smartConnect.IsActive() {
				if notePath, ok := m.smartConnect.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
			}
			return m, nil
		}

		if m.writingStats.IsActive() {
			m.writingStats, _ = m.writingStats.Update(msg)
			return m, nil
		}

		if m.quickCapture.IsActive() {
			m.quickCapture, _ = m.quickCapture.Update(msg)
			if !m.quickCapture.IsActive() {
				if filePath, ok := m.quickCapture.GetResult(); ok {
					if err := m.vault.Scan(); err != nil {
						log.Printf("warning: vault scan failed: %v", err)
					}
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetMessage("Saved to " + filePath)
				}
			}
			return m, nil
		}

		if m.dashboard.IsActive() {
			m.dashboard, _ = m.dashboard.Update(msg)
			if !m.dashboard.IsActive() {
				if action, ok := m.dashboard.GetAction(); ok {
					return m.executeCommand(action)
				}
			}
			return m, nil
		}

		if m.mindMap.IsActive() {
			m.mindMap, _ = m.mindMap.Update(msg)
			return m, nil
		}

		if m.journalPrompts.IsActive() {
			m.journalPrompts, _ = m.journalPrompts.Update(msg)
			if !m.journalPrompts.IsActive() {
				if filePath, ok := m.journalPrompts.GetResult(); ok {
					if err := m.vault.Scan(); err != nil {
						log.Printf("warning: vault scan failed: %v", err)
					}
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.loadNote(filePath)
					m.statusbar.SetMessage("Journal saved")
				}
			}
			return m, nil
		}

		if m.clipManager.IsActive() {
			m.clipManager, _ = m.clipManager.Update(msg)
			if !m.clipManager.IsActive() {
				if text, ok := m.clipManager.GetResult(); ok {
					if m.viewMode {
						m.statusbar.SetMessage("Cannot paste in view mode")
					} else {
						if m.editor.HasSelection() {
							m.editor.DeleteSelection()
						}
						if !m.editor.SmartPaste(text) {
							m.editor.InsertText(text)
						}
						m.statusbar.SetMessage("Pasted from clipboard")
					}
				}
			}
			return m, nil
		}

		if m.dailyPlanner.IsActive() {
			// Clear refresh flag — planner reads from its own saved file
			if m.needsRefresh {
				m.needsRefresh = false
			}
			m.dailyPlanner, _ = m.dailyPlanner.Update(msg)

			// Sync completed tasks back to source files
			if completions := m.dailyPlanner.GetCompletedTasks(); len(completions) > 0 {
				for _, tc := range completions {
					if tc.NotePath == "" || tc.LineNum < 1 {
						continue
					}
					if note := m.vault.GetNote(tc.NotePath); note != nil {
						lines := strings.Split(note.Content, "\n")
						idx := tc.LineNum - 1 // TaskCompletion.LineNum is 1-based
						if idx < len(lines) {
							// Validate that the line still contains the expected task marker and text
							line := lines[idx]
							hasMarker := strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]")
							hasText := tc.Text == "" || strings.Contains(line, tc.Text)
							if !hasMarker || !hasText {
								continue
							}
							if tc.Done {
								lines[idx] = strings.Replace(lines[idx], "- [ ]", "- [x]", 1)
							} else {
								lines[idx] = strings.Replace(lines[idx], "- [x]", "- [ ]", 1)
							}
							newContent := strings.Join(lines, "\n")
							if err := atomicWriteNote(filepath.Join(m.vault.Root, tc.NotePath), newContent); err != nil {
								m.statusbar.SetError("Error syncing task: " + err.Error())
							}
						}
					}
				}
				m.refreshComponents("")
			}

			if !m.dailyPlanner.IsActive() {
				m.refreshComponents("")
				m.statusbar.SetMessage("Daily planner closed")
			}
			return m, nil
		}

		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			if !m.aiScheduler.IsActive() {
				if slots, ok := m.aiScheduler.GetSchedule(); ok && len(slots) > 0 {
					tasks, _, habits := m.gatherPlannerData()
					m.dailyPlanner.SetSize(m.width, m.height)
					m.dailyPlanner.Open(m.vault.Root, tasks, nil, habits)
					m.dailyPlanner.ApplyAISchedule(slots)

					// Route every slot through the unified schedule layer so
					// both the ⏰ marker on the source task AND the planner
					// block land together. Non-task slots (break, lunch)
					// only get a planner block — they have no source line.
					today := time.Now().Format("2006-01-02")
					for _, slot := range slots {
						start := fmt.Sprintf("%02d:%02d", slot.StartHour, slot.StartMin)
						end := fmt.Sprintf("%02d:%02d", slot.EndHour, slot.EndMin)
						ref := scheduleRefForSlotText(slot.Task, m.taskManager.allTasks)
						if isTaskSlot(slot.Type) && ref.hasLocation() {
							_ = SetTaskSchedule(m.vault.Root, today, ref, start, end, slot.Type)
						} else {
							_ = UpsertPlannerBlock(m.vault.Root, today, ScheduleRef{Text: slot.Task}, PlannerBlock{
								Date: today, StartTime: start, EndTime: end,
								Text: slot.Task, BlockType: slot.Type, SourceRef: ref,
							})
						}
					}

					// Auto-save planner so calendar can load the schedule
					m.dailyPlanner.SaveNow()

					// Populate pomodoro focus queue from scheduled task slots
					var queueTasks []QueueTask
					for _, slot := range slots {
						if slot.Type != "task" {
							continue
						}
						estMin := slot.slotMinutes()
						if estMin <= 0 {
							estMin = 25
						}
						// Try to find source note info for bidirectional sync
						var srcPath string
						var srcLine int
						for _, t := range m.taskManager.allTasks {
							if t.Text == slot.Task || strings.Contains(t.Text, slot.Task) || strings.Contains(slot.Task, t.Text) {
								srcPath = t.NotePath
								srcLine = t.LineNum
								break
							}
						}
						queueTasks = append(queueTasks, QueueTask{
							Text:       slot.Task,
							Priority:   slot.Priority,
							Estimated:  estMin,
							SourcePath: srcPath,
							SourceLine: srcLine,
						})
					}
					if len(queueTasks) > 0 {
						m.pomodoro.SetVaultRoot(m.vault.Root)
						m.pomodoro.SetQueue(queueTasks)
					}

					// Refresh vault so all components see updated Tasks.md
					m.refreshComponents("")

					m.statusbar.SetMessage("AI schedule applied to planner")
				}
			}
			return m, cmd
		}

		if m.planMyDay.IsActive() {
			var cmd tea.Cmd
			m.planMyDay, cmd = m.planMyDay.Update(msg)
			if !m.planMyDay.IsActive() {
				if sched, goal, focus, advice, ok := m.planMyDay.GetAppliedPlan(); ok && len(sched) > 0 {
					m.writePlanMyDayToDailyNote(sched, goal, focus, advice)
					m.calendar.SetDailyFocus(time.Now().Format("2006-01-02"), DailyFocus{
						TopGoal:    goal,
						FocusItems: focus,
					})
					m.refreshComponents("")
					m.statusbar.SetMessage("Day plan applied to daily note")
				}
			}
			return m, cmd
		}

		if m.aiProjectPlanner.IsActive() {
			var cmd tea.Cmd
			wasActive := m.aiProjectPlanner.IsActive()
			m.aiProjectPlanner, cmd = m.aiProjectPlanner.Update(msg)
			if wasActive && !m.aiProjectPlanner.IsActive() {
				// Refresh project mode data after creating a project
				m.refreshComponents("")
				m.statusbar.SetMessage("AI project plan created")
			}
			return m, cmd
		}

		if m.blogDraft.IsActive() {
			var cmd tea.Cmd
			m.blogDraft, cmd = m.blogDraft.Update(msg)
			if !m.blogDraft.IsActive() {
				if title, content, ok := m.blogDraft.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Blog post created: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}

		if m.taskTriage.IsActive() {
			var cmd tea.Cmd
			m.taskTriage, cmd = m.taskTriage.Update(msg)
			return m, cmd
		}

		if m.recurringTasks.IsActive() {
			m.recurringTasks, _ = m.recurringTasks.Update(msg)
			if !m.recurringTasks.IsActive() {
				if count, ok := m.recurringTasks.GetCreatedCount(); ok && count > 0 {
					if err := m.vault.Scan(); err != nil {
						log.Printf("warning: vault scan failed: %v", err)
					}
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetMessage(fmt.Sprintf("%d recurring tasks created", count))
				}
			}
			return m, nil
		}

		if m.notePreview.IsActive() {
			m.notePreview, _ = m.notePreview.Update(msg)
			if !m.notePreview.IsActive() {
				if notePath, ok := m.notePreview.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
			}
			return m, nil
		}

		if m.scratchpad.IsActive() {
			m.scratchpad, _ = m.scratchpad.Update(msg)
			return m, nil
		}

		if m.backup.IsActive() {
			m.backup, _ = m.backup.Update(msg)
			return m, nil
		}

		if m.nextcloudOverlay.IsActive() {
			var cmd tea.Cmd
			m.nextcloudOverlay, cmd = m.nextcloudOverlay.Update(msg)
			return m, cmd
		}

		if m.tutorial.IsActive() {
			var cmd tea.Cmd
			m.tutorial, cmd = m.tutorial.Update(msg)
			return m, cmd
		}

		if m.projectMode.IsActive() {
			m.projectMode, _ = m.projectMode.Update(msg)
			// Sync task toggles back to vault
			if m.projectMode.WasFileChanged() {
				m.refreshComponents("")
			}
			if !m.projectMode.IsActive() {
				if notePath, ok := m.projectMode.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
				if action, ok := m.projectMode.GetAction(); ok {
					return m.executeCommand(action)
				}
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.commandCenter.IsActive() {
			m.commandCenter, _ = m.commandCenter.Update(msg)
			if !m.commandCenter.IsActive() {
				// Handle consumed-once results.
				if m.commandCenter.ShouldStartPomodoro() {
					m.pomodoro.Open()
					m.pomodoro.Start()
					return m, nil
				}
				if task := m.commandCenter.CompletedTask(); task != nil {
					// Toggle task done in source note. Task.LineNum is 1-based
					// (see internal/tui/taskmanager.go:34), so we index with -1.
					if task.NotePath != "" && task.LineNum >= 1 {
						if note := m.vault.GetNote(task.NotePath); note != nil {
							lines := strings.Split(note.Content, "\n")
							idx := task.LineNum - 1
							if idx < len(lines) {
								lines[idx] = strings.Replace(lines[idx], "- [ ]", "- [x]", 1)
								newContent := strings.Join(lines, "\n")
								if err := atomicWriteNote(filepath.Join(m.vault.Root, task.NotePath), newContent); err != nil {
									m.statusbar.SetError("Failed to mark task done: " + err.Error())
								} else {
									m.refreshComponents(task.NotePath)
								}
							}
						}
					}
				}
				if projName := m.commandCenter.SelectedProject(); projName != "" {
					m.projectMode.SetSize(m.width, m.height)
					m.projectMode.Open(m.vault.Root)
				}
				if habitName := m.commandCenter.ToggledHabit(); habitName != "" {
					m.habitTracker.dailyNotesFolder = m.config.DailyNotesFolder
					m.habitTracker.Open(m.vault.Root)
					m.habitTracker.toggleToday(habitName)
					m.habitTracker.Close()
				}
			} else {
				// Handle consumed-once results while still active.
				if m.commandCenter.ShouldStartPomodoro() {
					m.commandCenter.Close()
					m.pomodoro.Open()
					m.pomodoro.Start()
					return m, nil
				}
				if task := m.commandCenter.CompletedTask(); task != nil {
					if task.NotePath != "" && task.LineNum >= 1 {
						if note := m.vault.GetNote(task.NotePath); note != nil {
							lines := strings.Split(note.Content, "\n")
							idx := task.LineNum - 1 // Task.LineNum is 1-based
							if idx < len(lines) {
								lines[idx] = strings.Replace(lines[idx], "- [ ]", "- [x]", 1)
								newContent := strings.Join(lines, "\n")
								if err := atomicWriteNote(filepath.Join(m.vault.Root, task.NotePath), newContent); err != nil {
									m.statusbar.SetError("Failed to mark task done: " + err.Error())
								} else {
									m.refreshComponents(task.NotePath)
								}
							}
						}
					}
				}
				if projName := m.commandCenter.SelectedProject(); projName != "" {
					m.commandCenter.Close()
					m.projectMode.SetSize(m.width, m.height)
					m.projectMode.Open(m.vault.Root)
				}
				if habitName := m.commandCenter.ToggledHabit(); habitName != "" {
					m.habitTracker.dailyNotesFolder = m.config.DailyNotesFolder
					m.habitTracker.Open(m.vault.Root)
					m.habitTracker.toggleToday(habitName)
					m.habitTracker.Close()
				}
			}
			return m, nil
		}

		if m.projectDashboard.IsActive() {
			m.projectDashboard, _ = m.projectDashboard.Update(msg)
			if !m.projectDashboard.IsActive() {
				if projName := m.projectDashboard.SelectedProject(); projName != "" {
					m.projectMode.SetSize(m.width, m.height)
					m.projectMode.Open(m.vault.Root)
				}
			}
			return m, nil
		}

		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			if !m.nlSearch.IsActive() {
				if notePath, ok := m.nlSearch.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
			}
			return m, cmd
		}

		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			if !m.writingCoach.IsActive() {
				if suggestion, ok := m.writingCoach.GetSuggestion(); ok {
					m.editor.InsertText(suggestion)
					m.statusbar.SetMessage("Writing suggestion applied")
				}
			}
			return m, cmd
		}

		if m.dataview.IsActive() {
			m.dataview, _ = m.dataview.Update(msg)
			if !m.dataview.IsActive() {
				if notePath, ok := m.dataview.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
			}
			return m, nil
		}

		if m.timeTracker.IsActive() {
			var cmd tea.Cmd
			m.timeTracker, cmd = m.timeTracker.Update(msg)
			return m, cmd
		}

		if m.knowledgeGaps.IsActive() {
			m.knowledgeGaps, _ = m.knowledgeGaps.Update(msg)
			if !m.knowledgeGaps.IsActive() {
				if notePath, ok := m.knowledgeGaps.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
				}
			}
			return m, nil
		}

		if m.weeklyReview.IsActive() {
			m.weeklyReview, _ = m.weeklyReview.Update(msg)
			if m.weeklyReview.WasFileChanged() {
				m.refreshComponents("")
			}
			return m, nil
		}

		if m.readingList.IsActive() {
			m.readingList, _ = m.readingList.Update(msg)
			return m, nil
		}

		if m.spellcheck.IsActive() {
			m.spellcheck, _ = m.spellcheck.Update(msg)
			if errMsg := m.spellcheck.ConsumeError(); errMsg != "" {
				m.statusbar.SetWarning(errMsg)
			}
			wasApplied := false
			if word, line, col, replacement, ok := m.spellcheck.GetCorrection(); ok {
				_ = word
				if line < len(m.editor.content) {
					lineStr := m.editor.content[line]
					if col+len(word) <= len(lineStr) {
						m.editor.content[line] = lineStr[:col] + replacement + lineStr[col+len(word):]
						m.editor.modified = true
						m.statusbar.SetMessage("Fixed: " + word + " → " + replacement)
						wasApplied = true
						// Re-open overlay with updated content to refresh positions
						m.spellcheck.Open(m.editor.GetContent())
					}
				}
			}
			if !m.spellcheck.IsActive() && m.spellcheck.InlineEnabled() {
				// Overlay closed — refresh inline highlights
				now := time.Now()
				m.lastSpellEditTime = now
				return m, ScheduleInlineCheck(now)
			}
			if wasApplied {
				return m, nil
			}
			return m, nil
		}

		if m.luaOverlay.IsActive() {
			var cmd tea.Cmd
			m.luaOverlay, cmd = m.luaOverlay.Update(msg)
			return m, cmd
		}

		if m.publisher.IsActive() {
			var cmd tea.Cmd
			m.publisher, cmd = m.publisher.Update(msg)
			return m, cmd
		}

		if m.splitPane.IsActive() {
			var cmd tea.Cmd
			m.splitPane, cmd = m.splitPane.Update(msg)
			return m, cmd
		}

		if m.flashcards.IsActive() {
			var cmd tea.Cmd
			m.flashcards, cmd = m.flashcards.Update(msg)
			if !m.flashcards.IsActive() {
				// Record review activity
				m.learnDash.RecordReview(time.Now().Format("2006-01-02"))
			}
			return m, cmd
		}

		if m.quizMode.IsActive() {
			var cmd tea.Cmd
			m.quizMode, cmd = m.quizMode.Update(msg)
			if !m.quizMode.IsActive() {
				m.learnDash.RecordReview(time.Now().Format("2006-01-02"))
			}
			return m, cmd
		}

		if m.learnDash.IsActive() {
			var cmd tea.Cmd
			m.learnDash, cmd = m.learnDash.Update(msg)
			return m, cmd
		}

		if m.aiChat.IsActive() {
			var cmd tea.Cmd
			m.aiChat, cmd = m.aiChat.Update(msg)
			return m, cmd
		}

		if m.composer.IsActive() {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			if !m.composer.IsActive() {
				if title, content, ok := m.composer.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("AI note created: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
			return m, cmd
		}

		if m.knowledgeGraph.IsActive() {
			var cmd tea.Cmd
			m.knowledgeGraph, cmd = m.knowledgeGraph.Update(msg)
			return m, cmd
		}

		if m.tableEditor.IsActive() {
			var cmd tea.Cmd
			m.tableEditor, cmd = m.tableEditor.Update(msg)
			if !m.tableEditor.IsActive() {
				if md, startLine, endLine, ok := m.tableEditor.GetResult(); ok {
					m.editor.saveSnapshot()
					newLines := strings.Split(md, "\n")
					if startLine < 0 {
						startLine = 0
					}
					if startLine > len(m.editor.content) {
						startLine = len(m.editor.content)
					}
					if endLine < 0 {
						endLine = 0
					}
					if endLine >= len(m.editor.content) {
						endLine = len(m.editor.content) - 1
					}
					if endLine < startLine {
						// Insert mode: insert before startLine
						before := make([]string, len(m.editor.content[:startLine]))
						copy(before, m.editor.content[:startLine])
						after := m.editor.content[startLine:]
						m.editor.content = append(before, append(newLines, after...)...)
						m.statusbar.SetMessage("Table inserted")
					} else {
						// Replace mode
						before := make([]string, len(m.editor.content[:startLine]))
						copy(before, m.editor.content[:startLine])
						var after []string
						if endLine+1 < len(m.editor.content) {
							after = m.editor.content[endLine+1:]
						}
						m.editor.content = append(before, append(newLines, after...)...)
						m.statusbar.SetMessage("Table updated")
					}
					m.editor.modified = true
				}
			}
			return m, cmd
		}

		if m.semanticSearch.IsActive() {
			var cmd tea.Cmd
			m.semanticSearch, cmd = m.semanticSearch.Update(msg)
			if !m.semanticSearch.IsActive() {
				if nav := m.semanticSearch.SelectedResult(); nav != "" {
					m.loadNote(nav)
					m.setSidebarCursorToFile(nav)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.threadWeaver.IsActive() {
			var cmd tea.Cmd
			m.threadWeaver, cmd = m.threadWeaver.Update(msg)
			if !m.threadWeaver.IsActive() {
				if title, content, ok := m.threadWeaver.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Thread woven: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
			return m, cmd
		}

		if m.noteChat.IsActive() {
			var cmd tea.Cmd
			m.noteChat, cmd = m.noteChat.Update(msg)
			return m, cmd
		}

		if m.vaultRefactor.IsActive() {
			var cmd tea.Cmd
			m.vaultRefactor, cmd = m.vaultRefactor.Update(msg)
			if !m.vaultRefactor.IsActive() {
				if plan, ok := m.vaultRefactor.GetResult(); ok {
					m.applyVaultRefactor(plan)
				}
			}
			return m, cmd
		}

		if m.dailyBriefing.IsActive() {
			var cmd tea.Cmd
			m.dailyBriefing, cmd = m.dailyBriefing.Update(msg)
			if !m.dailyBriefing.IsActive() {
				if content, ok := m.dailyBriefing.GetResult(); ok {
					m.writeBriefingToDailyNote(content)
				}
			}
			return m, cmd
		}

		if m.devotional.IsActive() {
			var cmd tea.Cmd
			m.devotional, cmd = m.devotional.Update(msg)
			return m, cmd
		}

		if m.morningRoutine.IsActive() {
			var cmd tea.Cmd
			m.morningRoutine, cmd = m.morningRoutine.Update(msg)
			if !m.morningRoutine.IsActive() {
				if m.morningRoutine.phase == morningComplete {
					m.statusbar.SetDayPlanned(true)
					// Refresh vault to pick up daily note changes
					_ = m.vault.Scan()
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetNoteCount(m.vault.NoteCount())
				}
				// If the user pressed P on the complete screen, continue
				// into Plan My Day so the AI can refine today's schedule
				// with the goal, tasks, and habits just captured.
				if m.morningRoutine.ConsumeAIRefineRequest() {
					var planModel tea.Model
					var planCmd tea.Cmd
					planModel, planCmd = m.executeCommand(CmdPlanMyDay)
					if mm, ok := planModel.(Model); ok {
						m = mm
					}
					return m, tea.Batch(cmd, planCmd)
				}
			}
			return m, cmd
		}

		if m.encryption.IsActive() {
			var cmd tea.Cmd
			m.encryption, cmd = m.encryption.Update(msg)
			if !m.encryption.IsActive() {
				if result, ok := m.encryption.GetResult(); ok {
					m.applyEncryptionResult(result)
				}
			}
			return m, cmd
		}

		if m.gitHistory.IsActive() {
			var cmd tea.Cmd
			m.gitHistory, cmd = m.gitHistory.Update(msg)
			if !m.gitHistory.IsActive() {
				if _, ok := m.gitHistory.GetRestoreResult(); ok {
					m.statusbar.SetMessage("Restoring from git...")
				}
			}
			return m, cmd
		}

		if m.workspace.IsActive() {
			var cmd tea.Cmd
			m.workspace, cmd = m.workspace.Update(msg)
			if !m.workspace.IsActive() {
				if layout, ok := m.workspace.GetLoadResult(); ok {
					m.applyWorkspaceLayout(layout)
				}
				if m.workspace.IsSaveRequested() {
					name := m.workspace.SaveName()
					layout := m.captureWorkspaceLayout(name)
					m.workspace.SaveLayout(layout)
					m.statusbar.SetMessage("Workspace saved: " + name)
				}
			}
			return m, cmd
		}

		if m.timeline.IsActive() {
			var cmd tea.Cmd
			m.timeline, cmd = m.timeline.Update(msg)
			if !m.timeline.IsActive() {
				if notePath, ok := m.timeline.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.setSidebarCursorToFile(notePath)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.taskManager.IsActive() {
			// Refresh task list if another component changed vault files
			if m.needsRefresh {
				m.taskManager.Refresh(m.vault)
				m.needsRefresh = false
			}
			var cmd tea.Cmd
			m.taskManager, cmd = m.taskManager.Update(msg)
			// Check if task manager wrote any files
			if m.taskManager.WasFileChanged() {
				changedNote := m.taskManager.ActiveNotePath()
				m.refreshComponents(changedNote)
			}
			// Handle jump result (closes overlay)
			if notePath, lineNum, ok := m.taskManager.GetJumpResult(); ok {
				m.loadNote(notePath)
				m.setSidebarCursorToFile(notePath)
				m.setFocus(focusEditor)
				if lineNum > 0 {
					m.editor.cursor = lineNum - 1
					m.editor.scroll = maxInt(0, lineNum-m.editor.height/2)
				}
			}
			// Launch focus session for selected task
			if task, ok := m.taskManager.GetFocusRequest(); ok {
				m.focusSession.SetSize(m.width, m.height)
				m.focusSession.OpenWithTask(m.vault.Root, task)
			}
			return m, cmd
		}

		if m.linkAssist.IsActive() {
			var cmd tea.Cmd
			m.linkAssist, cmd = m.linkAssist.Update(msg)
			if !m.linkAssist.IsActive() {
				if suggestions, ok := m.linkAssist.GetApplyResult(); ok && m.activeNote != "" {
					content := m.editor.GetContent()
					content = applyLinkSuggestions(content, suggestions)
					m.editor.LoadContent(content, m.editor.filePath)
					m.editor.modified = true
					m.statusbar.SetMessage(fmt.Sprintf("Linked %d mentions", len(suggestions)))
				}
			}
			return m, cmd
		}

		if m.blogPublisher.IsActive() {
			var cmd tea.Cmd
			m.blogPublisher, cmd = m.blogPublisher.Update(msg)
			return m, cmd
		}

		if m.themeEditor.IsActive() {
			var cmd tea.Cmd
			m.themeEditor, cmd = m.themeEditor.Update(msg)
			return m, cmd
		}

		if m.layoutPicker.IsActive() {
			var cmd tea.Cmd
			m.layoutPicker, cmd = m.layoutPicker.Update(msg)
			if !m.layoutPicker.IsActive() {
				if selected, ok := m.layoutPicker.GetResult(); ok {
					m.config.Layout = selected
					m.statusbar.SetMessage("Layout: " + LayoutDescription(selected))
					if !LayoutHasSidebar(m.config.Layout) && m.focus == focusSidebar {
						m.setFocus(focusEditor)
					}
					if !LayoutHasBacklinks(m.config.Layout) && m.focus == focusBacklinks {
						m.setFocus(focusEditor)
					}
					m.updateLayout()
					_ = m.config.Save()
				}
			}
			return m, cmd
		}

		if m.imageManager.IsActive() {
			var cmd tea.Cmd
			m.imageManager, cmd = m.imageManager.Update(msg)
			if !m.imageManager.IsActive() {
				if insertText, ok := m.imageManager.GetInsertResult(); ok && m.activeNote != "" {
					m.editor.InsertText(insertText)
					m.editor.modified = true
				}
			}
			return m, cmd
		}

		if m.statusTray.IsActive() {
			var cmd tea.Cmd
			m.statusTray, cmd = m.statusTray.Update(msg)
			return m, cmd
		}

		if m.research.IsActive() {
			var cmd tea.Cmd
			m.research, cmd = m.research.Update(msg)
			if !m.research.IsActive() {
				if filePath, ok := m.research.GetSelectedFile(); ok {
					m.loadNote(filePath)
					m.setSidebarCursorToFile(filePath)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.vaultSwitch.IsActive() {
			var cmd tea.Cmd
			m.vaultSwitch, cmd = m.vaultSwitch.Update(msg)
			if !m.vaultSwitch.IsActive() {
				if vaultPath, ok := m.vaultSwitch.GetSelectedVault(); ok {
					// Relaunch with new vault
					newModel, err := NewModel(vaultPath)
					if err != nil {
						m.statusbar.SetMessage("Failed to open vault: " + err.Error())
						return m, m.clearMessageAfter(5 * time.Second)
					}
					newModel.width = m.width
					newModel.height = m.height
					newModel.showSplash = false
					newModel.updateLayout()
					return &newModel, nil
				}
			}
			return m, cmd
		}

		if m.frontmatterEdit.IsActive() {
			var cmd tea.Cmd
			m.frontmatterEdit, cmd = m.frontmatterEdit.Update(msg)
			if !m.frontmatterEdit.IsActive() {
				if yamlBlock, ok := m.frontmatterEdit.GetResult(); ok && m.activeNote != "" {
					content := m.editor.GetContent()
					// Replace existing frontmatter or prepend new one
					newContent := replaceFrontmatter(content, yamlBlock)
					m.editor.LoadContent(newContent, m.editor.filePath)
					m.editor.modified = true
					m.statusbar.SetMessage("Frontmatter updated")
				}
			}
			return m, cmd
		}

		if m.kanban.IsActive() {
			var cmd tea.Cmd
			m.kanban, cmd = m.kanban.Update(msg)
			if !m.kanban.IsActive() {
				// Check if user toggled a task
				if notePath, line, newDone, ok := m.kanban.GetToggleResult(); ok {
					// Update the source note
					if note := m.vault.GetNote(notePath); note != nil {
						lines := strings.Split(note.Content, "\n")
						if line >= 0 && line < len(lines) {
							if newDone {
								lines[line] = strings.Replace(lines[line], "- [ ]", "- [x]", 1)
							} else {
								lines[line] = strings.Replace(lines[line], "- [x]", "- [ ]", 1)
							}
							newContent := strings.Join(lines, "\n")
							if err := atomicWriteNote(filepath.Join(m.vault.Root, notePath), newContent); err != nil {
								m.statusbar.SetError("Failed to update task: " + err.Error())
							} else {
								m.refreshComponents(notePath)
							}
						}
					}
				}
			}
			return m, cmd
		}

		if m.pomodoro.IsActive() {
			var cmd tea.Cmd
			m.pomodoro, cmd = m.pomodoro.Update(msg)
			m.syncPomodoroCompletions()
			m.syncPomodoroTimeRecords()
			return m, cmd
		}

		if m.webClipper.IsActive() {
			var cmd tea.Cmd
			m.webClipper, cmd = m.webClipper.Update(msg)
			if !m.webClipper.IsActive() {
				if title, content, ok := m.webClipper.GetResult(); ok {
					// Sanitize title for use as filename — prevent path traversal
					name := filepath.Base(title)
					name = strings.Map(func(r rune) rune {
						if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == '\x00' {
							return '_'
						}
						return r
					}, name)
					if name == "" || name == "." || name == ".." {
						name = "Untitled Clip"
					}
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := atomicWriteNote(path, content); err == nil {
							if err := m.vault.Scan(); err != nil {
								log.Printf("warning: vault scan failed: %v", err)
							}
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.setSidebarCursorToFile(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Web clip saved: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
			return m, cmd
		}

		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y", "enter":
				m.confirmDelete = false
				if m.confirmDeleteNote != "" {
					if err := m.trash.MoveToTrash(m.confirmDeleteNote); err == nil {
						if err := m.vault.Scan(); err != nil {
							log.Printf("warning: vault scan failed: %v", err)
						}
						m.index = vault.NewIndex(m.vault)
						m.index.Build()
						paths := m.vault.SortedPaths()
						m.sidebar.SetFiles(paths)
						m.autocomplete.SetNotes(paths)
						m.statusbar.SetNoteCount(m.vault.NoteCount())
						m.statusbar.SetMessage("Moved to trash: " + m.confirmDeleteNote)
						if len(paths) > 0 {
							m.loadNote(paths[0])
						} else {
							m.activeNote = ""
							m.editor.SetContent("")
							m.statusbar.SetActiveNote("")
						}
					}
					m.confirmDeleteNote = ""
				}
				return m, m.clearMessageAfter(2 * time.Second)
			case "n", "N", "esc":
				m.confirmDelete = false
				m.confirmDeleteNote = ""
				return m, nil
			}
			return m, nil
		}

		if m.pendingReload {
			switch msg.String() {
			case "y", "Y":
				m.pendingReload = false
				notePath := m.pendingReloadPath
				m.pendingReloadPath = ""
				curLine, curCol := m.editor.GetCursor()
				curScroll := m.editor.scroll
				if note := m.vault.GetNote(notePath); note != nil {
					m.editor.LoadContent(note.Content, notePath)
					m.editor.cursor = curLine
					m.editor.col = curCol
					if m.editor.cursor >= len(m.editor.content) {
						m.editor.cursor = len(m.editor.content) - 1
					}
					if m.editor.cursor < 0 {
						m.editor.cursor = 0
					}
					if len(m.editor.content) > 0 {
						if m.editor.col > len(m.editor.content[m.editor.cursor]) {
							m.editor.col = len(m.editor.content[m.editor.cursor])
						}
					}
					m.editor.scroll = curScroll
				}
				baseName := filepath.Base(notePath)
				var toastCmd tea.Cmd
				if m.toast != nil {
					toastCmd = m.toast.ShowInfo("Reloaded: " + baseName)
				}
				return m, toastCmd
			case "n", "N", "esc":
				m.pendingReload = false
				m.pendingReloadPath = ""
				var toastCmd tea.Cmd
				if m.toast != nil {
					toastCmd = m.toast.ShowInfo("Kept local changes")
				}
				return m, toastCmd
			}
			return m, nil
		}

		if m.newFolderMode {
			return m.updateNewFolder(msg)
		}

		if m.moveFileMode {
			return m.updateMoveFile(msg)
		}

		if m.commandPalette.IsActive() {
			m.commandPalette, _ = m.commandPalette.Update(msg)
			if action := m.commandPalette.Result(); action != CmdNone {
				return m.executeCommand(action)
			}
			return m, nil
		}

		if m.searchMode {
			return m.updateSearch(msg)
		}

		if m.newNoteMode {
			return m.updateNewNote(msg)
		}

		if m.extractMode {
			return m.updateExtractNote(msg)
		}

		// Selection copy: intercept Ctrl+C before global quit handler when editor has selection
		// Works in both edit and view mode — copying is a read-only operation.
		if m.focus == focusEditor && m.editor.HasSelection() {
			if msg.String() == "ctrl+c" {
				text := m.editor.GetSelectedText()
				if text != "" {
					if err := ClipboardCopy(text); err != nil {
						m.statusbar.SetMessage("Copy failed: " + err.Error())
					} else {
						m.statusbar.SetMessage("Copied to clipboard")
					}
					m.clipManager.AddClip(text, m.activeNote)
					m.editor.ClearSelection()
					return m, m.clearMessageAfter(2 * time.Second)
				}
			}
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			if m.showExitSplash {
				m.quitting = true
				return m, tea.Quit
			}
			return m, m.triggerExitSplash()

		case "ctrl+s":
			cmd := m.saveCurrentNote()
			m.lastSaveTime = time.Now()
			m.cachedTasks = ParseAllTasks(m.vault.Notes)
			m.dueTodayCount = CountTasksDueTodayFromList(m.cachedTasks)
			m.statusbar.SetDueTodayCount(m.dueTodayCount)
			m.statusbar.SetOverdueCount(CountOverdueTasksFromList(m.cachedTasks))
			return m, cmd

		case "f1", "alt+1":
			m.setFocus(focusSidebar)
			return m, nil

		case "f2", "alt+2":
			m.setFocus(focusEditor)
			return m, nil

		case "f3", "alt+3":
			m.setFocus(focusBacklinks)
			return m, nil

		case "f4":
			// Rename note
			if m.activeNote != "" {
				m.newNoteMode = true
				m.newNoteName = strings.TrimSuffix(m.activeNote, ".md")
			}
			return m, nil

		case "f5", "alt+?":
			m.helpOverlay.SetSize(m.width, m.height)
			m.helpOverlay.Toggle()
			return m, nil

		case "tab":
			if m.focus != focusEditor {
				m.cycleFocus(1)
				return m, nil
			}

		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil

		case "ctrl+p":
			m.searchMode = true
			m.searchQuery = ""
			m.searchResults = m.vault.SortedPaths()
			m.searchCursor = 0
			return m, nil

		case "ctrl+n":
			// Show template picker first, then name input
			m.templates.SetSize(m.width, m.height)
			m.templates.OpenWithVault(m.vault.Root)
			return m, nil

		case "ctrl+e":
			m.viewMode = !m.viewMode
			if m.viewMode {
				m.renderer.SetViewStyle("reading")
				m.statusbar.SetMode("VIEW")
				m.statusbar.SetViewMode(true)
				m.viewScroll = 0
				m.updateReadingProgress()
			} else {
				m.renderer.SetViewStyle(m.config.ViewStyle)
				m.statusbar.SetMode("EDIT")
				m.statusbar.SetViewMode(false)
			}
			return m, nil

		case "ctrl+,":
			m.settings.SetConfig(m.config)
			m.settings.SetSize(m.width, m.height)
			m.settings.Toggle()
			return m, nil

		case "ctrl+g":
			m.graphView.SetSize(m.width, m.height)
			m.graphView.Open(m.activeNote)
			return m, nil

		case "ctrl+t":
			m.tagBrowser.SetSize(m.width, m.height)
			m.tagBrowser.Open()
			return m, nil

		case "ctrl+x":
			// Cut selected text when editing with a selection; otherwise open command palette
			if m.focus == focusEditor && !m.viewMode && m.editor.HasSelection() {
				text := m.editor.GetSelectedText()
				if text != "" {
					if err := ClipboardCopy(text); err != nil {
						m.statusbar.SetMessage("Cut failed: " + err.Error())
					} else {
						m.statusbar.SetMessage("Cut to clipboard")
					}
					m.clipManager.AddClip(text, m.activeNote)
					m.editor.DeleteSelection()
					line, col := m.editor.GetCursor()
					m.statusbar.SetCursor(line, col)
					m.statusbar.SetWordCount(m.editor.GetWordCount())
					return m, m.clearMessageAfter(2 * time.Second)
				}
			}
			m.commandPalette.SetSize(m.width, m.height)
			m.commandPalette.Open()
			return m, nil

		case "ctrl+/":
			m.universalSearch.SetSize(m.width, m.height)
			allTasks := ParseAllTasks(m.vault.Notes)
			gm := NewGoalsMode()
			gm.vaultRoot = m.vault.Root
			gm.loadGoals()
			var habits []habitEntry
			if m.habitTracker.habits != nil {
				habits = m.habitTracker.habits
			}
			m.universalSearch.Open(m.vault.Notes, allTasks, gm.goals, habits)
			return m, nil

		case "ctrl+k":
			m.taskManager.SetSize(m.width, m.height)
			m.taskManager.config = m.config
			m.taskManager.ai = m.aiConfig()
			m.taskManager.Open(m.vault)
			return m, nil

		case "ctrl+o":
			m.outline.SetSize(m.width, m.height)
			m.outline.Open(m.editor.GetContent())
			return m, nil

		case "ctrl+b":
			m.bookmarks.SetSize(m.width, m.height)
			m.bookmarks.Open()
			return m, nil

		case "ctrl+f":
			m.findReplace.SetSize(m.width, m.height)
			m.findReplace.OpenFind(m.vault.Root)
			m.findReplace.UpdateMatches(m.editor.content)
			return m, nil

		case "ctrl+h":
			m.findReplace.SetSize(m.width, m.height)
			m.findReplace.OpenReplace(m.vault.Root)
			m.findReplace.UpdateMatches(m.editor.content)
			return m, nil

		case "ctrl+j":
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
			return m, nil

		case "ctrl+w":
			// Close active tab
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
			}
			return m, nil

		case "ctrl+l":
			m.calendar.SetSize(m.width, m.height)
			m.calendar.SetVaultRoot(m.vault.Root)
			m.calendar.SetActiveGoals(loadActiveGoals(m.vault.Root))
			// Pass note contents for task parsing
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
			ht := NewHabitTracker()
			ht.Open(m.vault.Root)
			m.calendar.SetHabitData(ht.habits, ht.logs)
			m.calendar.Open()
			return m, nil

		case "ctrl+r":
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
			return m, nil

		case "ctrl+z":
			if m.focusMode.IsActive() {
				m.focusMode.Close()
			} else {
				m.focusMode.SetSize(m.width, m.height)
				m.focusMode.Open(m.editor.GetWordCount())
				m.setFocus(focusEditor)
			}
			return m, nil

		case "alt+g":
			if m.focusMode.IsActive() {
				m.focusMode.OpenGoalPrompt()
				return m, nil
			}

		case "ctrl+tab":
			// Cycle to next tab
			if m.tabBar != nil {
				if path := m.tabBar.NextTab(); path != "" && path != m.activeNote {
					m.loadNote(path)
					m.setSidebarCursorToFile(path)
				}
			}
			return m, nil

		case "ctrl+shift+tab":
			// Cycle to previous tab
			if m.tabBar != nil {
				if path := m.tabBar.PrevTab(); path != "" && path != m.activeNote {
					m.loadNote(path)
					m.setSidebarCursorToFile(path)
				}
			}
			return m, nil

		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5",
			"ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9":
			if m.tabBar != nil {
				// Parse digit from key string (last character)
				keyStr := msg.String()
				digit := int(keyStr[len(keyStr)-1] - '0')
				idx := digit - 1 // 1-indexed to 0-indexed
				if path := m.tabBar.SwitchToIndex(idx); path != "" {
					if path != m.activeNote {
						m.loadNote(path)
						m.setSidebarCursorToFile(path)
					}
				}
			}
			return m, nil

		case "alt+left":
			if m.breadcrumb != nil {
				if nav := m.breadcrumb.Back(); nav != "" {
					m.loadNoteWithoutBreadcrumb(nav)
					m.setSidebarCursorToFile(nav)
				}
			}
			return m, nil

		case "alt+right":
			if m.breadcrumb != nil {
				if nav := m.breadcrumb.Forward(); nav != "" {
					m.loadNoteWithoutBreadcrumb(nav)
					m.setSidebarCursorToFile(nav)
				}
			}
			return m, nil

		case "alt+shift+left":
			// Move active tab left
			if m.tabBar != nil {
				m.tabBar.MoveLeft()
			}
			return m, nil

		case "alt+shift+right":
			// Move active tab right
			if m.tabBar != nil {
				m.tabBar.MoveRight()
			}
			return m, nil

		case "alt+d":
			m.openDailyNote()
			return m, nil

		case "alt+h":
			return m.executeCommand(CmdDashboard)

		case "alt+j":
			return m.executeCommand(CmdDailyJot)

		case "alt+m":
			return m.executeCommand(CmdMorningRoutine)

		case "alt+l":
			return m.executeCommand(CmdLayoutPicker)

		case "alt+e":
			return m.executeCommand(CmdDailyReview)

		case "alt+p":
			return m.executeCommand(CmdPlanMyDay)

		case "alt+[":
			m.navigateDailyNote(-1)
			return m, nil

		case "alt+]":
			m.navigateDailyNote(1)
			return m, nil

		case "alt+f":
			// Toggle fold at cursor
			if m.activeNote != "" {
				m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
			}
			return m, nil

		case "alt+r":
			// Open research agent with current note as context
			if m.activeNote != "" && !m.research.IsRunning() {
				m.research.SetSize(m.width, m.height)
				m.research.OpenNoteEnhance(m.vault.Root, m.activeNote, m.editor.GetContent(), m.vault.SortedPaths())
			} else if !m.research.IsRunning() {
				m.research.SetSize(m.width, m.height)
				m.research.Open(m.vault.Root, m.vault.SortedPaths(), m.activeNote)
			}
			return m, nil

		case "alt+b":
			// Habit tracker
			return m.executeCommand(CmdHabitTracker)

		case "alt+t":
			// Time tracker
			return m.executeCommand(CmdTimeTracker)

		case "alt+i":
			// Quick capture (inbox)
			return m.executeCommand(CmdQuickCapture)

		case "alt+s":
			// Quick focus session
			return m.executeCommand(CmdFocusSession)

		case "alt+w":
			// Weekly note
			return m.executeCommand(CmdWeeklyNote)

		case "alt+c":
			// Command Center
			return m.executeCommand(CmdCommandCenter)

		case "alt+C":
			// Toggle calendar panel in right sidebar
			m.rightPanelCalendar = !m.rightPanelCalendar
			if m.rightPanelCalendar {
				m.calendarPanel.SetVaultRoot(m.vault.Root)
				noteContents := make(map[string]string)
				for _, p := range m.vault.SortedPaths() {
					if note := m.vault.GetNote(p); note != nil {
						noteContents[p] = note.Content
					}
				}
				panelBlocks, _ := loadPlannerBlocks(m.vault.Root)
				m.calendarPanel.Refresh(panelBlocks, noteContents)
				m.statusbar.SetMessage("Calendar panel enabled (Alt+Shift+C to toggle)")
			} else {
				m.statusbar.SetMessage("Backlinks panel restored (Alt+Shift+C to toggle)")
			}
			return m, nil

		case "esc":
			// If selection is active, clear it first
			if m.focus == focusEditor && !m.viewMode && m.editor.HasSelection() {
				m.editor.ClearSelection()
				return m, nil
			}
			// If multi-cursors are active, clear them first
			if m.focus == focusEditor && !m.viewMode && m.editor.HasMultiCursors() {
				m.editor.clearMultiCursors()
				return m, nil
			}
			if m.viewMode {
				m.viewMode = false
				m.statusbar.SetMode("EDIT")
				m.statusbar.SetViewMode(false)
				return m, nil
			}
			if m.focus == focusEditor || m.focus == focusBacklinks {
				m.setFocus(focusSidebar)
				return m, nil
			}

		case "enter":
			if m.focus == focusSidebar {
				selected := m.sidebar.Selected()
				if selected != "" {
					m.loadNote(selected)
					m.setFocus(focusEditor)
					return m, nil
				}
				// No file selected (directory node) — fall through so the
				// sidebar/filetree can handle enter to toggle expand/collapse.
			}
			if m.focus == focusBacklinks {
				selected := m.backlinks.Selected()
				if selected != "" {
					notePart, anchor := splitLinkAnchor(selected)
					if notePart == "" && anchor != "" {
						// Same-note anchor like [[#heading]]
						m.navigateToHeading(anchor)
					} else {
						resolved := m.resolveLink(selected)
						if resolved != "" {
							m.loadNote(resolved)
							m.navigateToHeading(anchor)
							m.setSidebarCursorToFile(resolved)
						}
					}
				}
				return m, nil
			}
		}

		// View mode scrolling
		if m.viewMode && m.focus == focusEditor {
			switch msg.String() {
			case "up", "k":
				if m.viewScroll > 0 {
					m.viewScroll--
				}
				m.updateReadingProgress()
				return m, nil
			case "down", "j":
				m.viewScroll++
				m.updateReadingProgress()
				return m, nil
			case "pgup", "ctrl+u":
				m.viewScroll -= m.height / 2
				if m.viewScroll < 0 {
					m.viewScroll = 0
				}
				m.updateReadingProgress()
				return m, nil
			case "pgdown", "ctrl+d", " ":
				m.viewScroll += m.height / 2
				m.updateReadingProgress()
				return m, nil
			case "home", "g":
				m.viewScroll = 0
				m.updateReadingProgress()
				return m, nil
			case "end", "G":
				totalLines := m.renderer.RenderLineCount(m.editor.GetContent())
				vpH := m.renderer.height - 4
				if vpH < 1 {
					vpH = 1
				}
				m.viewScroll = totalLines - vpH
				if m.viewScroll < 0 {
					m.viewScroll = 0
				}
				m.updateReadingProgress()
				return m, nil
			}
		}
	}

	// Delegate to focused component
	var cmd tea.Cmd
	switch m.focus {
	case focusSidebar:
		m.sidebar, cmd = m.sidebar.Update(msg)
	case focusEditor:
		if !m.viewMode {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				k := keyMsg.String()

				// Bracketed paste: handle text pasted via terminal (Ctrl+Shift+V, right-click, etc.)
				// Must run before link completer / slash menu to avoid desync.
				if keyMsg.Paste {
					// Dismiss popups that would misinterpret paste content
					if m.linkCompleter != nil && m.linkCompleter.IsActive() {
						m.linkCompleter.Deactivate()
					}
					if m.slashMenu != nil && m.slashMenu.IsActive() {
						m.slashMenu.Close()
					}
					text := string(keyMsg.Runes)
					if text == "" {
						return m, nil // empty paste, nothing to do
					}
					if m.editor.HasSelection() {
						m.editor.DeleteSelection()
					}
					if m.editor.SmartPaste(text) {
						m.statusbar.SetMessage("Smart paste: created markdown link")
					} else {
						m.editor.InsertText(text)
					}
					m.clipManager.AddClip(text, m.activeNote)
					line, col := m.editor.GetCursor()
					m.statusbar.SetCursor(line, col)
					m.statusbar.SetWordCount(m.editor.GetWordCount())
					return m, nil
				}

				// Link completer intercepts keys when active
				if m.linkCompleter != nil && m.linkCompleter.IsActive() {
					switch k {
					case "esc":
						m.linkCompleter.Deactivate()
						return m, nil
					case "tab", "enter":
						result := m.linkCompleter.Confirm()
						if result != "" {
							// Remove the query text typed after [[ and insert the completed link
							q := m.linkCompleter.GetQuery()
							// Remove query chars already typed
							for range q {
								m.editor.col--
								if m.editor.col >= 0 && m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
									line := m.editor.content[m.editor.cursor]
									if m.editor.col < len(line) {
										m.editor.content[m.editor.cursor] = line[:m.editor.col] + line[m.editor.col+1:]
									}
								}
							}
							if m.editor.col < 0 {
								m.editor.col = 0
							}
							m.editor.InsertText(result + "]]")
							m.editor.modified = true
						}
						m.linkCompleter.Deactivate()
						return m, nil
					case "up":
						m.linkCompleter.MoveUp()
						return m, nil
					case "down":
						m.linkCompleter.MoveDown()
						return m, nil
					case "backspace":
						if m.linkCompleter.GetQuery() == "" {
							m.linkCompleter.Deactivate()
						} else {
							m.linkCompleter.RemoveChar()
						}
						// Also pass through to editor
						m.editor, cmd = m.editor.Update(msg)
						return m, cmd
					default:
						if len(k) == 1 && k[0] >= 32 {
							m.linkCompleter.AddChar(k)
						}
						// Pass through to editor
						m.editor, cmd = m.editor.Update(msg)
						return m, cmd
					}
				}

				// Slash menu intercepts keys when active
				if m.slashMenu != nil && m.slashMenu.IsActive() {
					queryLen := m.slashMenu.QueryLen()
					insertText, consumed, closed := m.slashMenu.HandleKey(k)
					if insertText != "" {
						// Delete the "/" and query chars that triggered the menu
						if m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
							line := m.editor.content[m.editor.cursor]
							// Remove "/" + query chars (slash is at col - queryLen - 1)
							slashCol := m.editor.col - queryLen - 1
							if slashCol < 0 {
								slashCol = 0
							}
							if m.editor.col > len(line) {
								m.editor.col = len(line)
							}
							if slashCol < len(line) {
								m.editor.content[m.editor.cursor] = line[:slashCol] + line[m.editor.col:]
								m.editor.col = slashCol
							}
						}
						// Expand placeholders
						expanded := m.snippets.ExpandPlaceholders(insertText)
						m.editor.InsertText(expanded)
						m.editor.modified = true
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
					if closed {
						return m, nil
					}
					if consumed {
						return m, nil
					}
				}

				// Clipboard: Ctrl+C copies selection, Ctrl+V pastes (smart paste for URLs)
				if k == "ctrl+v" {
					text, err := ClipboardPaste()
					if err != nil {
						m.statusbar.SetMessage("Clipboard error: " + err.Error())
						return m, m.clearMessageAfter(3 * time.Second)
					}
					if text != "" {
						if m.editor.HasSelection() {
							m.editor.DeleteSelection()
						}
						if m.editor.SmartPaste(text) {
							m.statusbar.SetMessage("Smart paste: created markdown link")
						} else {
							m.editor.InsertText(text)
						}
						m.clipManager.AddClip(text, m.activeNote)
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
				}

				// Ghost writer: accept suggestion with Tab
				if k == "tab" {
					if m.ghostWriter != nil && m.ghostWriter.GetSuggestion() != "" {
						suggestion := m.ghostWriter.Accept()
						m.editor.InsertText(suggestion)
						m.editor.SetGhostText("")
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
				}

				// Escape dismisses ghost text
				if k == "esc" && m.ghostWriter != nil && m.ghostWriter.GetSuggestion() != "" {
					m.ghostWriter.Dismiss()
					m.editor.SetGhostText("")
					return m, nil
				}

				// Vim mode handling
				if m.vimState != nil && m.vimState.IsEnabled() {
					result := m.vimState.HandleKey(k, m.editor.content, m.editor.cursor, m.editor.col, m.editor.height)
					// Record keystroke for macro (skip keys that start/stop recording)
					if m.vimState.IsRecording() && !result.MacroStop && result.MacroStart == 0 {
						if keyMsg, ok := msg.(tea.KeyMsg); ok {
							m.vimState.RecordKey(keyMsg)
						}
					}
					cmd = m.applyVimResult(result)
					if !result.PassThrough {
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						if m.vimState.IsEnabled() {
							mode := "VIM:" + m.vimState.ModeString()
							if rs := m.vimState.RecordingStatus(); rs != "" {
								mode += " [" + rs + "]"
							}
							m.statusbar.SetMode(mode)
						}
						return m, cmd
					}
				}
			}

			m.editor, cmd = m.editor.Update(msg)
			// Snippet expansion: when space is typed, check if word before cursor is a snippet trigger
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == " " {
				m.tryExpandSnippet()
			}
			line, col := m.editor.GetCursor()
			m.statusbar.SetCursor(line, col)
			m.statusbar.SetWordCount(m.editor.GetWordCount())

			// Live backlink preview: update on every cursor move
			m.backlinkPreview.Update(m.editor.content, m.editor.cursor, m.editor.col, func(name string) string {
				// Strip heading anchor before looking up note content.
				notePart, _ := splitLinkAnchor(name)
				// Try with and without .md extension
				if note := m.vault.GetNote(notePart + ".md"); note != nil {
					return note.Content
				}
				if note := m.vault.GetNote(notePart); note != nil {
					return note.Content
				}
				return ""
			})

			// Detect [[ for link completion
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "[" {
				if m.linkCompleter != nil && !m.linkCompleter.IsActive() &&
					m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
					// Check if the char before cursor is also [
					curLine := m.editor.content[m.editor.cursor]
					c := m.editor.col
					if c >= 2 && c <= len(curLine) && curLine[c-2:c] == "[[" {
						m.linkCompleter.Activate(m.editor.cursor, m.editor.col)
					}
				}
			}

			// Detect "/" at start of line or after space for slash menu
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "/" {
				if m.slashMenu != nil && !m.slashMenu.IsActive() &&
					m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
					curLine := m.editor.content[m.editor.cursor]
					c := m.editor.col
					// "/" is valid if at col 1 (just typed at start) or after a space
					if c == 1 || (c >= 2 && c <= len(curLine) && curLine[c-2] == ' ') {
						m.slashMenu.Activate(m.editor.cursor, m.editor.col)
					}
				}
			}

			// Ghost writer: trigger completion on edits
			if m.ghostWriter != nil && m.ghostWriter.IsEnabled() {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					// Only trigger on actual text input, not navigation
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" {
						m.ghostWriter.Dismiss()
						m.editor.SetGhostText("")
						ghostCmd := m.ghostWriter.OnEdit(m.editor.content, m.editor.cursor, m.editor.col)
						if ghostCmd != nil {
							cmd = tea.Batch(cmd, ghostCmd)
						}
					}
				}
			}

			// Pomodoro: track word count during work sessions
			if m.pomodoro.IsRunning() {
				m.pomodoro.UpdateWordCount(m.editor.GetWordCount())
			}

			// Auto-save: debounce 2 seconds after last edit
			if m.config.AutoSave && m.editor.modified {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" || k == "tab" || k == "delete" {
						now := time.Now()
						m.lastEditTime = now
						autoSaveCmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
							return autoSaveTickMsg{editTime: now}
						})
						cmd = tea.Batch(cmd, autoSaveCmd)
					}
				}
			}

			// Inline spell check: debounce 1 second after last edit
			if m.spellcheck.InlineEnabled() {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" || k == "tab" || k == "delete" {
						now := time.Now()
						m.lastSpellEditTime = now
						cmd = tea.Batch(cmd, ScheduleInlineCheck(now))
					}
				}
			}
		}
	case focusBacklinks:
		m.backlinks, cmd = m.backlinks.Update(msg)
		// Insert suggested link into editor
		if linkPath, ok := m.backlinks.GetInsertLink(); ok {
			linkName := strings.TrimSuffix(filepath.Base(linkPath), ".md")
			m.editor.InsertText("[[" + linkName + "]]")
			m.editor.modified = true
			m.setFocus(focusEditor)
		}
	}

	return m, cmd
}
