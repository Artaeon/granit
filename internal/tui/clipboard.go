package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dropLastRune removes the last UTF-8 rune from a string safely.
func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// clipboardTimeout is the max duration for clipboard tool execution.
const clipboardTimeout = 3 * time.Second

// ---------------------------------------------------------------------------
// Clipboard — system clipboard read/write via CLI tools
// ---------------------------------------------------------------------------

// Clipboard provides platform-agnostic system clipboard access.
type Clipboard struct{}

// ClipboardCopy copies the given text to the system clipboard.
// It tries platform-appropriate tools and falls back gracefully.
func ClipboardCopy(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "linux":
		// Try Wayland first, then X11 tools
		if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.CommandContext(ctx, path)
		} else if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, path, "-selection", "clipboard")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("clipboard: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("clipboard: %w", err)
	}

	if _, err := io.WriteString(stdin, text); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("clipboard: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("clipboard: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("clipboard: %w", err)
	}
	return nil
}

// ClipboardPaste reads text from the system clipboard.
func ClipboardPaste() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbpaste")
	case "linux":
		if path, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.CommandContext(ctx, path)
		} else if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, path, "-selection", "clipboard", "-o")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-paste)")
		}
	default:
		return "", fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	return string(out), nil
}

// ClipboardAvailable checks whether clipboard tools for both copy and paste are installed.
func ClipboardAvailable() bool {
	switch runtime.GOOS {
	case "darwin":
		_, errCopy := exec.LookPath("pbcopy")
		_, errPaste := exec.LookPath("pbpaste")
		return errCopy == nil && errPaste == nil
	case "linux":
		// Need both copy and paste tools; xclip/xsel handle both
		for _, tool := range []string{"xclip", "xsel"} {
			if _, err := exec.LookPath(tool); err == nil {
				return true
			}
		}
		// Wayland: need both wl-copy and wl-paste
		_, errCopy := exec.LookPath("wl-copy")
		_, errPaste := exec.LookPath("wl-paste")
		return errCopy == nil && errPaste == nil
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Web Clip Result — message returned by the async fetch command
// ---------------------------------------------------------------------------

type webClipResult struct {
	title   string
	content string
	url     string
	err     error
}

// webClipTickMsg drives the loading animation.
type webClipTickMsg struct{}

// ---------------------------------------------------------------------------
// WebClipper overlay — fetch a URL, extract content, prepare a markdown note
// ---------------------------------------------------------------------------

// clipFormat controls the output format of the web clip.
type clipFormat int

const (
	clipFormatFull       clipFormat = iota // frontmatter + heading + content
	clipFormatSimplified                   // just content, no frontmatter
)

// WebClipper is an overlay for configuring and previewing a web clip.
type WebClipper struct {
	active bool
	width  int
	height int

	url     string
	title   string
	content string
	loading bool
	loadingTick int
	done    bool
	errored bool // true when fetch failed — prevents saving

	// Result
	resultReady   bool
	resultTitle   string
	resultContent string

	// Internal UI state
	editingTitle bool
	titleBuf     string
	scrollOffset int

	// URL input mode — when opened with empty URL
	urlInputMode bool
	urlBuf       string

	// Save format toggle
	format clipFormat

	// Custom tags
	editingTags bool
	tagsBuf     string
	customTags  []string
}

// NewWebClipper returns a zero-value WebClipper ready for use.
func NewWebClipper() WebClipper {
	return WebClipper{}
}

// IsActive reports whether the overlay is currently visible.
func (wc *WebClipper) IsActive() bool {
	return wc.active
}

// Open activates the web clipper. If url is empty, the URL input field is shown.
func (wc *WebClipper) Open(url string) {
	wc.active = true
	wc.url = url
	wc.title = ""
	wc.content = ""
	wc.loadingTick = 0
	wc.done = false
	wc.errored = false
	wc.resultReady = false
	wc.resultTitle = ""
	wc.resultContent = ""
	wc.editingTitle = false
	wc.titleBuf = ""
	wc.scrollOffset = 0
	wc.format = clipFormatFull
	wc.editingTags = false
	wc.tagsBuf = ""
	wc.customTags = nil

	if url == "" {
		// URL input mode — wait for user to type URL
		wc.urlInputMode = true
		wc.urlBuf = ""
		wc.loading = false
	} else {
		wc.urlInputMode = false
		wc.urlBuf = ""
		wc.loading = true
	}
}

// Close deactivates the overlay and resets state.
func (wc *WebClipper) Close() {
	wc.active = false
	wc.loading = false
	wc.done = false
	wc.errored = false
	wc.resultReady = false
	wc.editingTitle = false
	wc.urlInputMode = false
	wc.editingTags = false
}

// SetSize updates the overlay dimensions.
func (wc *WebClipper) SetSize(w, h int) {
	wc.width = w
	wc.height = h
}

// GetResult returns the final title, markdown content, and whether a result
// is ready to be saved.
func (wc *WebClipper) GetResult() (title, content string, ok bool) {
	if !wc.resultReady {
		return "", "", false
	}
	return wc.resultTitle, wc.resultContent, true
}

// Update processes messages for the web clipper overlay.
func (wc WebClipper) Update(msg tea.Msg) (WebClipper, tea.Cmd) {
	if !wc.active {
		return wc, nil
	}

	switch msg := msg.(type) {
	case webClipResult:
		wc.loading = false
		if msg.err != nil {
			wc.content = "Error: " + msg.err.Error()
			wc.title = "Error"
			wc.done = true
			wc.errored = true
			return wc, nil
		}
		wc.title = msg.title
		wc.content = msg.content
		wc.url = msg.url
		wc.done = true
		wc.errored = false
		return wc, nil

	case webClipTickMsg:
		if wc.loading {
			wc.loadingTick++
			return wc, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
				return webClipTickMsg{}
			})
		}
		return wc, nil

	case tea.KeyMsg:
		// URL input mode
		if wc.urlInputMode {
			return wc.updateURLInput(msg)
		}

		// Editing title mode
		if wc.editingTitle {
			return wc.updateEditTitle(msg)
		}

		// Editing tags mode
		if wc.editingTags {
			return wc.updateEditTags(msg)
		}

		// Normal navigation
		switch msg.String() {
		case "esc":
			wc.Close()
			return wc, nil

		case "enter":
			if wc.done && wc.title != "" && !wc.errored {
				wc.resultReady = true
				wc.resultTitle = wc.title
				wc.resultContent = wc.buildOutput()
				wc.active = false
				return wc, nil
			}

		case "e":
			if wc.done {
				wc.editingTitle = true
				wc.titleBuf = wc.title
			}

		case "f":
			if wc.done {
				if wc.format == clipFormatFull {
					wc.format = clipFormatSimplified
				} else {
					wc.format = clipFormatFull
				}
			}

		case "t":
			if wc.done {
				wc.editingTags = true
				wc.tagsBuf = strings.Join(wc.customTags, ", ")
			}

		case "j", "down":
			if wc.done {
				wc.scrollOffset++
				maxScroll := wc.maxScroll()
				if wc.scrollOffset > maxScroll {
					wc.scrollOffset = maxScroll
				}
			}

		case "k", "up":
			if wc.done {
				wc.scrollOffset--
				if wc.scrollOffset < 0 {
					wc.scrollOffset = 0
				}
			}
		}
	}

	return wc, nil
}

// updateURLInput handles key events in URL input mode.
func (wc WebClipper) updateURLInput(msg tea.KeyMsg) (WebClipper, tea.Cmd) {
	switch msg.String() {
	case "enter":
		u := strings.TrimSpace(wc.urlBuf)
		if u == "" {
			return wc, nil
		}
		// Auto-add scheme if missing
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = "https://" + u
		}
		wc.url = u
		wc.urlInputMode = false
		wc.loading = true
		return wc, tea.Batch(
			fetchAndClip(u),
			tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
				return webClipTickMsg{}
			}),
		)
	case "esc":
		wc.active = false
		return wc, nil
	case "backspace":
		if len(wc.urlBuf) > 0 {
			wc.urlBuf = dropLastRune(wc.urlBuf)
		}
	case "ctrl+u":
		wc.urlBuf = ""
	case "ctrl+v":
		if pasted, err := ClipboardPaste(); err == nil {
			pasted = strings.TrimSpace(pasted)
			// Only paste first line
			if idx := strings.IndexAny(pasted, "\r\n"); idx >= 0 {
				pasted = pasted[:idx]
			}
			wc.urlBuf += pasted
		}
	case "space":
		// No spaces in URLs — ignore
	default:
		ch := msg.String()
		if len(ch) == 1 {
			wc.urlBuf += ch
		}
	}
	return wc, nil
}

// updateEditTitle handles key events while editing the title.
func (wc WebClipper) updateEditTitle(msg tea.KeyMsg) (WebClipper, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if wc.titleBuf != "" {
			wc.title = wc.titleBuf
		}
		wc.editingTitle = false
	case "esc":
		wc.editingTitle = false
	case "backspace":
		if len(wc.titleBuf) > 0 {
			wc.titleBuf = dropLastRune(wc.titleBuf)
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			wc.titleBuf += ch
		}
	}
	return wc, nil
}

// updateEditTags handles key events while editing tags.
func (wc WebClipper) updateEditTags(msg tea.KeyMsg) (WebClipper, tea.Cmd) {
	switch msg.String() {
	case "enter":
		wc.customTags = nil
		for _, t := range strings.Split(wc.tagsBuf, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				wc.customTags = append(wc.customTags, t)
			}
		}
		wc.editingTags = false
	case "esc":
		wc.editingTags = false
	case "backspace":
		if len(wc.tagsBuf) > 0 {
			wc.tagsBuf = dropLastRune(wc.tagsBuf)
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			wc.tagsBuf += ch
		}
	}
	return wc, nil
}

// yamlEscape quotes a string for safe inclusion in YAML frontmatter.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":\n\"'{}[]#&*!|>,@`") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}

// buildOutput assembles the final markdown output based on current settings.
func (wc WebClipper) buildOutput() string {
	var sb strings.Builder

	if wc.format == clipFormatFull {
		now := time.Now().Format("2006-01-02")
		sb.WriteString("---\n")
		sb.WriteString("source: " + yamlEscape(wc.url) + "\n")
		sb.WriteString("clipped: " + now + "\n")

		// Build tags list with escaped values
		var quotedTags []string
		for _, t := range append([]string{"clipped"}, wc.customTags...) {
			quotedTags = append(quotedTags, yamlEscape(t))
		}
		sb.WriteString("tags: [" + strings.Join(quotedTags, ", ") + "]\n")
		sb.WriteString("---\n\n")
		sb.WriteString("# " + wc.title + "\n\n")
	}

	sb.WriteString(wc.content)
	return sb.String()
}

// maxScroll returns the maximum scroll offset for the content preview.
func (wc WebClipper) maxScroll() int {
	lines := strings.Count(wc.content, "\n") + 1
	viewH := wc.previewHeight()
	if lines > viewH {
		return lines - viewH
	}
	return 0
}

// previewHeight returns how many lines are available for the content preview.
func (wc WebClipper) previewHeight() int {
	// Reserve space for header, URL line, title line, separator, help, border
	h := wc.height/2 - 12
	if h < 5 {
		h = 5
	}
	return h
}

// View renders the web clipper overlay.
func (wc WebClipper) View() string {
	width := wc.width / 2
	if width < 56 {
		width = 56
	}
	if width > 80 {
		width = 80
	}
	innerW := width - 6

	var b strings.Builder

	// Header
	titleStr := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Web Clipper")
	b.WriteString(titleStr)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n\n")

	// URL input mode
	if wc.urlInputMode {
		urlLabel := lipgloss.NewStyle().Foreground(text).Render("  URL: ")
		cursor := lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u2588")
		urlVal := lipgloss.NewStyle().
			Foreground(blue).
			Bold(true).
			Render(wc.urlBuf + cursor)
		b.WriteString(urlLabel + urlVal)
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Type or paste a URL and press Enter"))
		b.WriteString("\n\n")

		helpKeys := lipgloss.NewStyle().Foreground(surface0).Background(overlay0).Padding(0, 1)
		helpDesc := DimStyle
		b.WriteString("  ")
		b.WriteString(helpKeys.Render("Enter") + helpDesc.Render(" fetch") + "  ")
		b.WriteString(helpKeys.Render("Ctrl+V") + helpDesc.Render(" paste") + "  ")
		b.WriteString(helpKeys.Render("Ctrl+U") + helpDesc.Render(" clear") + "  ")
		b.WriteString(helpKeys.Render("Esc") + helpDesc.Render(" cancel"))

		border := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(mauve).
			Padding(1, 2).
			Width(width).
			Background(mantle)

		return border.Render(b.String())
	}

	// URL
	urlLabel := lipgloss.NewStyle().Foreground(text).Render("  URL: ")
	urlValue := lipgloss.NewStyle().Foreground(blue).Render(truncate(wc.url, innerW-8))
	b.WriteString(urlLabel + urlValue)
	b.WriteString("\n\n")

	if wc.loading {
		// Loading animation
		frames := []string{"\u28f7", "\u28ef", "\u28df", "\u287f", "\u28bf", "\u28fb", "\u28fd", "\u28fe"}
		frame := frames[wc.loadingTick%len(frames)]
		spinner := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(frame)
		label := lipgloss.NewStyle().Foreground(text).Render(" Fetching page...")
		b.WriteString("  " + spinner + label)
		b.WriteString("\n")
	} else if wc.done {
		// Title
		titleLabel := lipgloss.NewStyle().Foreground(text).Render("  Title: ")
		if wc.editingTitle {
			cursor := lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u2588")
			editVal := lipgloss.NewStyle().
				Foreground(peach).
				Bold(true).
				Render(wc.titleBuf + cursor)
			b.WriteString(titleLabel + editVal)
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  Enter: confirm  Esc: cancel"))
		} else {
			titleVal := lipgloss.NewStyle().
				Foreground(peach).
				Bold(true).
				Render(truncate(wc.title, innerW-10))
			b.WriteString(titleLabel + titleVal)
		}
		b.WriteString("\n")

		// Format indicator
		formatLabel := lipgloss.NewStyle().Foreground(text).Render("  Format: ")
		formatName := "full (frontmatter + content)"
		if wc.format == clipFormatSimplified {
			formatName = "simplified (content only)"
		}
		formatVal := lipgloss.NewStyle().Foreground(teal).Render(formatName)
		b.WriteString(formatLabel + formatVal)
		b.WriteString("\n")

		// Tags
		tagsLabel := lipgloss.NewStyle().Foreground(text).Render("  Tags: ")
		if wc.editingTags {
			cursor := lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u2588")
			editVal := lipgloss.NewStyle().
				Foreground(yellow).
				Bold(true).
				Render(wc.tagsBuf + cursor)
			b.WriteString(tagsLabel + editVal)
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  comma-separated  Enter: confirm  Esc: cancel"))
		} else {
			allTags := []string{"clipped"}
			allTags = append(allTags, wc.customTags...)
			tagsVal := lipgloss.NewStyle().Foreground(yellow).Render(strings.Join(allTags, ", "))
			b.WriteString(tagsLabel + tagsVal)
		}
		b.WriteString("\n\n")

		// Content preview
		previewLabel := SearchPromptStyle.Render("  Preview:")
		b.WriteString(previewLabel)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)))
		b.WriteString("\n")

		lines := strings.Split(wc.content, "\n")
		viewH := wc.previewHeight()
		start := wc.scrollOffset
		if start > len(lines) {
			start = len(lines)
		}
		end := start + viewH
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[start:end]

		contentStyle := lipgloss.NewStyle().Foreground(text)
		for _, line := range visible {
			rendered := contentStyle.Render("  " + truncate(line, innerW-4))
			b.WriteString(rendered)
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(lines) > viewH {
			pct := 0
			if wc.maxScroll() > 0 {
				pct = wc.scrollOffset * 100 / wc.maxScroll()
			}
			scrollInfo := DimStyle.Render(fmt.Sprintf("  [%d%%] %d/%d lines", pct, start+1, len(lines)))
			b.WriteString(scrollInfo)
			b.WriteString("\n")
		}

		b.WriteString("\n")

		// Help
		if !wc.editingTitle && !wc.editingTags {
			helpKeys := lipgloss.NewStyle().Foreground(surface0).Background(overlay0).Padding(0, 1)
			helpDesc := DimStyle

			b.WriteString("  ")
			b.WriteString(helpKeys.Render("Enter") + helpDesc.Render(" save") + "  ")
			b.WriteString(helpKeys.Render("e") + helpDesc.Render(" title") + "  ")
			b.WriteString(helpKeys.Render("f") + helpDesc.Render(" format") + "  ")
			b.WriteString(helpKeys.Render("t") + helpDesc.Render(" tags") + "\n  ")
			b.WriteString(helpKeys.Render("j/k") + helpDesc.Render(" scroll") + "  ")
			b.WriteString(helpKeys.Render("Esc") + helpDesc.Render(" cancel"))
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// fetchAndClip — async command to HTTP GET a URL and extract content
// ---------------------------------------------------------------------------

// fetchAndClip returns a tea.Cmd that fetches the URL, extracts the title
// and body content, converts to markdown, and sends back a webClipResult.
func fetchAndClip(rawURL string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return webClipResult{url: rawURL, err: fmt.Errorf("invalid URL: %w", err)}
		}
		req.Header.Set("User-Agent", "Granit/1.0 WebClipper")

		resp, err := client.Do(req)
		if err != nil {
			return webClipResult{url: rawURL, err: fmt.Errorf("fetch failed: %w", err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB limit
		if err != nil {
			return webClipResult{url: rawURL, err: fmt.Errorf("read failed: %w", err)}
		}

		html := string(body)

		// Derive base URL for resolving relative paths
		baseURL := ""
		if parsed, err := url.Parse(rawURL); err == nil {
			baseURL = parsed.Scheme + "://" + parsed.Host
		}

		// Extract <title>
		title := extractTitle(html)
		if title == "" {
			title = extractOGTitle(html)
		}
		if title == "" {
			title = "Untitled"
		}

		// Try reader-mode extraction: prefer <article> or <main>
		contentHTML := extractReaderContent(html)

		// Convert HTML to markdown-like text
		content := htmlToMarkdown(contentHTML, baseURL)

		return webClipResult{
			title:   title,
			content: content,
			url:     rawURL,
		}
	}
}

// ---------------------------------------------------------------------------
// HTML → Markdown extraction (regex-based, no external parser)
// ---------------------------------------------------------------------------

var (
	reClipTitle      = regexp.MustCompile(`(?i)<title[^>]*>([\s\S]*?)</title>`)
	reOGTitle        = regexp.MustCompile(`(?i)<meta\s[^>]*property\s*=\s*"og:title"[^>]*content\s*=\s*"([^"]*)"`)
	reOGTitleAlt     = regexp.MustCompile(`(?i)<meta\s[^>]*content\s*=\s*"([^"]*)"[^>]*property\s*=\s*"og:title"`)
	reScript         = regexp.MustCompile(`(?is)<script[\s>][\s\S]*?</script>`)
	reStyle          = regexp.MustCompile(`(?is)<style[\s>][\s\S]*?</style>`)
	reNav            = regexp.MustCompile(`(?is)<nav[\s>][\s\S]*?</nav>`)
	reFooter         = regexp.MustCompile(`(?is)<footer[\s>][\s\S]*?</footer>`)
	reHeader         = regexp.MustCompile(`(?is)<header[\s>][\s\S]*?</header>`)
	reAside          = regexp.MustCompile(`(?is)<aside[\s>][\s\S]*?</aside>`)
	reForm           = regexp.MustCompile(`(?is)<form[\s>][\s\S]*?</form>`)
	reIframe         = regexp.MustCompile(`(?is)<iframe[\s>][\s\S]*?</iframe>`)
	reArticle        = regexp.MustCompile(`(?is)<article[^>]*>([\s\S]*?)</article>`)
	reMain           = regexp.MustCompile(`(?is)<main[^>]*>([\s\S]*?)</main>`)
	reH1             = regexp.MustCompile(`(?i)<h1[^>]*>([\s\S]*?)</h1>`)
	reH2             = regexp.MustCompile(`(?i)<h2[^>]*>([\s\S]*?)</h2>`)
	reH3             = regexp.MustCompile(`(?i)<h3[^>]*>([\s\S]*?)</h3>`)
	reH4             = regexp.MustCompile(`(?i)<h4[^>]*>([\s\S]*?)</h4>`)
	reH5             = regexp.MustCompile(`(?i)<h5[^>]*>([\s\S]*?)</h5>`)
	reH6             = regexp.MustCompile(`(?i)<h6[^>]*>([\s\S]*?)</h6>`)
	reParagraph      = regexp.MustCompile(`(?i)<p[^>]*>([\s\S]*?)</p>`)
	reAnchor         = regexp.MustCompile(`(?i)<a\s[^>]*href="([^"]*)"[^>]*>([\s\S]*?)</a>`)
	reStrong         = regexp.MustCompile(`(?i)<(?:strong|b)[^>]*>([\s\S]*?)</(?:strong|b)>`)
	reEm             = regexp.MustCompile(`(?i)<(?:em|i)[^>]*>([\s\S]*?)</(?:em|i)>`)
	reStrikethrough  = regexp.MustCompile(`(?i)<(?:del|s)[^>]*>([\s\S]*?)</(?:del|s)>`)
	reSup            = regexp.MustCompile(`(?i)<sup[^>]*>([\s\S]*?)</sup>`)
	reSub            = regexp.MustCompile(`(?i)<sub[^>]*>([\s\S]*?)</sub>`)
	clipReListItem       = regexp.MustCompile(`(?i)<li[^>]*>([\s\S]*?)</li>`)
	clipReBlockquote     = regexp.MustCompile(`(?i)<blockquote[^>]*>([\s\S]*?)</blockquote>`)
	reCode           = regexp.MustCompile(`(?i)<code[^>]*>([\s\S]*?)</code>`)
	rePreCode        = regexp.MustCompile(`(?is)<pre[^>]*>\s*<code(?:\s+class="(?:language-)?([^"]*)")?[^>]*>([\s\S]*?)</code>\s*</pre>`)
	rePre            = regexp.MustCompile(`(?is)<pre[^>]*>([\s\S]*?)</pre>`)
	reImg            = regexp.MustCompile(`(?i)<img\s[^>]*>`)
	reImgSrc         = regexp.MustCompile(`(?i)src="([^"]*)"`)
	reImgAlt         = regexp.MustCompile(`(?i)alt="([^"]*)"`)
	reFigure         = regexp.MustCompile(`(?is)<figure[^>]*>([\s\S]*?)</figure>`)
	reFigCaption     = regexp.MustCompile(`(?is)<figcaption[^>]*>([\s\S]*?)</figcaption>`)
	reHr             = regexp.MustCompile(`(?i)<hr\s*/?>`)
	reTable          = regexp.MustCompile(`(?is)<table[^>]*>([\s\S]*?)</table>`)
	reThead          = regexp.MustCompile(`(?is)<thead[^>]*>([\s\S]*?)</thead>`)
	reTr             = regexp.MustCompile(`(?is)<tr[^>]*>([\s\S]*?)</tr>`)
	reThTd           = regexp.MustCompile(`(?is)<(?:th|td)[^>]*>([\s\S]*?)</(?:th|td)>`)
	reBr             = regexp.MustCompile(`(?i)<br\s*/?>`)
	reTagStrip       = regexp.MustCompile(`<[^>]+>`)
	reMultiNewline   = regexp.MustCompile(`\n{3,}`)
	reMultiSpace     = regexp.MustCompile(`[ \t]{2,}`)
	reHTMLComment    = regexp.MustCompile(`(?s)<!--[\s\S]*?-->`)
	reHTMLEntAmp     = regexp.MustCompile(`&amp;`)
	reHTMLEntLt      = regexp.MustCompile(`&lt;`)
	reHTMLEntGt      = regexp.MustCompile(`&gt;`)
	reHTMLEntQuot    = regexp.MustCompile(`&quot;`)
	reHTMLEntApos    = regexp.MustCompile(`&#0?39;|&apos;`)
	reHTMLEntNbsp    = regexp.MustCompile(`&nbsp;`)
	reHTMLEntNumeric = regexp.MustCompile(`&#(\d+);`)
)

// extractTitle pulls the text content of the first <title> tag.
func extractTitle(html string) string {
	m := reClipTitle.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	title := reTagStrip.ReplaceAllString(m[1], "")
	title = decodeHTMLEntities(title)
	return strings.TrimSpace(title)
}

// extractOGTitle extracts the og:title meta tag as fallback title.
func extractOGTitle(html string) string {
	m := reOGTitle.FindStringSubmatch(html)
	if len(m) >= 2 {
		return decodeHTMLEntities(strings.TrimSpace(m[1]))
	}
	m = reOGTitleAlt.FindStringSubmatch(html)
	if len(m) >= 2 {
		return decodeHTMLEntities(strings.TrimSpace(m[1]))
	}
	return ""
}

// extractReaderContent tries to find <article> or <main> for focused content.
// Falls back to full HTML body if neither is found.
func extractReaderContent(html string) string {
	// Try <article> first (most blog/news sites)
	if m := reArticle.FindStringSubmatch(html); len(m) >= 2 {
		return m[1]
	}
	// Try <main>
	if m := reMain.FindStringSubmatch(html); len(m) >= 2 {
		return m[1]
	}
	// Fallback: use whole document
	return html
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(href, baseURL string) string {
	if baseURL == "" || href == "" {
		return href
	}
	// Already absolute
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	// Protocol-relative
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	// Data URIs, mailto, etc. — leave alone
	if strings.Contains(href, ":") {
		return href
	}
	// Absolute path
	if strings.HasPrefix(href, "/") {
		return baseURL + href
	}
	// Relative path
	return baseURL + "/" + href
}

// htmlToMarkdown performs a best-effort conversion of HTML to markdown.
func htmlToMarkdown(html string, baseURL string) string {
	// Remove comments
	html = reHTMLComment.ReplaceAllString(html, "")

	// Remove entire blocks we don't want
	html = reScript.ReplaceAllString(html, "")
	html = reStyle.ReplaceAllString(html, "")
	html = reNav.ReplaceAllString(html, "")
	html = reFooter.ReplaceAllString(html, "")
	html = reHeader.ReplaceAllString(html, "")
	html = reAside.ReplaceAllString(html, "")
	html = reForm.ReplaceAllString(html, "")
	html = reIframe.ReplaceAllString(html, "")

	// --- Pre/code blocks (before inline code, to avoid clobbering) ---
	html = rePreCode.ReplaceAllStringFunc(html, func(match string) string {
		m := rePreCode.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		lang := m[1]
		code := m[2]
		// Decode HTML entities in code blocks
		code = decodeHTMLEntities(code)
		// Strip any remaining tags inside code
		code = reTagStrip.ReplaceAllString(code, "")
		code = strings.TrimRight(code, "\n")
		return "\n\n```" + lang + "\n" + code + "\n```\n\n"
	})
	// Standalone <pre> without <code>
	html = rePre.ReplaceAllStringFunc(html, func(match string) string {
		m := rePre.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		code := decodeHTMLEntities(m[1])
		code = reTagStrip.ReplaceAllString(code, "")
		code = strings.TrimRight(code, "\n")
		return "\n\n```\n" + code + "\n```\n\n"
	})

	// --- Figures with captions ---
	html = reFigure.ReplaceAllStringFunc(html, func(match string) string {
		m := reFigure.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		inner := m[1]

		// Extract image
		imgMatch := reImg.FindString(inner)
		imgMd := ""
		if imgMatch != "" {
			src := ""
			alt := ""
			if sm := reImgSrc.FindStringSubmatch(imgMatch); len(sm) >= 2 {
				src = resolveURL(sm[1], baseURL)
			}
			if am := reImgAlt.FindStringSubmatch(imgMatch); len(am) >= 2 {
				alt = am[1]
			}
			imgMd = "![" + alt + "](" + src + ")"
		}

		// Extract caption
		caption := ""
		if cm := reFigCaption.FindStringSubmatch(inner); len(cm) >= 2 {
			caption = reTagStrip.ReplaceAllString(cm[1], "")
			caption = strings.TrimSpace(decodeHTMLEntities(caption))
		}

		result := "\n\n"
		if imgMd != "" {
			result += imgMd + "\n"
		}
		if caption != "" {
			result += "*" + caption + "*\n"
		}
		return result + "\n"
	})

	// --- Images ---
	html = reImg.ReplaceAllStringFunc(html, func(match string) string {
		src := ""
		alt := ""
		if sm := reImgSrc.FindStringSubmatch(match); len(sm) >= 2 {
			src = resolveURL(sm[1], baseURL)
		}
		if am := reImgAlt.FindStringSubmatch(match); len(am) >= 2 {
			alt = am[1]
		}
		if src == "" {
			return ""
		}
		return "![" + alt + "](" + src + ")"
	})

	// --- Tables ---
	html = reTable.ReplaceAllStringFunc(html, func(match string) string {
		return convertTable(match)
	})

	// --- Horizontal rules ---
	html = reHr.ReplaceAllString(html, "\n\n---\n\n")

	// --- Ordered lists with numbering ---
	html = convertOrderedLists(html)

	// --- Unordered lists with nesting ---
	html = convertUnorderedLists(html)

	// Convert inline elements first (inside tags that will be processed next)
	// Links: <a href="url">text</a> -> [text](url)
	html = reAnchor.ReplaceAllStringFunc(html, func(match string) string {
		m := reAnchor.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		href := resolveURL(m[1], baseURL)
		linkText := m[2]
		return "[" + linkText + "](" + href + ")"
	})

	// Bold: <strong>/<b> -> **text**
	html = reStrong.ReplaceAllString(html, "**$1**")

	// Italic: <em>/<i> -> *text*
	html = reEm.ReplaceAllString(html, "*$1*")

	// Strikethrough: <del>/<s> -> ~~text~~
	html = reStrikethrough.ReplaceAllString(html, "~~$1~~")

	// Superscript: <sup> -> ^text^
	html = reSup.ReplaceAllString(html, "^$1^")

	// Subscript: <sub> -> ~text~
	html = reSub.ReplaceAllString(html, "~$1~")

	// Inline code (only remaining after pre/code blocks were handled)
	html = reCode.ReplaceAllString(html, "`$1`")

	// Headings
	html = reH1.ReplaceAllString(html, "\n\n# $1\n\n")
	html = reH2.ReplaceAllString(html, "\n\n## $1\n\n")
	html = reH3.ReplaceAllString(html, "\n\n### $1\n\n")
	html = reH4.ReplaceAllString(html, "\n\n#### $1\n\n")
	html = reH5.ReplaceAllString(html, "\n\n##### $1\n\n")
	html = reH6.ReplaceAllString(html, "\n\n###### $1\n\n")

	// Paragraphs
	html = reParagraph.ReplaceAllString(html, "\n\n$1\n\n")

	// Remaining list items (not already handled by ol/ul conversion)
	html = clipReListItem.ReplaceAllString(html, "\n- $1")

	// Blockquotes
	html = clipReBlockquote.ReplaceAllStringFunc(html, func(match string) string {
		inner := clipReBlockquote.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		lines := strings.Split(strings.TrimSpace(inner[1]), "\n")
		var quoted []string
		for _, line := range lines {
			quoted = append(quoted, "> "+strings.TrimSpace(line))
		}
		return "\n\n" + strings.Join(quoted, "\n") + "\n\n"
	})

	// Line breaks
	html = reBr.ReplaceAllString(html, "\n")

	// Strip all remaining HTML tags
	html = reTagStrip.ReplaceAllString(html, "")

	// Decode HTML entities
	html = decodeHTMLEntities(html)

	// Clean up whitespace
	// Collapse multiple spaces on a line (but preserve newlines)
	lines := strings.Split(html, "\n")
	var cleaned []string
	for _, line := range lines {
		line = reMultiSpace.ReplaceAllString(line, " ")
		line = strings.TrimRight(line, " \t")
		cleaned = append(cleaned, line)
	}
	html = strings.Join(cleaned, "\n")

	// Collapse excessive blank lines
	html = reMultiNewline.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}

// convertTable converts an HTML table to a markdown table.
func convertTable(tableHTML string) string {
	// Extract all rows
	rows := reTr.FindAllStringSubmatch(tableHTML, -1)
	if len(rows) == 0 {
		return ""
	}

	var mdRows [][]string

	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		cells := reThTd.FindAllStringSubmatch(row[1], -1)
		var mdCells []string
		for _, cell := range cells {
			if len(cell) < 2 {
				continue
			}
			cellText := reTagStrip.ReplaceAllString(cell[1], "")
			cellText = decodeHTMLEntities(cellText)
			cellText = strings.TrimSpace(cellText)
			// Replace pipes in cell content to avoid breaking table
			cellText = strings.ReplaceAll(cellText, "|", "\\|")
			mdCells = append(mdCells, cellText)
		}
		if len(mdCells) > 0 {
			mdRows = append(mdRows, mdCells)
		}
	}

	if len(mdRows) == 0 {
		return ""
	}

	// Determine max columns
	maxCols := 0
	for _, row := range mdRows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Pad rows to same column count
	for i := range mdRows {
		for len(mdRows[i]) < maxCols {
			mdRows[i] = append(mdRows[i], "")
		}
	}

	// Determine if first row came from <thead>
	hasThead := reThead.MatchString(tableHTML)

	var sb strings.Builder
	sb.WriteString("\n\n")

	// Header row
	sb.WriteString("| " + strings.Join(mdRows[0], " | ") + " |\n")

	// Separator row
	sep := make([]string, maxCols)
	for i := range sep {
		sep[i] = "---"
	}
	sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")

	// Data rows — skip first row if it was a header
	startIdx := 1
	if !hasThead && len(mdRows) > 1 {
		// If no thead, first row is still treated as header (already written)
		startIdx = 1
	}
	for i := startIdx; i < len(mdRows); i++ {
		sb.WriteString("| " + strings.Join(mdRows[i], " | ") + " |\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// convertOrderedLists converts <ol> with <li> to numbered markdown lists.
// Handles nesting by tracking depth.
func convertOrderedLists(html string) string {
	// Process from innermost to outermost to handle nesting
	for i := 0; i < 5; i++ { // Max 5 nesting levels
		reOlFull := regexp.MustCompile(`(?is)<ol[^>]*>([\s\S]*?)</ol>`)
		if !reOlFull.MatchString(html) {
			break
		}
		html = reOlFull.ReplaceAllStringFunc(html, func(match string) string {
			m := reOlFull.FindStringSubmatch(match)
			if len(m) < 2 {
				return match
			}
			inner := m[1]
			items := clipReListItem.FindAllStringSubmatch(inner, -1)
			var result strings.Builder
			result.WriteString("\n")
			indent := strings.Repeat("  ", i)
			for idx, item := range items {
				if len(item) < 2 {
					continue
				}
				content := strings.TrimSpace(reTagStrip.ReplaceAllString(item[1], ""))
				result.WriteString(indent + strconv.Itoa(idx+1) + ". " + content + "\n")
			}
			return result.String()
		})
	}
	return html
}

// convertUnorderedLists converts <ul> with <li> to bullet-point markdown lists.
// Handles nesting by tracking depth.
func convertUnorderedLists(html string) string {
	for i := 0; i < 5; i++ { // Max 5 nesting levels
		reUlFull := regexp.MustCompile(`(?is)<ul[^>]*>([\s\S]*?)</ul>`)
		if !reUlFull.MatchString(html) {
			break
		}
		html = reUlFull.ReplaceAllStringFunc(html, func(match string) string {
			m := reUlFull.FindStringSubmatch(match)
			if len(m) < 2 {
				return match
			}
			inner := m[1]
			items := clipReListItem.FindAllStringSubmatch(inner, -1)
			var result strings.Builder
			result.WriteString("\n")
			indent := strings.Repeat("  ", i)
			for _, item := range items {
				if len(item) < 2 {
					continue
				}
				content := strings.TrimSpace(reTagStrip.ReplaceAllString(item[1], ""))
				result.WriteString(indent + "- " + content + "\n")
			}
			return result.String()
		})
	}
	return html
}

// decodeHTMLEntities converts common HTML entities to their text equivalents.
// Note: &amp; is decoded last to prevent double-decoding (e.g. &amp;lt; → &lt; → <).
func decodeHTMLEntities(s string) string {
	s = reHTMLEntLt.ReplaceAllString(s, "<")
	s = reHTMLEntGt.ReplaceAllString(s, ">")
	s = reHTMLEntQuot.ReplaceAllString(s, "\"")
	s = reHTMLEntApos.ReplaceAllString(s, "'")
	s = reHTMLEntNbsp.ReplaceAllString(s, " ")
	s = reHTMLEntNumeric.ReplaceAllStringFunc(s, func(match string) string {
		sub := reHTMLEntNumeric.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		var n int
		for _, c := range sub[1] {
			n = n*10 + int(c-'0')
		}
		if n > 0 && n < 0x10FFFF {
			return string(rune(n))
		}
		return match
	})
	// Decode &amp; last to avoid double-decoding
	s = reHTMLEntAmp.ReplaceAllString(s, "&")
	return s
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// truncate shortens a string to at most maxLen characters, adding ellipsis.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
