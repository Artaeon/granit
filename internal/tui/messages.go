package tui

// messages.go is a central reference for all message types used across the TUI.
//
// Message types in Bubble Tea are dispatched through the Update() method. Each
// overlay or subsystem defines its own message types in its source file. This
// file serves as an index to help developers find where each message is defined
// and which component handles it.
//
// Message type naming convention: <component><Action>Msg
//   - Result messages:  <component>ResultMsg   (async operation completed)
//   - Tick messages:    <component>TickMsg      (animation/timer tick)
//   - Done messages:    <component>DoneMsg      (component finished)
//
// Core messages (app.go):
//   clearMessageMsg      — Clears the status bar message
//   autoSaveTickMsg      — Debounced auto-save trigger
//   saveResultMsg        — Result of a save operation
//
// AI messages:
//   aiChatResultMsg      — aichat.go:    AI chat response received
//   aiChatTickMsg        — aichat.go:    AI chat loading animation
//   aiSchedulerResultMsg — aischeduler.go: Scheduler AI response
//   aiSchedulerTickMsg   — aischeduler.go: Scheduler loading tick
//   aiTemplateResultMsg  — aitemplates.go: Template AI response
//   aiTemplateTickMsg    — aitemplates.go: Template loading tick
//   composerResultMsg    — composer.go:  Composer AI response
//   composerTickMsg      — composer.go:  Composer loading tick
//   briefingResultMsg    — dailybriefing.go: Briefing AI response
//   briefingTickMsg      — dailybriefing.go: Briefing loading tick
//   nlSearchResultMsg    — nlsearch.go:  NL search AI response
//   nlSearchTickMsg      — nlsearch.go:  NL search loading tick
//   writingCoachResultMsg — writingcoach.go: Writing coach response
//   writingCoachTickMsg  — writingcoach.go: Writing coach loading tick
//   vaultRefactorResultMsg — vaultrefactor.go: Refactor AI response
//   vaultRefactorTickMsg — vaultrefactor.go: Refactor loading tick
//   planMyDayResultMsg   — planmyday.go: Plan AI response
//   planMyDayTickMsg     — planmyday.go: Plan loading tick
//   planMyDayGatherMsg   — planmyday.go: Gather task data signal
//
// Bot messages:
//   ollamaResultMsg      — bots.go:     Ollama API response
//   openaiResultMsg      — bots.go:     OpenAI API response
//   botsTickMsg          — bots.go:     Bots loading animation
//   autoTagResultMsg     — autotag.go:  Auto-tagger result
//   atOllamaChatMsg      — autotag.go:  Auto-tagger Ollama response
//   atOpenAIMsg          — autotag.go:  Auto-tagger OpenAI response
//   noteChatResultMsg    — autotag.go:  Note chat response
//   noteChatTickMsg      — autotag.go:  Note chat loading tick
//   ghostSuggestionMsg   — ghostwriter.go: Ghost text suggestion
//   ghostDebounceMsg     — ghostwriter.go: Ghost text debounce
//   threadWeaverResultMsg — threadweaver.go: Thread weaver response
//   threadWeaverTickMsg  — threadweaver.go: Thread weaver loading tick
//   twOllamaMsg          — threadweaver.go: Thread weaver Ollama
//   twOpenAIMsg          — threadweaver.go: Thread weaver OpenAI
//
// Search & Embeddings:
//   semanticBuildMsg     — embeddings.go: Embedding build progress
//   semanticSearchMsg    — embeddings.go: Search results
//   semanticTickMsg      — embeddings.go: Loading animation
//   semanticBgIndexMsg   — embeddings.go: Background index done
//
// Git & Sync:
//   gitCmdResultMsg      — git.go:      Git command result
//   gitHistoryResultMsg  — githistory.go: Git history loaded
//   autoSyncResultMsg    — autosync.go: Auto-sync result
//
// Plugins & Scripts:
//   pluginCmdResultMsg   — plugins.go:  Plugin execution result
//   luaRunResultMsg      — luaoverlay.go: Lua script result
//   researchResultMsg    — research.go: Research agent result
//   researchTickMsg      — research.go: Research loading tick
//
// Publishing:
//   blogPublishResultMsg — blogpublish.go: Blog publish result
//   publishResultMsg     — publish.go:  Publish result
//   publishProgressMsg   — publish.go:  Publish progress update
//
// UI & Timer:
//   splashTickMsg        — splash.go:   Splash animation tick
//   exitTickMsg          — splash.go:   Exit splash animation tick
//   pomodoroTickMsg      — pomodoro.go: Pomodoro timer tick
//   focusSessionTickMsg  — focussession.go: Focus session tick
//   timeTrackerTickMsg   — timetracker.go: Time tracker tick
//   toastExpireMsg       — toast.go:    Toast notification expired
//   webClipTickMsg       — clipboard.go: Web clipper loading tick
//
// Editor & Spell:
//   spellCheckDoneMsg    — spellcheck.go: Spell check completed
//   spellCheckTickMsg    — spellcheck.go: Spell check debounce
//   fileChangeMsg        — watcher.go:  External file changed
//   splitPanePickMsg     — splitpane.go: Split pane file selected
//   vimMacroReplayMsg    — vim.go:      Vim macro replay step
//
// Settings & Config:
//   ollamaSetupMsg       — settings.go: Ollama install wizard
//   tutorialSaveErrMsg   — tutorial.go: Tutorial save failed
//   noteHistoryResultMsg — notehistory.go: Note history loaded
