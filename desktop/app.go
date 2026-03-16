package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------- DTO types (serialized to/from frontend as JSON) ----------

type NoteInfo struct {
	RelPath string `json:"relPath"`
	Title   string `json:"title"`
	ModTime string `json:"modTime"`
	Size    int64  `json:"size"`
}

type NoteDetail struct {
	RelPath     string                 `json:"relPath"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Links       []string               `json:"links"`
	Backlinks   []string               `json:"backlinks"`
	ModTime     string                 `json:"modTime"`
	WordCount   int                    `json:"wordCount"`
}

type FolderNode struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	IsFolder bool          `json:"isFolder"`
	Children []*FolderNode `json:"children,omitempty"`
}

type SearchHit struct {
	RelPath   string  `json:"relPath"`
	Title     string  `json:"title"`
	Line      int     `json:"line"`
	Column    int     `json:"column"`
	MatchLine string  `json:"matchLine"`
	Score     float64 `json:"score"`
}

// ---------- Application struct ----------

type GranitApp struct {
	ctx       context.Context
	vault     *vault.Vault
	index     *vault.Index
	config    config.Config
	vaultRoot string
	watcher   interface{ Close() error }
}

func NewGranitApp() *GranitApp {
	return &GranitApp{}
}

func (a *GranitApp) startup(ctx context.Context) {
	a.ctx = ctx
	a.config = config.Load()
}

func (a *GranitApp) shutdown(_ context.Context) {
	a.stopFileWatcher()
}

// validatePath ensures the resolved path stays inside the vault root.
func (a *GranitApp) validatePath(userPath string) (string, error) {
	joined := filepath.Join(a.vaultRoot, userPath)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(abs, a.vaultRoot) {
		return "", fmt.Errorf("path escapes vault root")
	}
	return abs, nil
}

// ---------- Vault management ----------

func (a *GranitApp) OpenVault(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	v, err := vault.NewVault(abs)
	if err != nil {
		return err
	}
	if err := v.Scan(); err != nil {
		return err
	}
	idx := vault.NewIndex(v)
	idx.Build()

	a.vault = v
	a.index = idx
	a.vaultRoot = abs
	a.config = config.LoadForVault(abs)

	// Start file watcher for auto-refresh
	a.startFileWatcher()

	// Register in vault list
	vl := config.LoadVaultList()
	vl.AddVault(abs)
	config.SaveVaultList(vl)

	return nil
}

func (a *GranitApp) SelectVaultDialog() (string, error) {
	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Open Vault",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}
	return dir, a.OpenVault(dir)
}

func (a *GranitApp) IsVaultOpen() bool {
	return a.vault != nil
}

func (a *GranitApp) GetVaultPath() string {
	return a.vaultRoot
}

// ---------- Note operations ----------

func (a *GranitApp) GetNotes() []NoteInfo {
	if a.vault == nil {
		return nil
	}
	paths := a.vault.SortedPaths()
	notes := make([]NoteInfo, 0, len(paths))
	for _, p := range paths {
		note := a.vault.Notes[p]
		notes = append(notes, NoteInfo{
			RelPath: note.RelPath,
			Title:   note.Title,
			ModTime: note.ModTime.Format(time.RFC3339),
			Size:    note.Size,
		})
	}
	return notes
}

func (a *GranitApp) GetNote(relPath string) (*NoteDetail, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", relPath)
	}
	backlinks := a.index.GetBacklinks(relPath)
	wordCount := len(strings.Fields(note.Content))

	return &NoteDetail{
		RelPath:     note.RelPath,
		Title:       note.Title,
		Content:     note.Content,
		Frontmatter: note.Frontmatter,
		Links:       note.Links,
		Backlinks:   backlinks,
		ModTime:     note.ModTime.Format(time.RFC3339),
		WordCount:   wordCount,
	}, nil
}

func (a *GranitApp) SaveNote(relPath string, content string) error {
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	absPath := filepath.Join(a.vaultRoot, relPath)

	abs, err := filepath.Abs(absPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return fmt.Errorf("invalid path")
	}

	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return err
	}

	// Update in-memory state
	note := a.vault.Notes[relPath]
	if note != nil {
		note.Content = content
		note.Frontmatter = vault.ParseFrontmatter(content)
		note.Links = vault.ParseWikiLinks(content)
		note.ModTime = time.Now()
	}

	if a.vault.SearchIndex != nil {
		a.vault.SearchIndex.Update(relPath, content)
	}
	a.index.Build()

	return nil
}

func (a *GranitApp) CreateNote(name string, content string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	absPath, err := a.validatePath(name)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", err
	}
	if _, err := os.Stat(absPath); err == nil {
		return "", fmt.Errorf("note already exists: %s", name)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return "", err
	}

	if err := a.vault.Scan(); err != nil {
		return "", err
	}
	a.index.Build()

	return name, nil
}

func (a *GranitApp) DeleteNote(relPath string) error {
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	absPath := filepath.Join(a.vaultRoot, relPath)

	abs, err := filepath.Abs(absPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return fmt.Errorf("invalid path")
	}

	trashDir := filepath.Join(a.vaultRoot, ".granit-trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return err
	}

	// Use timestamped filename + JSON sidecar (compatible with TUI trash)
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	base := filepath.Base(relPath)
	trashFile := ts + "_" + base
	trashPath := filepath.Join(trashDir, trashFile)

	// Copy content to trash
	content, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(trashPath, content, 0644); err != nil {
		return err
	}

	// Write metadata sidecar
	meta, _ := json.Marshal(map[string]interface{}{
		"orig_path":  relPath,
		"trash_path": trashFile,
		"deleted_at": time.Now(),
	})
	os.WriteFile(filepath.Join(trashDir, trashFile+".json"), meta, 0644)

	// Remove original
	os.Remove(abs)

	delete(a.vault.Notes, relPath)
	if a.vault.SearchIndex != nil {
		a.vault.SearchIndex.Remove(relPath)
	}
	a.index.Build()

	return nil
}

func (a *GranitApp) RenameNote(oldPath string, newName string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	if !strings.HasSuffix(newName, ".md") {
		newName += ".md"
	}
	oldAbs, err := a.validatePath(oldPath)
	if err != nil {
		return "", err
	}
	newRelPath := filepath.Join(filepath.Dir(oldPath), newName)
	newAbs, err := a.validatePath(newRelPath)
	if err != nil {
		return "", err
	}

	if err := os.Rename(oldAbs, newAbs); err != nil {
		return "", err
	}

	if err := a.vault.Scan(); err != nil {
		return "", err
	}
	a.index.Build()

	return newRelPath, nil
}

// ---------- Folder tree ----------

func (a *GranitApp) GetFolderTree() *FolderNode {
	if a.vault == nil {
		return &FolderNode{Name: "No vault", IsFolder: true}
	}

	root := &FolderNode{
		Name:     filepath.Base(a.vaultRoot),
		Path:     "",
		IsFolder: true,
		Children: []*FolderNode{},
	}

	nodeMap := map[string]*FolderNode{"": root}

	for _, relPath := range a.vault.SortedPaths() {
		dir := filepath.Dir(relPath)

		if dir != "." {
			parts := strings.Split(dir, string(filepath.Separator))
			current := ""
			for _, part := range parts {
				parent := current
				if current == "" {
					current = part
				} else {
					current = current + "/" + part
				}
				if _, exists := nodeMap[current]; !exists {
					node := &FolderNode{
						Name:     part,
						Path:     current,
						IsFolder: true,
						Children: []*FolderNode{},
					}
					nodeMap[parent].Children = append(nodeMap[parent].Children, node)
					nodeMap[current] = node
				}
			}
		}

		parentPath := ""
		if dir != "." {
			parentPath = dir
		}
		name := filepath.Base(relPath)
		title := strings.TrimSuffix(name, filepath.Ext(name))

		fileNode := &FolderNode{
			Name:     title,
			Path:     relPath,
			IsFolder: false,
		}
		nodeMap[parentPath].Children = append(nodeMap[parentPath].Children, fileNode)
	}

	return root
}

// ---------- Search ----------

func (a *GranitApp) Search(query string) []SearchHit {
	if a.vault == nil || a.vault.SearchIndex == nil {
		return nil
	}
	results := a.vault.SearchIndex.Search(query)
	hits := make([]SearchHit, 0, len(results))
	for _, r := range results {
		title := ""
		if note := a.vault.Notes[r.Path]; note != nil {
			title = note.Title
		}
		hits = append(hits, SearchHit{
			RelPath:   r.Path,
			Title:     title,
			Line:      r.Line,
			Column:    r.Column,
			MatchLine: r.MatchLine,
			Score:     r.Score,
		})
	}
	return hits
}

// ---------- Config ----------

func (a *GranitApp) GetTheme() string {
	return a.config.Theme
}

func (a *GranitApp) SetTheme(theme string) error {
	a.config.Theme = theme
	return a.config.Save()
}

// ---------- Vault assets handler (serves images/files from vault) ----------

type VaultAssetsHandler struct {
	app *GranitApp
}

func NewVaultAssetsHandler(app *GranitApp) *VaultAssetsHandler {
	return &VaultAssetsHandler{app: app}
}

func (h *VaultAssetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.app.vaultRoot == "" || !strings.HasPrefix(r.URL.Path, "/vault-assets/") {
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/vault-assets/")
	absPath := filepath.Join(h.app.vaultRoot, relPath)

	abs, err := filepath.Abs(absPath)
	if err != nil || !strings.HasPrefix(abs, h.app.vaultRoot) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, abs)
}
