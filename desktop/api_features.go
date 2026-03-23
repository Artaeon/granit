package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// ==================== Templates ====================

type TemplateInfo struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	IsUser  bool   `json:"isUser"`
}

var builtinTemplates = []TemplateInfo{
	{Name: "Blank Note", Content: ""},
	{Name: "Standard Note", Content: "---\ntitle: {{title}}\ndate: {{date}}\ntags: []\n---\n\n# {{title}}\n\n"},
	{Name: "Meeting Notes", Content: "---\ntitle: Meeting Notes\ndate: {{date}}\ntype: meeting\ntags: [meeting]\n---\n\n# Meeting Notes\n\n## Attendees\n-\n\n## Agenda\n1.\n\n## Notes\n\n\n## Action Items\n- [ ]\n"},
	{Name: "Project Plan", Content: "---\ntitle: Project Plan\ndate: {{date}}\ntype: project\ntags: [project]\n---\n\n# Project Plan\n\n## Overview\n\n\n## Goals\n-\n\n## Timeline\n| Phase | Start | End | Status |\n|-------|-------|-----|--------|\n|       |       |     |        |\n\n## Tasks\n- [ ]\n\n## Resources\n-\n"},
	{Name: "Weekly Review", Content: "---\ntitle: Weekly Review\ndate: {{date}}\ntype: review\ntags: [weekly, review]\n---\n\n# Weekly Review - {{date}}\n\n## Accomplishments\n-\n\n## Challenges\n-\n\n## Next Week\n- [ ]\n\n## Notes\n\n"},
	{Name: "Book Notes", Content: "---\ntitle: Book Notes\ndate: {{date}}\nauthor: \"\"\ntype: book\ntags: [book, notes]\n---\n\n# Book Notes\n\n## Summary\n\n\n## Key Ideas\n1.\n\n## Quotes\n>\n\n## Thoughts\n\n"},
	{Name: "Decision Record", Content: "---\ntitle: Decision Record\ndate: {{date}}\nstatus: proposed\ntype: decision\ntags: [decision]\n---\n\n# Decision Record\n\n## Context\n\n\n## Decision\n\n\n## Consequences\n\n### Positive\n-\n\n### Negative\n-\n\n### Risks\n-\n"},
	{Name: "Journal Entry", Content: "---\ntitle: Journal - {{date}}\ndate: {{date}}\ntype: journal\ntags: [journal]\n---\n\n# {{date}}\n\n## Mood\n\n\n## What happened today\n\n\n## Gratitude\n1.\n2.\n3.\n\n## Tomorrow\n- [ ]\n"},
	{Name: "Research Note", Content: "---\ntitle: {{title}}\ndate: {{date}}\ntype: research\ntags: [research]\nsource: \"\"\n---\n\n# {{title}}\n\n## Key Findings\n\n\n## Methodology\n\n\n## Data / Evidence\n\n\n## Questions\n-\n\n## Related Notes\n-\n"},
	{Name: "Zettelkasten", Content: "---\ntitle: {{title}}\ndate: {{date}}\ntype: zettel\ntags: []\n---\n\n# {{title}}\n\n## Main Idea\n\n\n## In My Own Words\n\n\n## Source\n\n\n## Connections\n- [[]]\n\n## Questions\n-\n"},
}

func (a *GranitApp) GetTemplates() []TemplateInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	templates := make([]TemplateInfo, len(builtinTemplates))
	copy(templates, builtinTemplates)
	if a.vaultRoot == "" {
		return templates
	}
	templDir := filepath.Join(a.vaultRoot, "templates")
	entries, err := os.ReadDir(templDir)
	if err != nil {
		return templates
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			content, err := os.ReadFile(filepath.Join(templDir, e.Name()))
			if err == nil {
				templates = append(templates, TemplateInfo{
					Name:    strings.TrimSuffix(e.Name(), ".md"),
					Content: string(content),
					IsUser:  true,
				})
			}
		}
	}
	return templates
}

func (a *GranitApp) CreateFromTemplate(idx int, name string) (string, error) {
	templates := a.GetTemplates()
	if idx < 0 || idx >= len(templates) {
		return "", fmt.Errorf("invalid template index")
	}
	content := templates[idx].Content
	now := time.Now()
	r := strings.NewReplacer(
		"{{date}}", now.Format("2006-01-02"),
		"{{time}}", now.Format("15:04"),
		"{{datetime}}", now.Format("2006-01-02 15:04"),
		"{{yesterday}}", now.AddDate(0, 0, -1).Format("2006-01-02"),
		"{{tomorrow}}", now.AddDate(0, 0, 1).Format("2006-01-02"),
		"{{weekday}}", now.Weekday().String(),
		"{{title}}", name,
	)
	content = r.Replace(content)
	return a.CreateNote(name, content)
}

// ==================== Vault Stats ====================

type StatEntry struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type VaultStatsData struct {
	TotalNotes     int         `json:"totalNotes"`
	TotalWords     int         `json:"totalWords"`
	TotalLinks     int         `json:"totalLinks"`
	TotalBacklinks int         `json:"totalBacklinks"`
	UniqueTagCount int         `json:"uniqueTagCount"`
	OrphanNotes    int         `json:"orphanNotes"`
	AvgLinks       float64     `json:"avgLinks"`
	TopLinked      []StatEntry `json:"topLinked"`
	LargestNotes   []StatEntry `json:"largestNotes"`
	TopTags        []StatEntry `json:"topTags"`
}

func (a *GranitApp) GetVaultStats() *VaultStatsData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.getVaultStatsInternal()
}

// getVaultStatsInternal is the lock-free version of GetVaultStats.
// Callers must hold at least a.mu.RLock().
func (a *GranitApp) getVaultStatsInternal() *VaultStatsData {
	if a.vault == nil {
		return &VaultStatsData{}
	}
	stats := &VaultStatsData{}
	type scored struct {
		path  string
		score int
	}
	var connScores, sizeScores []scored
	tagMap := make(map[string]int)

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		words := len(strings.Fields(note.Content))
		backlinks := a.index.GetBacklinks(p)

		stats.TotalNotes++
		stats.TotalWords += words
		stats.TotalLinks += len(note.Links)
		stats.TotalBacklinks += len(backlinks)

		conn := len(note.Links) + len(backlinks)
		if conn == 0 {
			stats.OrphanNotes++
		}
		connScores = append(connScores, scored{p, conn})
		sizeScores = append(sizeScores, scored{p, words})

		if tags, ok := note.Frontmatter["tags"]; ok {
			extractTagsFromValue(tags, tagMap)
		}
	}

	stats.UniqueTagCount = len(tagMap)
	if stats.TotalNotes > 0 {
		stats.AvgLinks = float64(stats.TotalLinks) / float64(stats.TotalNotes)
	}

	sort.Slice(connScores, func(i, j int) bool { return connScores[i].score > connScores[j].score })
	for i := 0; i < 5 && i < len(connScores); i++ {
		if connScores[i].score > 0 {
			stats.TopLinked = append(stats.TopLinked, StatEntry{
				Name:  strings.TrimSuffix(filepath.Base(connScores[i].path), ".md"),
				Value: connScores[i].score,
			})
		}
	}

	sort.Slice(sizeScores, func(i, j int) bool { return sizeScores[i].score > sizeScores[j].score })
	for i := 0; i < 5 && i < len(sizeScores); i++ {
		if sizeScores[i].score > 0 {
			stats.LargestNotes = append(stats.LargestNotes, StatEntry{
				Name:  strings.TrimSuffix(filepath.Base(sizeScores[i].path), ".md"),
				Value: sizeScores[i].score,
			})
		}
	}

	type tagScore struct {
		name  string
		count int
	}
	var tagScores []tagScore
	for name, count := range tagMap {
		tagScores = append(tagScores, tagScore{name, count})
	}
	sort.Slice(tagScores, func(i, j int) bool {
		if tagScores[i].count != tagScores[j].count {
			return tagScores[i].count > tagScores[j].count
		}
		return tagScores[i].name < tagScores[j].name
	})
	for i := 0; i < 5 && i < len(tagScores); i++ {
		stats.TopTags = append(stats.TopTags, StatEntry{Name: tagScores[i].name, Value: tagScores[i].count})
	}

	return stats
}

func extractTagsFromValue(tags interface{}, tagMap map[string]int) {
	switch v := tags.(type) {
	case []interface{}:
		for _, t := range v {
			if s, ok := t.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					tagMap[s]++
				}
			}
		}
	case []string:
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				tagMap[s]++
			}
		}
	case string:
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				tagMap[s]++
			}
		}
	}
}

// ==================== Tags ====================

type TagEntryDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (a *GranitApp) GetAllTags() []TagEntryDTO {
	if a.vault == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	tagMap := make(map[string]int)
	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		if tags, ok := note.Frontmatter["tags"]; ok {
			extractTagsFromValue(tags, tagMap)
		}
		for _, word := range strings.Fields(note.Content) {
			if strings.HasPrefix(word, "#") && len(word) > 1 {
				tag := strings.TrimRight(word[1:], ".,;:!?)")
				if tag != "" && !strings.HasPrefix(tag, "#") {
					tagMap[tag]++
				}
			}
		}
	}
	var result []TagEntryDTO
	for name, count := range tagMap {
		result = append(result, TagEntryDTO{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (a *GranitApp) GetNotesForTag(tag string) []NoteInfo {
	if a.vault == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	var result []NoteInfo
	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		found := false
		if tags, ok := note.Frontmatter["tags"]; ok {
			switch v := tags.(type) {
			case []interface{}:
				for _, t := range v {
					if s, ok := t.(string); ok && strings.TrimSpace(s) == tag {
						found = true
					}
				}
			case []string:
				for _, s := range v {
					if strings.TrimSpace(s) == tag {
						found = true
					}
				}
			case string:
				for _, s := range strings.Split(v, ",") {
					if strings.TrimSpace(s) == tag {
						found = true
					}
				}
			}
		}
		if !found && strings.Contains(note.Content, "#"+tag) {
			found = true
		}
		if found {
			result = append(result, NoteInfo{
				RelPath: note.RelPath, Title: note.Title,
				ModTime: note.ModTime.Format(time.RFC3339), Size: note.Size,
			})
		}
	}
	return result
}

// ==================== Graph ====================

type GraphNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Incoming int    `json:"incoming"`
	Outgoing int    `json:"outgoing"`
	Total    int    `json:"total"`
	IsCenter bool   `json:"isCenter"`
	HopDist  int    `json:"hopDist"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Stats struct {
		TotalNodes  int `json:"totalNodes"`
		TotalEdges  int `json:"totalEdges"`
		OrphanCount int `json:"orphanCount"`
	} `json:"stats"`
}

func (a *GranitApp) GetGraphData(centerNote string) *GraphData {
	if a.vault == nil {
		return &GraphData{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	data := &GraphData{}
	edgeSet := make(map[string]bool)

	if centerNote != "" {
		hopMap := map[string]int{centerNote: 0}
		frontier := []string{centerNote}
		for hop := 1; hop <= 2; hop++ {
			var next []string
			for _, p := range frontier {
				for _, nb := range a.getNeighbors(p) {
					if _, seen := hopMap[nb]; !seen {
						hopMap[nb] = hop
						next = append(next, nb)
					}
				}
			}
			frontier = next
		}
		for p, dist := range hopMap {
			note := a.vault.GetNote(p)
			if note == nil {
				continue
			}
			bl := a.index.GetBacklinks(p)
			data.Nodes = append(data.Nodes, GraphNode{
				ID: p, Name: note.Title, Incoming: len(bl), Outgoing: len(note.Links),
				Total: len(bl) + len(note.Links), IsCenter: p == centerNote, HopDist: dist,
			})
			for _, link := range note.Links {
				resolved := a.index.ResolveLink(link)
				if resolved != "" {
					if _, inScope := hopMap[resolved]; inScope {
						key := p + "|" + resolved
						if !edgeSet[key] {
							edgeSet[key] = true
							data.Edges = append(data.Edges, GraphEdge{Source: p, Target: resolved})
						}
					}
				}
			}
		}
	} else {
		for _, p := range a.vault.SortedPaths() {
			note := a.vault.GetNote(p)
			if note == nil {
				continue
			}
			bl := a.index.GetBacklinks(p)
			total := len(bl) + len(note.Links)
			data.Nodes = append(data.Nodes, GraphNode{
				ID: p, Name: note.Title, Incoming: len(bl), Outgoing: len(note.Links),
				Total: total, HopDist: -1,
			})
			if total == 0 {
				data.Stats.OrphanCount++
			}
			for _, link := range note.Links {
				resolved := a.index.ResolveLink(link)
				if resolved != "" {
					key := p + "|" + resolved
					if !edgeSet[key] {
						edgeSet[key] = true
						data.Edges = append(data.Edges, GraphEdge{Source: p, Target: resolved})
					}
				}
			}
		}
	}
	data.Stats.TotalNodes = len(data.Nodes)
	data.Stats.TotalEdges = len(data.Edges)
	sort.Slice(data.Nodes, func(i, j int) bool { return data.Nodes[i].Total > data.Nodes[j].Total })
	return data
}

func (a *GranitApp) getNeighbors(path string) []string {
	seen := make(map[string]bool)
	var result []string
	note := a.vault.GetNote(path)
	if note != nil {
		for _, link := range note.Links {
			resolved := a.index.ResolveLink(link)
			if resolved != "" && !seen[resolved] {
				seen[resolved] = true
				result = append(result, resolved)
			}
		}
	}
	for _, bl := range a.index.GetBacklinks(path) {
		if !seen[bl] {
			seen[bl] = true
			result = append(result, bl)
		}
	}
	return result
}

// ==================== Bookmarks ====================

type BookmarkFile struct {
	Starred []string `json:"starred"`
	Recent  []string `json:"recent"`
}

func (a *GranitApp) bookmarksPath() string {
	return filepath.Join(a.vaultRoot, ".granit-bookmarks.json")
}

func (a *GranitApp) loadBookmarks() *BookmarkFile {
	data, err := os.ReadFile(a.bookmarksPath())
	if err != nil {
		return &BookmarkFile{}
	}
	var bm BookmarkFile
	if err := json.Unmarshal(data, &bm); err != nil {
		log.Printf("warning: failed to parse bookmarks: %v", err)
		return &BookmarkFile{}
	}
	return &bm
}

func (a *GranitApp) saveBookmarks(bm *BookmarkFile) error {
	data, err := json.MarshalIndent(bm, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(a.bookmarksPath(), data, 0644)
}

func (a *GranitApp) GetBookmarks() *BookmarkFile {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return &BookmarkFile{}
	}
	return a.loadBookmarks()
}

func (a *GranitApp) ToggleBookmark(relPath string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	bm := a.loadBookmarks()
	for i, s := range bm.Starred {
		if s == relPath {
			bm.Starred = append(bm.Starred[:i], bm.Starred[i+1:]...)
			return false, a.saveBookmarks(bm)
		}
	}
	bm.Starred = append([]string{relPath}, bm.Starred...)
	return true, a.saveBookmarks(bm)
}

func (a *GranitApp) AddRecent(relPath string) error {
	if a.vaultRoot == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	bm := a.loadBookmarks()
	filtered := make([]string, 0, len(bm.Recent))
	for _, r := range bm.Recent {
		if r != relPath {
			filtered = append(filtered, r)
		}
	}
	bm.Recent = append([]string{relPath}, filtered...)
	if len(bm.Recent) > 20 {
		bm.Recent = bm.Recent[:20]
	}
	return a.saveBookmarks(bm)
}

// ==================== Trash ====================

type TrashItemInfo struct {
	OrigPath  string `json:"origPath"`
	TrashFile string `json:"trashFile"`
	DeletedAt string `json:"deletedAt"`
	TimeAgo   string `json:"timeAgo"`
}

type trashMeta struct {
	OrigPath  string    `json:"orig_path"`
	TrashPath string    `json:"trash_path"`
	DeletedAt time.Time `json:"deleted_at"`
}

func (a *GranitApp) GetTrashItems() []TrashItemInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return nil
	}
	trashDir := filepath.Join(a.vaultRoot, ".granit-trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		return nil
	}
	var items []TrashItemInfo
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(trashDir, e.Name()))
		if err != nil {
			continue
		}
		var meta trashMeta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		contentFile := strings.TrimSuffix(e.Name(), ".json")
		if _, err := os.Stat(filepath.Join(trashDir, contentFile)); err != nil {
			continue
		}
		items = append(items, TrashItemInfo{
			OrigPath:  meta.OrigPath,
			TrashFile: contentFile,
			DeletedAt: meta.DeletedAt.Format(time.RFC3339),
			TimeAgo:   timeAgo(meta.DeletedAt),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DeletedAt > items[j].DeletedAt })
	return items
}

func (a *GranitApp) RestoreFromTrash(trashFile string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	trashDir := filepath.Join(a.vaultRoot, ".granit-trash")
	// Validate trash file stays within trash dir
	srcAbs, err := filepath.Abs(filepath.Join(trashDir, trashFile))
	if err != nil || !strings.HasPrefix(srcAbs, trashDir) {
		return fmt.Errorf("invalid trash file path")
	}
	metaPath := srcAbs + ".json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	var meta trashMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	dst, err := a.validatePath(meta.OrigPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	content, err := os.ReadFile(srcAbs)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(dst, content, 0644); err != nil {
		return err
	}
	if err := os.Remove(srcAbs); err != nil {
		log.Printf("warning: failed to remove trash file %s: %v", srcAbs, err)
	}
	if err := os.Remove(metaPath); err != nil {
		log.Printf("warning: failed to remove trash metadata %s: %v", metaPath, err)
	}
	a.vault.Scan()
	a.index.Build()
	return nil
}

func (a *GranitApp) PurgeFromTrash(trashFile string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	trashDir := filepath.Join(a.vaultRoot, ".granit-trash")
	if err := os.Remove(filepath.Join(trashDir, trashFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("purge trash file: %w", err)
	}
	if err := os.Remove(filepath.Join(trashDir, trashFile+".json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("purge trash metadata: %w", err)
	}
	return nil
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ==================== Outline ====================

type OutlineItem struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

func (a *GranitApp) GetOutline(relPath string) []OutlineItem {
	if a.vault == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	note := a.vault.GetNote(relPath)
	if note == nil {
		return nil
	}
	var items []OutlineItem
	inCodeBlock := false
	for i, line := range strings.Split(note.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock || !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for _, c := range trimmed {
			if c == '#' {
				level++
			} else {
				break
			}
		}
		if level >= 1 && level <= 6 && len(trimmed) > level && trimmed[level] == ' ' {
			text := strings.TrimSpace(trimmed[level+1:])
			items = append(items, OutlineItem{Level: level, Text: text, Line: i})
		}
	}
	return items
}

// ==================== Commands ====================

type CommandInfo struct {
	Action   string `json:"action"`
	Label    string `json:"label"`
	Desc     string `json:"desc"`
	Shortcut string `json:"shortcut"`
	Icon     string `json:"icon"`
}

func (a *GranitApp) GetCommands() []CommandInfo {
	return allCommands
}

var allCommands = []CommandInfo{
	{"open_file", "Open File", "Quick open a file", "Ctrl+P", "search"},
	{"new_note", "New Note", "Create a new note", "Ctrl+N", "new"},
	{"save_note", "Save Note", "Save the current note", "Ctrl+S", "save"},
	{"daily_note", "Daily Note", "Open or create today's daily note", "Alt+D", "calendar"},
	{"prev_daily", "Previous Daily Note", "Navigate to the previous daily note", "Alt+[", "calendar"},
	{"next_daily", "Next Daily Note", "Navigate to the next daily note", "Alt+]", "calendar"},
	{"weekly_note", "Weekly Note", "Open or create this week's note", "Alt+W", "calendar"},
	{"toggle_view", "Toggle View/Edit", "Switch between view and edit mode", "Ctrl+E", "view"},
	{"settings", "Settings", "Open settings panel", "Ctrl+,", "settings"},
	{"focus_editor", "Focus Editor", "Switch focus to the editor", "Alt+2", "edit"},
	{"focus_sidebar", "Focus Sidebar", "Switch focus to the file sidebar", "Alt+1", "folder"},
	{"focus_backlinks", "Focus Backlinks", "Switch focus to the backlinks panel", "Alt+3", "link"},
	{"toggle_sidebar", "Toggle Sidebar", "Show or hide the file sidebar", "", "folder"},
	{"refresh_vault", "Refresh Vault", "Rescan vault for changes", "", "search"},
	{"delete_note", "Delete Note", "Delete the current note", "", "trash"},
	{"rename_note", "Rename Note", "Rename the current note", "F4", "edit"},
	{"show_graph", "Show Graph", "Show note connection graph", "Ctrl+G", "graph"},
	{"show_tags", "Show Tags", "Browse notes by tags", "Ctrl+T", "tag"},
	{"show_help", "Help", "Show keyboard shortcuts", "F5", "help"},
	{"show_outline", "Outline", "Show note heading outline", "Ctrl+O", "outline"},
	{"show_bookmarks", "Bookmarks", "View starred & recent notes", "Ctrl+B", "bookmark"},
	{"toggle_bookmark", "Toggle Bookmark", "Star/unstar current note", "", "bookmark"},
	{"find_in_file", "Find", "Search within current file", "Ctrl+F", "search"},
	{"replace_in_file", "Find & Replace", "Find and replace in file", "Ctrl+H", "search"},
	{"show_stats", "Vault Statistics", "Show vault stats & charts", "", "graph"},
	{"new_from_template", "New from Template", "Create note from template", "", "file"},
	{"focus_mode", "Focus Mode", "Distraction-free writing", "Ctrl+Z", "edit"},
	{"quick_switch", "Quick Switch", "Switch between recent files", "Ctrl+J", "file"},
	{"show_trash", "Trash", "View and restore deleted notes", "", "trash"},
	{"show_canvas", "Canvas", "Visual note canvas / whiteboard", "Ctrl+W", "canvas"},
	{"show_calendar", "Calendar", "Calendar view with daily notes", "Ctrl+L", "calendar"},
	{"show_bots", "Bots", "AI bots for note analysis", "Ctrl+R", "bot"},
	{"new_folder", "New Folder", "Create a new folder", "", "folder"},
	{"move_file", "Move File", "Move current note to a folder", "", "folder"},
	{"export_note", "Export Current Note", "Export note as HTML, text, or PDF", "", "save"},
	{"git_overlay", "Git: Status & Commit", "Git status, log, diff, commit, push, pull", "", "bot"},
	{"plugin_manager", "Plugins", "Manage and run plugins", "", "settings"},
	{"content_search", "Search Vault Contents", "Full-text search across all notes", "", "search"},
	{"global_replace", "Global Search & Replace", "Find and replace across all vault files", "", "search"},
	{"spell_check", "Spell Check", "Check spelling in current note", "", "edit"},
	{"import_obsidian", "Import Obsidian Config", "Import settings from .obsidian/ directory", "", "settings"},
	{"publish_site", "Publish Site", "Export vault as static HTML site", "", "save"},
	{"split_pane", "Split View", "View two notes side by side", "", "view"},
	{"flashcards", "Flashcards", "Spaced repetition study from your notes", "", "bookmark"},
	{"quiz_mode", "Quiz Mode", "Test your knowledge with auto-generated quizzes", "", "help"},
	{"ai_chat", "AI Chat", "Ask questions about your vault", "", "bot"},
	{"ai_compose", "AI Compose Note", "Generate a note from a topic prompt", "", "new"},
	{"ai_template", "AI Template", "Generate a full note from template type + topic", "", "bot"},
	{"knowledge_graph", "Knowledge Graph AI", "Analyze clusters, hubs, orphans", "", "graph"},
	{"auto_link", "Auto-Link Suggestions", "Find unlinked mentions in current note", "", "link"},
	{"similar_notes", "Similar Notes", "Find notes similar to current one", "", "search"},
	{"table_editor", "Table Editor", "Visual markdown table editor", "", "edit"},
	{"semantic_search", "Semantic Search", "AI-powered meaning-based vault search", "", "search"},
	{"thread_weaver", "Thread Weaver", "Synthesize multiple notes into a new essay", "", "new"},
	{"note_chat", "Chat with Note", "AI Q&A focused on current note", "", "bot"},
	{"toggle_ghost_writer", "Ghost Writer", "Toggle inline AI writing suggestions", "", "edit"},
	{"pomodoro", "Pomodoro Timer", "Focus timer with writing stats", "", "calendar"},
	{"clock_in", "Clock In", "Start a work session timer", "", "calendar"},
	{"clock_out", "Clock Out", "Stop work session and log time", "", "calendar"},
	{"web_clip", "Web Clipper", "Save a web page as a markdown note", "", "save"},
	{"toggle_vim", "Toggle Vim Mode", "Enable/disable Vim keybindings", "", "edit"},
	{"toggle_word_wrap", "Toggle Word Wrap", "Wrap long lines at viewport width", "", "edit"},
	{"pin_note", "Pin Note", "Pin current note as a tab", "", "bookmark"},
	{"nav_back", "Navigate Back", "Go to previous note in history", "Alt+Left", "folder"},
	{"nav_forward", "Navigate Forward", "Go to next note in history", "Alt+Right", "folder"},
	{"kanban", "Kanban Board", "View tasks as a Kanban board", "", "canvas"},
	{"zettel_note", "New Zettelkasten Note", "Create a note with unique Zettelkasten ID", "", "new"},
	{"daily_briefing", "Daily Briefing", "Granit morning briefing with today's focus", "", "calendar"},
	{"encrypt_note", "Encrypt/Decrypt Note", "AES-256-GCM encryption for secure sync", "", "save"},
	{"git_history", "Git History", "View commit history for current note", "", "edit"},
	{"workspaces", "Workspaces", "Save and restore named workspace layouts", "", "view"},
	{"timeline", "Timeline", "Chronological view of all notes", "", "calendar"},
	{"vault_switch", "Switch Vault", "Switch to a different vault", "", "folder"},
	{"fold_toggle", "Toggle Fold", "Fold/unfold section under cursor", "", "outline"},
	{"frontmatter_edit", "Edit Frontmatter", "Structured frontmatter property editor", "", "edit"},
	{"research_agent", "Deep Dive Research", "AI research agent — notes from any topic", "", "bot"},
	{"vault_analyzer", "Vault Analyzer", "AI analysis of vault structure and gaps", "", "graph"},
	{"note_enhancer", "Note Enhancer", "AI-enhance current note with links and depth", "", "edit"},
	{"daily_digest", "Daily Digest", "Generate weekly review from recent activity", "", "calendar"},
	{"habit_tracker", "Habit Tracker", "Daily habits, goals, streaks, and progress", "", "graph"},
	{"focus_session", "Focus Session", "Guided work session with timer and tasks", "", "calendar"},
	{"standup_gen", "Daily Standup", "Auto-generate standup from git and tasks", "", "calendar"},
	{"note_history", "Note History", "Git version timeline for current note", "", "outline"},
	{"smart_connections", "Smart Connections", "Find semantically related notes", "", "link"},
	{"writing_stats", "Writing Statistics", "Word counts, streaks, productivity charts", "", "graph"},
	{"quick_capture", "Quick Capture", "Jot down a quick thought to inbox", "", "new"},
	{"dashboard", "Dashboard", "Vault home screen with tasks and stats", "", "calendar"},
	{"mind_map", "Mind Map", "Visual mind map from headings and wikilinks", "", "graph"},
	{"journal_prompts", "Journal Prompts", "Daily reflection prompts", "", "edit"},
	{"daily_planner", "Daily Planner", "Time-blocked daily schedule", "", "calendar"},
	{"ai_scheduler", "AI Smart Scheduler", "AI-powered optimal schedule generation", "", "bot"},
	{"plan_my_day", "Plan My Day", "One-click AI daily plan", "Alt+P", "bot"},
	{"recurring_tasks", "Recurring Tasks", "Manage daily/weekly/monthly recurring tasks", "", "calendar"},
	{"scratchpad", "Scratchpad", "Floating persistent scratchpad", "", "edit"},
	{"projects", "Projects", "Project management with dashboards", "", "folder"},
	{"nl_search", "Natural Language Search", "AI-powered meaning-based vault search", "", "search"},
	{"writing_coach", "Writing Coach", "AI writing analysis with persona support", "", "bot"},
	{"dataview", "Dataview Query", "Query notes by frontmatter properties", "", "graph"},
	{"time_tracker", "Time Tracker", "Track time per note/task with stats", "", "calendar"},
	{"task_manager", "Task Manager", "View and manage all tasks across vault", "Ctrl+K", "calendar"},
	{"link_assist", "Link Assistant", "Find unlinked mentions and suggest wikilinks", "", "link"},
	{"image_manager", "Image Manager", "Browse and manage vault images", "", "view"},
	{"theme_editor", "Theme Editor", "Create and customize color themes", "", "settings"},
	{"layout_default", "Default Layout", "3-panel: sidebar, editor, backlinks", "", "view"},
	{"layout_writer", "Writer Layout", "2-panel: sidebar, editor", "", "view"},
	{"layout_minimal", "Minimal Layout", "Editor only", "", "view"},
	{"layout_reading", "Reading Layout", "Editor + backlinks, no sidebar", "", "view"},
	{"layout_dashboard", "Dashboard Layout", "4-panel: sidebar, editor, outline, backlinks", "", "view"},
	{"vault_backup", "Vault Backup", "Create, restore, and manage vault backups", "", "save"},
	{"show_tutorial", "Show Tutorial", "Interactive walkthrough of Granit features", "", "help"},
	{"knowledge_gaps", "AI Knowledge Gaps", "Find missing topics, stale notes, orphans", "", "graph"},
	{"extract_to_note", "Extract to Note", "Move selection to a new note, leave wikilink", "", "link"},
	{"command_center", "Command Center", "What do I do RIGHT NOW? dashboard", "Alt+C", "calendar"},
	{"quit", "Quit", "Exit Granit", "Ctrl+Q", ""},
}

// ==================== Settings ====================

type SettingItem struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Type        string      `json:"type"`
	Value       interface{} `json:"value"`
	Options     []string    `json:"options,omitempty"`
	Category    string      `json:"category"`
	Description string      `json:"description,omitempty"`
}

func (a *GranitApp) GetAllSettings() []SettingItem {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c := a.config
	return []SettingItem{
		{"theme", "Theme", "select", c.Theme, []string{"catppuccin-mocha", "catppuccin-latte", "catppuccin-frappe", "catppuccin-macchiato", "tokyo-night", "gruvbox-dark", "nord", "dracula", "solarized-dark", "solarized-light", "rose-pine", "rose-pine-dawn", "everforest-dark", "kanagawa", "one-dark", "github-dark", "github-light", "ayu-dark", "ayu-light", "palenight", "synthwave-84", "nightfox", "vesper", "poimandres", "moonlight", "vitesse-dark", "min-light", "oxocarbon", "matrix", "cobalt2", "monokai-pro", "horizon", "zenburn", "iceberg", "amber", "high-contrast-dark", "high-contrast-light", "deuteranopia", "protanopia", "tritanopia"}, "Appearance", "Color theme for the entire app"},
		{"icon_theme", "Icon Theme", "select", c.IconTheme, []string{"unicode", "nerd", "emoji", "ascii"}, "Appearance", "Icon set used throughout the interface"},
		{"layout", "Layout", "select", c.Layout, []string{"default", "writer", "minimal", "reading", "dashboard"}, "Appearance", "Panel arrangement: default (3-panel), writer (2-panel), minimal (editor only)"},
		{"sidebar_position", "Sidebar Position", "select", c.SidebarPosition, []string{"left", "right"}, "Appearance", "Which side to show the file explorer"},
		{"show_icons", "Show Icons", "bool", c.ShowIcons, nil, "Appearance", "Display file and folder icons in sidebar"},
		{"compact_mode", "Compact Mode", "bool", c.CompactMode, nil, "Appearance", "Reduce padding and spacing for more content"},
		{"show_splash", "Show Splash Screen", "bool", c.ShowSplash, nil, "Appearance", "Show animated logo on startup"},
		{"show_help", "Show Help Bar", "bool", c.ShowHelp, nil, "Appearance", "Display keyboard shortcut hints"},
		{"vim_mode", "Vim Mode", "bool", c.VimMode, nil, "Editor", "Enable Vim keybindings (hjkl, modes, etc.)"},
		{"word_wrap", "Word Wrap", "bool", c.WordWrap, nil, "Editor", "Wrap long lines instead of horizontal scroll"},
		{"tab_size", "Tab Size", "int", c.Editor.TabSize, nil, "Editor", "Number of spaces per tab (2 or 4)"},
		{"line_numbers", "Line Numbers", "bool", c.LineNumbers, nil, "Editor", "Show line numbers in the editor gutter"},
		{"auto_close_brackets", "Auto Close Brackets", "bool", c.AutoCloseBrackets, nil, "Editor", "Automatically insert closing brackets and quotes"},
		{"highlight_current_line", "Highlight Current Line", "bool", c.HighlightCurrentLine, nil, "Editor", "Subtle background highlight on the cursor line"},
		{"default_view_mode", "Default View Mode", "bool", c.DefaultViewMode, nil, "Editor", "Open notes in preview mode by default"},
		{"spell_check", "Inline Spell Check", "bool", c.SpellCheck, nil, "Editor", "Underline misspelled words while typing"},
		{"ai_provider", "AI Provider", "select", c.AIProvider, []string{"local", "ollama", "openai"}, "AI", "AI backend for bots, chat, and writing tools"},
		{"ollama_model", "Ollama Model", "select", c.OllamaModel, []string{"qwen2.5:0.5b", "qwen2.5:1.5b", "qwen2.5:3b", "phi3:mini", "phi3.5:3.8b", "gemma2:2b", "tinyllama", "llama3.2", "llama3.2:1b", "mistral", "gemma2"}, "AI", "Local Ollama model to use for AI features"},
		{"ollama_url", "Ollama URL", "string", c.OllamaURL, nil, "AI", "Ollama server address (default: http://localhost:11434)"},
		{"openai_key", "OpenAI API Key", "string", c.OpenAIKey, nil, "AI", "Your OpenAI API key for cloud AI features"},
		{"openai_model", "OpenAI Model", "select", c.OpenAIModel, []string{"gpt-4o-mini", "gpt-4o", "gpt-4.1-mini", "gpt-4.1-nano"}, "AI", "OpenAI model to use (mini = faster, cheaper)"},
		{"auto_save", "Auto Save", "bool", c.AutoSave, nil, "Files", "Automatically save notes after 2 seconds of inactivity"},
		{"auto_daily_note", "Auto Daily Note", "bool", c.AutoDailyNote, nil, "Files", "Create today's daily note on vault open"},
		{"daily_notes_folder", "Daily Notes Folder", "string", c.DailyNotesFolder, nil, "Files", "Folder for daily notes (empty = vault root)"},
		{"sort_by", "Sort Files By", "select", c.SortBy, []string{"name", "modified", "created"}, "Files", "Default file sorting order in sidebar"},
		{"confirm_delete", "Confirm Delete", "bool", c.ConfirmDelete, nil, "Files", "Ask for confirmation before deleting notes"},
		{"auto_refresh", "Auto Refresh Vault", "bool", c.AutoRefresh, nil, "Files", "Watch for external file changes and refresh"},
		{"git_auto_sync", "Git Auto Sync", "bool", c.GitAutoSync, nil, "Advanced", "Auto-commit and push changes on save"},
		{"auto_tag", "Auto-Tag on Save", "bool", c.AutoTag, nil, "Advanced", "Use AI to suggest tags when saving notes"},
	}
}

func (a *GranitApp) UpdateSetting(key string, value interface{}) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := &a.config
	setBool := func(target *bool) error {
		v, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool for %s", key)
		}
		*target = v
		return nil
	}
	setStr := func(target *string) error {
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string for %s", key)
		}
		*target = v
		return nil
	}
	var err error
	switch key {
	case "theme":
		err = setStr(&c.Theme)
	case "icon_theme":
		err = setStr(&c.IconTheme)
	case "layout":
		err = setStr(&c.Layout)
	case "sidebar_position":
		err = setStr(&c.SidebarPosition)
	case "show_icons":
		err = setBool(&c.ShowIcons)
	case "compact_mode":
		err = setBool(&c.CompactMode)
	case "show_splash":
		err = setBool(&c.ShowSplash)
	case "show_help":
		err = setBool(&c.ShowHelp)
	case "vim_mode":
		err = setBool(&c.VimMode)
	case "word_wrap":
		err = setBool(&c.WordWrap)
	case "tab_size":
		if v, ok := value.(float64); ok {
			c.Editor.TabSize = int(v)
		} else {
			err = fmt.Errorf("expected number for tab_size")
		}
	case "line_numbers":
		err = setBool(&c.LineNumbers)
	case "auto_close_brackets":
		err = setBool(&c.AutoCloseBrackets)
	case "highlight_current_line":
		err = setBool(&c.HighlightCurrentLine)
	case "default_view_mode":
		err = setBool(&c.DefaultViewMode)
	case "spell_check":
		err = setBool(&c.SpellCheck)
	case "ai_provider":
		err = setStr(&c.AIProvider)
	case "ollama_model":
		err = setStr(&c.OllamaModel)
	case "ollama_url":
		err = setStr(&c.OllamaURL)
	case "openai_key":
		err = setStr(&c.OpenAIKey)
	case "openai_model":
		err = setStr(&c.OpenAIModel)
	case "auto_save":
		err = setBool(&c.AutoSave)
	case "auto_daily_note":
		err = setBool(&c.AutoDailyNote)
	case "daily_notes_folder":
		err = setStr(&c.DailyNotesFolder)
	case "sort_by":
		err = setStr(&c.SortBy)
	case "confirm_delete":
		err = setBool(&c.ConfirmDelete)
	case "auto_refresh":
		err = setBool(&c.AutoRefresh)
	case "git_auto_sync":
		err = setBool(&c.GitAutoSync)
	case "auto_tag":
		err = setBool(&c.AutoTag)
	default:
		return fmt.Errorf("unknown setting: %s", key)
	}
	if err != nil {
		return err
	}
	return c.Save()
}

// ==================== Misc ====================

func (a *GranitApp) RefreshVault() error {
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.vault.Scan(); err != nil {
		return err
	}
	a.index.Build()
	return nil
}

func (a *GranitApp) CreateFolder(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	absPath, err := a.validatePath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(absPath, 0755)
}

func (a *GranitApp) MoveFile(relPath string, newDir string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	oldAbs, err := a.validatePath(relPath)
	if err != nil {
		return "", err
	}
	newRelPath := filepath.Join(newDir, filepath.Base(relPath))
	newAbs, err := a.validatePath(newRelPath)
	if err != nil {
		return "", err
	}
	os.MkdirAll(filepath.Dir(newAbs), 0755)
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return "", err
	}
	a.vault.Scan()
	a.index.Build()
	return newRelPath, nil
}

type VaultListEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	LastOpen string `json:"lastOpen"`
}

func (a *GranitApp) GetKnownVaults() []VaultListEntry {
	vl := config.LoadVaultList()
	var entries []VaultListEntry
	for _, v := range vl.Vaults {
		entries = append(entries, VaultListEntry{
			Name: v.Name, Path: v.Path, LastOpen: v.LastOpen,
		})
	}
	return entries
}

// ==================== Backlink Context ====================

type BacklinkContextEntry struct {
	RelPath string `json:"relPath"`
	Title   string `json:"title"`
	Context string `json:"context"`
}

func (a *GranitApp) GetBacklinkContext(relPath string) []BacklinkContextEntry {
	if a.vault == nil || a.index == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	backlinks := a.index.GetBacklinks(relPath)
	entries := make([]BacklinkContextEntry, 0, len(backlinks))
	for _, bl := range backlinks {
		note := a.vault.GetNote(bl)
		if note == nil {
			continue
		}
		title := strings.TrimSuffix(filepath.Base(relPath), ".md")
		context := ""
		for _, line := range strings.Split(note.Content, "\n") {
			if strings.Contains(line, "[["+title+"]]") || strings.Contains(line, "[["+relPath+"]]") {
				context = strings.TrimSpace(line)
				if len(context) > 120 {
					context = context[:120] + "..."
				}
				break
			}
		}
		entries = append(entries, BacklinkContextEntry{
			RelPath: bl,
			Title:   note.Title,
			Context: context,
		})
	}
	return entries
}

// ==================== Platform ====================

func (a *GranitApp) GetPlatform() string {
	return runtime.GOOS
}
