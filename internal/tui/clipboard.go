package tui

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Clipboard — system clipboard read/write via CLI tools
// ---------------------------------------------------------------------------

// Clipboard provides platform-agnostic system clipboard access.
type Clipboard struct{}

// ClipboardCopy copies the given text to the system clipboard.
// It tries platform-appropriate tools and falls back gracefully.
func ClipboardCopy(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try Wayland first, then X11 tools
		if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command(path)
		} else if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command(path, "-selection", "clipboard")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command(path, "--clipboard", "--input")
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
		return fmt.Errorf("clipboard: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("clipboard: %w", err)
	}
	return nil
}

// ClipboardPaste reads text from the system clipboard.
func ClipboardPaste() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if path, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command(path)
		} else if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command(path, "-selection", "clipboard", "-o")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command(path, "--clipboard", "--output")
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

// ClipboardAvailable checks whether a clipboard tool is installed.
func ClipboardAvailable() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("pbcopy")
		return err == nil
	case "linux":
		for _, tool := range []string{"wl-copy", "xclip", "xsel"} {
			if _, err := exec.LookPath(tool); err == nil {
				return true
			}
		}
		return false
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

// WebClipper is an overlay for configuring and previewing a web clip.
type WebClipper struct {
	active  bool
	width   int
	height  int

	url     string
	title   string
	content string
	loading bool
	loadingTick int
	done    bool

	// Result
	resultReady   bool
	resultTitle   string
	resultContent string

	// Internal UI state
	editingTitle bool
	titleBuf     string
	scrollOffset int
}

// NewWebClipper returns a zero-value WebClipper ready for use.
func NewWebClipper() WebClipper {
	return WebClipper{}
}

// IsActive reports whether the overlay is currently visible.
func (wc *WebClipper) IsActive() bool {
	return wc.active
}

// Open activates the web clipper and begins fetching the given URL.
func (wc *WebClipper) Open(url string) {
	wc.active = true
	wc.url = url
	wc.title = ""
	wc.content = ""
	wc.loading = true
	wc.loadingTick = 0
	wc.done = false
	wc.resultReady = false
	wc.resultTitle = ""
	wc.resultContent = ""
	wc.editingTitle = false
	wc.titleBuf = ""
	wc.scrollOffset = 0
}

// Close deactivates the overlay and resets state.
func (wc *WebClipper) Close() {
	wc.active = false
	wc.loading = false
	wc.done = false
	wc.resultReady = false
	wc.editingTitle = false
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
			return wc, nil
		}
		wc.title = msg.title
		wc.content = msg.content
		wc.url = msg.url
		wc.done = true
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
		// Editing title mode
		if wc.editingTitle {
			return wc.updateEditTitle(msg)
		}

		// Normal navigation
		switch msg.String() {
		case "esc":
			wc.active = false
			return wc, nil

		case "enter":
			if wc.done && wc.title != "" {
				// Build final markdown with frontmatter
				now := time.Now().Format("2006-01-02")
				var sb strings.Builder
				sb.WriteString("---\n")
				sb.WriteString("source: " + wc.url + "\n")
				sb.WriteString("clipped: " + now + "\n")
				sb.WriteString("tags: [clipped]\n")
				sb.WriteString("---\n\n")
				sb.WriteString("# " + wc.title + "\n\n")
				sb.WriteString(wc.content)

				wc.resultReady = true
				wc.resultTitle = wc.title
				wc.resultContent = sb.String()
				wc.active = false
				return wc, nil
			}

		case "e":
			if wc.done {
				wc.editingTitle = true
				wc.titleBuf = wc.title
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
			wc.titleBuf = wc.titleBuf[:len(wc.titleBuf)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			wc.titleBuf += ch
		}
	}
	return wc, nil
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
		if !wc.editingTitle {
			helpKeys := lipgloss.NewStyle().Foreground(surface0).Background(overlay0).Padding(0, 1)
			helpDesc := DimStyle

			b.WriteString("  ")
			b.WriteString(helpKeys.Render("Enter") + helpDesc.Render(" save") + "  ")
			b.WriteString(helpKeys.Render("e") + helpDesc.Render(" edit title") + "  ")
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
func fetchAndClip(url string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return webClipResult{url: url, err: fmt.Errorf("invalid URL: %w", err)}
		}
		req.Header.Set("User-Agent", "Granit/1.0 WebClipper")

		resp, err := client.Do(req)
		if err != nil {
			return webClipResult{url: url, err: fmt.Errorf("fetch failed: %w", err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return webClipResult{url: url, err: fmt.Errorf("read failed: %w", err)}
		}

		html := string(body)

		// Extract <title>
		title := extractTitle(html)
		if title == "" {
			title = "Untitled"
		}

		// Convert HTML to markdown-like text
		content := htmlToMarkdown(html)

		return webClipResult{
			title:   title,
			content: content,
			url:     url,
		}
	}
}

// ---------------------------------------------------------------------------
// HTML → Markdown extraction (regex-based, no external parser)
// ---------------------------------------------------------------------------

var (
	reClipTitle      = regexp.MustCompile(`(?i)<title[^>]*>([\s\S]*?)</title>`)
	reScript         = regexp.MustCompile(`(?is)<script[\s>][\s\S]*?</script>`)
	reStyle          = regexp.MustCompile(`(?is)<style[\s>][\s\S]*?</style>`)
	reNav            = regexp.MustCompile(`(?is)<nav[\s>][\s\S]*?</nav>`)
	reFooter         = regexp.MustCompile(`(?is)<footer[\s>][\s\S]*?</footer>`)
	reHeader         = regexp.MustCompile(`(?is)<header[\s>][\s\S]*?</header>`)
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
	clipReListItem       = regexp.MustCompile(`(?i)<li[^>]*>([\s\S]*?)</li>`)
	clipReBlockquote     = regexp.MustCompile(`(?i)<blockquote[^>]*>([\s\S]*?)</blockquote>`)
	reCode           = regexp.MustCompile(`(?i)<code[^>]*>([\s\S]*?)</code>`)
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

// htmlToMarkdown performs a best-effort conversion of HTML to markdown.
func htmlToMarkdown(html string) string {
	// Remove comments
	html = reHTMLComment.ReplaceAllString(html, "")

	// Remove entire blocks we don't want
	html = reScript.ReplaceAllString(html, "")
	html = reStyle.ReplaceAllString(html, "")
	html = reNav.ReplaceAllString(html, "")
	html = reFooter.ReplaceAllString(html, "")
	html = reHeader.ReplaceAllString(html, "")

	// Convert inline elements first (inside tags that will be processed next)
	// Links: <a href="url">text</a> → [text](url)
	html = reAnchor.ReplaceAllString(html, "[$2]($1)")

	// Bold: <strong>/<b> → **text**
	html = reStrong.ReplaceAllString(html, "**$1**")

	// Italic: <em>/<i> → *text*
	html = reEm.ReplaceAllString(html, "*$1*")

	// Inline code
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

	// List items
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

// decodeHTMLEntities converts common HTML entities to their text equivalents.
func decodeHTMLEntities(s string) string {
	s = reHTMLEntAmp.ReplaceAllString(s, "&")
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
