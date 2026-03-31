package tui

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// ── WebDAV client ──────────────────────────────────────────────────────────

// NextcloudSync handles bidirectional file synchronisation with a Nextcloud
// server over WebDAV.  It uses only the Go standard library (net/http).
type NextcloudSync struct {
	baseURL    string // e.g. "https://cloud.example.com"
	user       string
	pass       string
	remotePath string // e.g. "/Notes"
	vaultRoot  string
	client     *http.Client
}

// NewNextcloudSync creates a configured sync client.
func NewNextcloudSync(cfg config.Config, vaultRoot string) *NextcloudSync {
	return &NextcloudSync{
		baseURL:    strings.TrimRight(cfg.NextcloudURL, "/"),
		user:       cfg.NextcloudUser,
		pass:       cfg.NextcloudPass,
		remotePath: "/" + strings.Trim(cfg.NextcloudPath, "/"),
		vaultRoot:  vaultRoot,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// davURL returns the full WebDAV URL for a relative path.
func (nc *NextcloudSync) davURL(relPath string) string {
	base := fmt.Sprintf("%s/remote.php/dav/files/%s%s",
		nc.baseURL, nc.user, nc.remotePath)
	if relPath == "" || relPath == "/" {
		return base
	}
	p := strings.TrimLeft(relPath, "/")
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	encoded := strings.Join(parts, "/")
	return base + "/" + encoded
}

// doReq builds and executes a WebDAV request with basic auth.
func (nc *NextcloudSync) doReq(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(nc.user, nc.pass)
	if method == "PROPFIND" {
		req.Header.Set("Depth", "infinity")
		req.Header.Set("Content-Type", "application/xml")
	}
	return nc.client.Do(req)
}

// TestConnection verifies that the credentials and URL are valid.
func (nc *NextcloudSync) TestConnection() error {
	if nc.baseURL == "" || nc.user == "" || nc.pass == "" {
		return fmt.Errorf("nextcloud URL, username, and password are required")
	}
	resp, err := nc.doReq("PROPFIND", nc.davURL(""), strings.NewReader(propfindBody))
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("remote path not found — check NextcloudPath setting")
	}
	if resp.StatusCode != 207 && resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ── PROPFIND XML parsing ───────────────────────────────────────────────────

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:getlastmodified/>
    <d:getcontentlength/>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

type davMultistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string      `xml:"href"`
	Propstat davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop   davProp `xml:"prop"`
	Status string  `xml:"status"`
}

type davProp struct {
	LastModified string          `xml:"getlastmodified"`
	Length       int64           `xml:"getcontentlength"`
	ResourceType davResourceType `xml:"resourcetype"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

type remoteFile struct {
	Path    string
	ModTime time.Time
	IsDir   bool
	Size    int64
}

// listRemote returns all files and directories under the remote sync path.
func (nc *NextcloudSync) listRemote() ([]remoteFile, error) {
	resp, err := nc.doReq("PROPFIND", nc.davURL(""), strings.NewReader(propfindBody))
	if err != nil {
		return nil, fmt.Errorf("PROPFIND failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 207 {
		return nil, fmt.Errorf("PROPFIND returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ms davMultistatus
	if err := xml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("failed to parse PROPFIND response: %w", err)
	}

	// Build the base href prefix we strip from each response href.
	basePath := fmt.Sprintf("/remote.php/dav/files/%s%s", nc.user, nc.remotePath)
	basePath = strings.TrimRight(basePath, "/")

	var files []remoteFile
	for _, r := range ms.Responses {
		href := r.Href
		// Strip the base path to get the relative path.
		rel := strings.TrimPrefix(href, basePath)
		rel = strings.TrimLeft(rel, "/")
		if rel == "" {
			continue // skip the root collection itself
		}

		// URL-decode the relative path from the href.
		relPath, err := url.PathUnescape(rel)
		if err != nil {
			continue
		}
		rel = relPath

		isDir := r.Propstat.Prop.ResourceType.Collection != nil

		var modTime time.Time
		if r.Propstat.Prop.LastModified != "" {
			modTime, _ = time.Parse(time.RFC1123, r.Propstat.Prop.LastModified)
			if modTime.IsZero() {
				var err2 error
				modTime, err2 = time.Parse(time.RFC1123Z, r.Propstat.Prop.LastModified)
				if err2 != nil {
					log.Printf("nextcloud: unable to parse lastmodified %q: %v", r.Propstat.Prop.LastModified, err2)
					// zero time will force a sync
				}
			}
		}

		// Remove trailing slash from directory paths.
		rel = strings.TrimRight(rel, "/")

		files = append(files, remoteFile{
			Path:    rel,
			ModTime: modTime,
			IsDir:   isDir,
			Size:    r.Propstat.Prop.Length,
		})
	}
	return files, nil
}

// ── File operations ────────────────────────────────────────────────────────

// safePath validates that a relative path doesn't escape the vault root.
func safePath(vaultRoot, rel string) (string, error) {
	abs := filepath.Join(vaultRoot, rel)
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, filepath.Clean(vaultRoot)+string(filepath.Separator)) &&
		abs != filepath.Clean(vaultRoot) {
		return "", fmt.Errorf("path traversal detected: %s", rel)
	}
	return abs, nil
}

// httpStatusMessage returns a human-readable error for common HTTP status codes.
func httpStatusMessage(code int) string {
	switch code {
	case 401:
		return fmt.Sprintf("HTTP %d: authentication required — check username/password", code)
	case 403:
		return fmt.Sprintf("HTTP %d: access forbidden — check permissions", code)
	case 404:
		return fmt.Sprintf("HTTP %d: not found — check remote path", code)
	case 409:
		return fmt.Sprintf("HTTP %d: conflict — parent directory may not exist", code)
	default:
		return fmt.Sprintf("HTTP %d", code)
	}
}

// uploadFile sends a local file to the remote via PUT.
func (nc *NextcloudSync) uploadFile(relPath string) error {
	localPath, err := safePath(nc.vaultRoot, relPath)
	if err != nil {
		return err
	}

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	req, err := http.NewRequest("PUT", nc.davURL(relPath), f)
	if err != nil {
		return err
	}
	req.SetBasicAuth(nc.user, nc.pass)
	if strings.HasSuffix(strings.ToLower(relPath), ".md") {
		req.Header.Set("Content-Type", "text/markdown; charset=utf-8")
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	resp, err := nc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("PUT %s: %s", relPath, httpStatusMessage(resp.StatusCode))
	}
	return nil
}

// downloadFile fetches a remote file and writes it locally.
func (nc *NextcloudSync) downloadFile(relPath string) error {
	localPath, err := safePath(nc.vaultRoot, relPath)
	if err != nil {
		return err
	}

	resp, err := nc.doReq("GET", nc.davURL(relPath), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s: %s", relPath, httpStatusMessage(resp.StatusCode))
	}

	absPath := localPath
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	out, err := os.Create(absPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, copyErr := io.Copy(out, resp.Body); copyErr != nil {
		os.Remove(absPath)
		return copyErr
	}
	return nil
}

// mkcolRemote creates a remote directory (and parents) via MKCOL.
func (nc *NextcloudSync) mkcolRemote(relDir string) error {
	// Create parent directories first.
	parts := strings.Split(relDir, "/")
	for i := range parts {
		partial := strings.Join(parts[:i+1], "/")
		resp, err := nc.doReq("MKCOL", nc.davURL(partial), nil)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
			return err
		}
		resp.Body.Close()
		// 201 = created, 405 = already exists — both OK.
		if resp.StatusCode >= 400 && resp.StatusCode != 405 {
			return fmt.Errorf("MKCOL %s returned %d", partial, resp.StatusCode)
		}
	}
	return nil
}

// Push uploads all local markdown files to the remote.
func (nc *NextcloudSync) Push() error {
	// Ensure remote root exists.
	// Ignore error — root may already exist.
	_ = nc.mkcolRemote("")

	return filepath.Walk(nc.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(nc.vaultRoot, path)
		if relErr != nil {
			return relErr
		}
		// Skip hidden files/dirs (.granit, .git, etc.)
		if strings.HasPrefix(filepath.Base(rel), ".") && rel != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if rel != "." {
				return nc.mkcolRemote(rel)
			}
			return nil
		}
		return nc.uploadFile(rel)
	})
}

// Pull downloads all remote files to the local vault.
func (nc *NextcloudSync) Pull() error {
	remoteFiles, err := nc.listRemote()
	if err != nil {
		return err
	}

	for _, rf := range remoteFiles {
		if rf.IsDir {
			localDir, pathErr := safePath(nc.vaultRoot, rf.Path)
			if pathErr != nil {
				continue
			}
			_ = os.MkdirAll(localDir, 0755)
			continue
		}
		if err := nc.downloadFile(rf.Path); err != nil {
			return fmt.Errorf("downloading %s: %w", rf.Path, err)
		}
	}
	return nil
}

// Sync performs bidirectional sync.  For each file that exists on both sides
// the newer version wins.  Files that only exist on one side are transferred
// to the other.  Returns counts of pushed and pulled files.
func (nc *NextcloudSync) Sync() (pushed int, pulled int, err error) {
	remoteFiles, err := nc.listRemote()
	if err != nil {
		return 0, 0, err
	}

	// Build a map of remote files.
	remoteMap := make(map[string]remoteFile, len(remoteFiles))
	for _, rf := range remoteFiles {
		remoteMap[rf.Path] = rf
	}

	// Walk local files.
	localSeen := make(map[string]bool)
	walkErr := filepath.Walk(nc.vaultRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(nc.vaultRoot, path)
		if relErr != nil {
			return relErr
		}
		if strings.HasPrefix(filepath.Base(rel), ".") && rel != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		localSeen[rel] = true

		rf, existsRemote := remoteMap[rel]
		if !existsRemote {
			// Only local — push.
			if uploadErr := nc.uploadFile(rel); uploadErr != nil {
				return uploadErr
			}
			pushed++
			return nil
		}

		// Both exist — compare modification times.  Newer wins.
		localMod := info.ModTime().UTC()
		diff := localMod.Sub(rf.ModTime)
		if diff < 0 {
			diff = -diff
		}
		if diff <= time.Second {
			return nil // already in sync
		}
		if localMod.After(rf.ModTime) {
			if uploadErr := nc.uploadFile(rel); uploadErr != nil {
				return uploadErr
			}
			pushed++
		} else if rf.ModTime.After(localMod) {
			if dlErr := nc.downloadFile(rel); dlErr != nil {
				return dlErr
			}
			pulled++
		}
		return nil
	})
	if walkErr != nil {
		return pushed, pulled, walkErr
	}

	// Files that only exist on remote — pull them.
	for _, rf := range remoteFiles {
		if rf.IsDir {
			continue
		}
		if !localSeen[rf.Path] {
			if dlErr := nc.downloadFile(rf.Path); dlErr != nil {
				return pushed, pulled, dlErr
			}
			pulled++
		}
	}

	return pushed, pulled, nil
}

// ── TUI messages ───────────────────────────────────────────────────────────

type ncTestResultMsg struct{ err error }
type ncPushResultMsg struct{ err error }
type ncPullResultMsg struct{ err error }
type ncSyncResultMsg struct {
	pushed int
	pulled int
	err    error
}

// ── TUI overlay ────────────────────────────────────────────────────────────

// NextcloudOverlay is the TUI panel for interacting with Nextcloud sync.
type NextcloudOverlay struct {
	active    bool
	width     int
	height    int
	cursor    int
	message   string
	msgStyle  string // "ok", "err", "info"
	running   bool   // an operation is in progress
	config    config.Config
	vaultRoot string

	lastSyncTime time.Time
	lastPushed   int
	lastPulled   int
}

var ncButtons = []string{"Test Connection", "Push", "Pull", "Sync", "Close"}

// NewNextcloudOverlay creates a new overlay.
func NewNextcloudOverlay() NextcloudOverlay {
	return NextcloudOverlay{}
}

func (n NextcloudOverlay) IsActive() bool { return n.active }

func (n *NextcloudOverlay) Open(cfg config.Config, vaultRoot string) {
	n.active = true
	n.cursor = 0
	n.config = cfg
	n.vaultRoot = vaultRoot
	n.message = ""
	n.running = false
}

func (n *NextcloudOverlay) Close() {
	n.active = false
}

func (n *NextcloudOverlay) SetSize(w, h int) {
	n.width = w
	n.height = h
}

func (n *NextcloudOverlay) Update(msg tea.Msg) (NextcloudOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case ncTestResultMsg:
		n.running = false
		if msg.err != nil {
			n.message = "Connection failed: " + msg.err.Error()
			n.msgStyle = "err"
		} else {
			n.message = "Connection successful!"
			n.msgStyle = "ok"
		}

	case ncPushResultMsg:
		n.running = false
		if msg.err != nil {
			n.message = "Push failed: " + msg.err.Error()
			n.msgStyle = "err"
		} else {
			n.message = "Push completed successfully"
			n.msgStyle = "ok"
			n.lastSyncTime = time.Now()
		}

	case ncPullResultMsg:
		n.running = false
		if msg.err != nil {
			n.message = "Pull failed: " + msg.err.Error()
			n.msgStyle = "err"
		} else {
			n.message = "Pull completed successfully"
			n.msgStyle = "ok"
			n.lastSyncTime = time.Now()
		}

	case ncSyncResultMsg:
		n.running = false
		if msg.err != nil {
			n.message = "Sync failed: " + msg.err.Error()
			n.msgStyle = "err"
		} else {
			n.message = fmt.Sprintf("Sync complete: %d pushed, %d pulled", msg.pushed, msg.pulled)
			n.msgStyle = "ok"
			n.lastSyncTime = time.Now()
			n.lastPushed = msg.pushed
			n.lastPulled = msg.pulled
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			n.Close()
			return *n, nil
		case "left", "h":
			if n.cursor > 0 {
				n.cursor--
			}
		case "right", "l":
			if n.cursor < len(ncButtons)-1 {
				n.cursor++
			}
		case "tab":
			n.cursor = (n.cursor + 1) % len(ncButtons)
		case "enter":
			if n.running {
				return *n, nil
			}
			return n.executeButton()
		}
	}
	return *n, nil
}

func (n *NextcloudOverlay) executeButton() (NextcloudOverlay, tea.Cmd) {
	nc := NewNextcloudSync(n.config, n.vaultRoot)
	switch ncButtons[n.cursor] {
	case "Test Connection":
		n.running = true
		n.message = "Testing connection..."
		n.msgStyle = "info"
		return *n, func() tea.Msg {
			return ncTestResultMsg{err: nc.TestConnection()}
		}
	case "Push":
		n.running = true
		n.message = "Pushing files..."
		n.msgStyle = "info"
		return *n, func() tea.Msg {
			return ncPushResultMsg{err: nc.Push()}
		}
	case "Pull":
		n.running = true
		n.message = "Pulling files..."
		n.msgStyle = "info"
		return *n, func() tea.Msg {
			return ncPullResultMsg{err: nc.Pull()}
		}
	case "Sync":
		n.running = true
		n.message = "Syncing..."
		n.msgStyle = "info"
		return *n, func() tea.Msg {
			pushed, pulled, err := nc.Sync()
			return ncSyncResultMsg{pushed: pushed, pulled: pulled, err: err}
		}
	case "Close":
		n.Close()
	}
	return *n, nil
}

func (n NextcloudOverlay) View() string {
	w := n.width - 4
	if w < 40 {
		w = 40
	}
	if w > 80 {
		w = 80
	}
	h := 16

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(mauve).
		Render("  " + IconSaveChar + " Nextcloud Sync")

	// Connection info
	ncURL := n.config.NextcloudURL
	if ncURL == "" {
		ncURL = "(not configured)"
	}
	ncUser := n.config.NextcloudUser
	if ncUser == "" {
		ncUser = "(not set)"
	}
	remotePath := n.config.NextcloudPath
	if remotePath == "" {
		remotePath = "/"
	}

	labelStyle := lipgloss.NewStyle().Foreground(subtext0)
	valStyle := lipgloss.NewStyle().Foreground(text)
	info := labelStyle.Render("  Server:  ") + valStyle.Render(ncURL) + "\n" +
		labelStyle.Render("  User:    ") + valStyle.Render(ncUser) + "\n" +
		labelStyle.Render("  Path:    ") + valStyle.Render(remotePath)
	if n.config.NextcloudAutoSync {
		info += "\n" + labelStyle.Render("  Auto:    ") + lipgloss.NewStyle().Foreground(green).Bold(true).Render("enabled")
	}

	// Last sync
	var syncInfo string
	if !n.lastSyncTime.IsZero() {
		syncInfo = "\n" + labelStyle.Render("  Last:    ") +
			valStyle.Render(fmt.Sprintf("%s (%d up, %d down)",
				n.lastSyncTime.Format("15:04:05"),
				n.lastPushed, n.lastPulled))
	}

	// Buttons
	var btns []string
	for i, label := range ncButtons {
		s := lipgloss.NewStyle().Padding(0, 1)
		if i == n.cursor {
			s = s.Bold(true).
				Background(mauve).
				Foreground(base)
		} else {
			s = s.Foreground(overlay0)
		}
		if n.running && label != "Close" {
			s = s.Foreground(surface1).Strikethrough(true)
		}
		btns = append(btns, s.Render(label))
	}
	buttonRow := strings.Join(btns, "  ")

	// Status message
	var msgLine string
	if n.message != "" {
		msgColor := overlay0
		switch n.msgStyle {
		case "ok":
			msgColor = green
		case "err":
			msgColor = red
		case "info":
			msgColor = blue
		}
		msgLine = lipgloss.NewStyle().Foreground(msgColor).Render("  " + n.message)
	}

	body := title + "\n" +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", w-6)) + "\n" +
		info + syncInfo + "\n\n" + buttonRow
	if msgLine != "" {
		body += "\n\n" + msgLine
	}
	body += "\n\n" + lipgloss.NewStyle().Foreground(overlay0).Render("  esc/q close  tab/arrows navigate  enter select")

	box := lipgloss.NewStyle().
		Border(PanelBorder).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(w).
		Height(h).
		Render(body)

	return lipgloss.Place(n.width, n.height, lipgloss.Center, lipgloss.Center, box)
}
