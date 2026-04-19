package tui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var blogHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// blogPublishResultMsg carries the outcome of an async blog publish operation.
type blogPublishResultMsg struct {
	url string
	err error
}

// ---------------------------------------------------------------------------
// State machine
// ---------------------------------------------------------------------------

type blogScreen int

const (
	blogScreenTarget     blogScreen = iota // choose Medium or GitHub
	blogScreenConfig                       // edit config fields
	blogScreenPublishing                   // in progress
	blogScreenResult                       // done
)

// ---------------------------------------------------------------------------
// BlogPublisher overlay
// ---------------------------------------------------------------------------

// BlogPublisher is an overlay for publishing notes to Medium or GitHub Blog.
type BlogPublisher struct {
	OverlayBase

	noteTitle   string
	noteContent string

	// Target selection
	target string // "medium" or "github"
	screen blogScreen
	cursor int

	// Inline editing
	editing   bool
	editBuf   string
	editField int

	// Medium config
	mediumToken   string
	publishStatus string // "draft", "public", "unlisted"

	// GitHub config
	ghToken  string
	ghRepo   string // "owner/repo"
	ghBranch string
	ghPath   string // e.g. "content/posts"

	// Result state
	status string
	done   bool
	err    error

	// Config persistence callback — saves tokens back to config
	configSave func(target, mediumToken, ghToken, ghRepo, ghBranch string)
}

// NewBlogPublisher creates a new BlogPublisher overlay with default settings.
func NewBlogPublisher() BlogPublisher {
	return BlogPublisher{
		publishStatus: "draft",
		ghBranch:      "main",
		ghPath:        "content/posts",
	}
}

// SetConfigSave sets the callback invoked after a successful publish to
// persist tokens and repo settings back to the user config.
func (bp *BlogPublisher) SetConfigSave(fn func(target, mediumToken, ghToken, ghRepo, ghBranch string)) {
	bp.configSave = fn
}

// PreFill loads previously-saved tokens into the publisher so the user does
// not have to re-enter them every session.
func (bp *BlogPublisher) PreFill(mediumToken, ghToken, ghRepo, ghBranch string) {
	if mediumToken != "" {
		bp.mediumToken = mediumToken
	}
	if ghToken != "" {
		bp.ghToken = ghToken
	}
	if ghRepo != "" {
		bp.ghRepo = ghRepo
	}
	if ghBranch != "" {
		bp.ghBranch = ghBranch
	}
}

func (bp *BlogPublisher) Open(noteTitle, noteContent string) {
	bp.Activate()
	bp.noteTitle = noteTitle
	bp.noteContent = noteContent
	bp.screen = blogScreenTarget
	bp.cursor = 0
	bp.editing = false
	bp.editBuf = ""
	bp.editField = 0
	bp.status = ""
	bp.done = false
	bp.err = nil
}

// Close hides the overlay and exits the inline text-edit mode so the next
// Open starts in navigation, not mid-field.
func (bp *BlogPublisher) Close() {
	bp.OverlayBase.Close()
	bp.editing = false
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (bp BlogPublisher) Update(msg tea.Msg) (BlogPublisher, tea.Cmd) {
	if !bp.active {
		return bp, nil
	}

	switch msg := msg.(type) {
	case blogPublishResultMsg:
		bp.screen = blogScreenResult
		bp.done = true
		bp.err = msg.err
		if msg.err != nil {
			bp.status = "Error: " + msg.err.Error()
		} else {
			bp.status = msg.url
			// Persist tokens to config on successful publish
			if bp.configSave != nil {
				bp.configSave(bp.target, bp.mediumToken, bp.ghToken, bp.ghRepo, bp.ghBranch)
			}
		}
		return bp, nil

	case tea.KeyMsg:
		switch bp.screen {
		case blogScreenTarget:
			return bp.updateTarget(msg)
		case blogScreenConfig:
			if bp.editing {
				return bp.updateEditing(msg)
			}
			return bp.updateConfig(msg)
		case blogScreenPublishing:
			// Ignore keys while publishing
			return bp, nil
		case blogScreenResult:
			return bp.updateResult(msg)
		}
	}
	return bp, nil
}

func (bp BlogPublisher) updateTarget(msg tea.KeyMsg) (BlogPublisher, tea.Cmd) {
	switch msg.String() {
	case "esc":
		bp.active = false
	case "up", "k":
		if bp.cursor > 0 {
			bp.cursor--
		}
	case "down", "j":
		if bp.cursor < 1 {
			bp.cursor++
		}
	case "enter":
		if bp.cursor == 0 {
			bp.target = "medium"
		} else {
			bp.target = "github"
		}
		bp.screen = blogScreenConfig
		bp.cursor = 0
		bp.editing = false
	}
	return bp, nil
}

func (bp BlogPublisher) updateConfig(msg tea.KeyMsg) (BlogPublisher, tea.Cmd) {
	maxRow := bp.configTotalRows() - 1

	switch msg.String() {
	case "esc":
		// Go back to target selection
		bp.screen = blogScreenTarget
		bp.cursor = 0
	case "up", "k":
		if bp.cursor > 0 {
			bp.cursor--
		}
	case "down", "j":
		if bp.cursor < maxRow {
			bp.cursor++
		}
	case "tab":
		// Cycle publish status on the status field (Medium only)
		if bp.target == "medium" && bp.cursor == 1 {
			switch bp.publishStatus {
			case "draft":
				bp.publishStatus = "public"
			case "public":
				bp.publishStatus = "unlisted"
			default:
				bp.publishStatus = "draft"
			}
		}
	case "enter":
		publishRow := bp.configPublishRow()
		if bp.cursor == publishRow {
			// Validate required fields before publishing
			if errMsg := bp.validateConfig(); errMsg != "" {
				bp.screen = blogScreenResult
				bp.done = true
				bp.err = fmt.Errorf("%s", errMsg)
				bp.status = "Error: " + errMsg
				return bp, nil
			}
			// Start publishing
			bp.screen = blogScreenPublishing
			bp.status = "Publishing..."
			return bp, bp.startPublish()
		}
		// For the publish status field in Medium, cycle with enter too
		if bp.target == "medium" && bp.cursor == 1 {
			switch bp.publishStatus {
			case "draft":
				bp.publishStatus = "public"
			case "public":
				bp.publishStatus = "unlisted"
			default:
				bp.publishStatus = "draft"
			}
			return bp, nil
		}
		// Start editing a text field
		bp.editing = true
		bp.editField = bp.cursor
		bp.editBuf = bp.configFieldValue(bp.cursor)
	}
	return bp, nil
}

func (bp BlogPublisher) updateEditing(msg tea.KeyMsg) (BlogPublisher, tea.Cmd) {
	switch msg.String() {
	case "esc":
		bp.editing = false
		bp.editBuf = ""
	case "enter":
		bp.commitEdit()
		bp.editing = false
		bp.editBuf = ""
	case "backspace":
		if len(bp.editBuf) > 0 {
			bp.editBuf = TrimLastRune(bp.editBuf)
		}
	case "ctrl+u":
		bp.editBuf = ""
	default:
		for _, r := range msg.Runes {
			bp.editBuf += string(r)
		}
	}
	return bp, nil
}

func (bp *BlogPublisher) commitEdit() {
	val := strings.TrimSpace(bp.editBuf)
	if bp.target == "medium" {
		switch bp.editField {
		case 0:
			bp.mediumToken = val
		}
	} else {
		switch bp.editField {
		case 0:
			bp.ghToken = val
		case 1:
			bp.ghRepo = val
		case 2:
			bp.ghBranch = val
			if bp.ghBranch == "" {
				bp.ghBranch = "main"
			}
		case 3:
			bp.ghPath = val
		}
	}
}

func (bp BlogPublisher) updateResult(msg tea.KeyMsg) (BlogPublisher, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		bp.active = false
		bp.done = false
		bp.status = ""
	}
	return bp, nil
}

// ---------------------------------------------------------------------------
// Config field helpers
// ---------------------------------------------------------------------------

type blogConfigField struct {
	label string
	value string
}

func (bp *BlogPublisher) configFields() []blogConfigField {
	if bp.target == "medium" {
		return []blogConfigField{
			{"API Token    ", bp.mediumToken},
			{"Status       ", bp.publishStatus},
		}
	}
	return []blogConfigField{
		{"API Token    ", bp.ghToken},
		{"Repository   ", bp.ghRepo},
		{"Branch       ", bp.ghBranch},
		{"Path         ", bp.ghPath},
	}
}

func (bp *BlogPublisher) configFieldValue(idx int) string {
	fields := bp.configFields()
	if idx >= 0 && idx < len(fields) {
		return fields[idx].value
	}
	return ""
}

func (bp *BlogPublisher) configTotalRows() int {
	return len(bp.configFields()) + 1 // fields + publish button
}

func (bp *BlogPublisher) configPublishRow() int {
	return len(bp.configFields())
}

// validateConfig checks that required fields are filled in before publishing.
// Returns an error message string, or "" if everything looks good.
func (bp *BlogPublisher) validateConfig() string {
	if bp.target == "medium" {
		if strings.TrimSpace(bp.mediumToken) == "" {
			return "Medium API token is required"
		}
	} else {
		if strings.TrimSpace(bp.ghToken) == "" {
			return "GitHub API token is required"
		}
		if strings.TrimSpace(bp.ghRepo) == "" {
			return "GitHub repository (owner/repo) is required"
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Publish commands
// ---------------------------------------------------------------------------

func (bp *BlogPublisher) startPublish() tea.Cmd {
	target := bp.target
	title := bp.noteTitle
	content := bp.noteContent
	tags := bp.extractTags()

	if target == "medium" {
		token := bp.mediumToken
		status := bp.publishStatus
		return func() tea.Msg {
			url, err := blogPublishMedium(token, title, content, status, tags)
			return blogPublishResultMsg{url: url, err: err}
		}
	}

	// GitHub
	ghToken := bp.ghToken
	ghRepo := bp.ghRepo
	ghBranch := bp.ghBranch
	ghPath := bp.ghPath
	return func() tea.Msg {
		url, err := blogPublishGitHub(ghToken, ghRepo, ghBranch, ghPath, title, content)
		return blogPublishResultMsg{url: url, err: err}
	}
}

// extractTags parses YAML frontmatter from the note content to get tags.
func (bp *BlogPublisher) extractTags() []string {
	content := bp.noteContent
	if !strings.HasPrefix(content, "---") {
		return nil
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return nil
	}
	block := content[3 : 3+end]
	lines := strings.Split(strings.TrimSpace(block), "\n")

	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key != "tags" {
			continue
		}
		// Handle [tag1, tag2] format
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			inner := value[1 : len(value)-1]
			items := strings.Split(inner, ",")
			var tags []string
			for _, item := range items {
				t := strings.TrimSpace(item)
				if t != "" {
					tags = append(tags, t)
				}
			}
			return tags
		}
		// Handle single-value or comma-separated
		if value != "" {
			items := strings.Split(value, ",")
			var tags []string
			for _, item := range items {
				t := strings.TrimSpace(item)
				if t != "" {
					tags = append(tags, t)
				}
			}
			return tags
		}
	}
	return nil
}

// extractAPIErrorMessage tries to pull a human-readable message from a JSON
// API error body.  Falls back to "HTTP <code>" when the body cannot be parsed.
func extractAPIErrorMessage(body []byte, statusCode int) string {
	var result struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &result) == nil && result.Message != "" {
		return result.Message
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}

// ---------------------------------------------------------------------------
// HTTP retry with exponential backoff
// ---------------------------------------------------------------------------

func blogHTTPDoWithRetry(req *http.Request, maxRetries int) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
			// Reset request body if needed
			if req.GetBody != nil {
				req.Body, _ = req.GetBody()
			}
		}
		resp, err = blogHTTPClient.Do(req)
		if err != nil {
			continue // network error, retry
		}
		// Don't retry on client errors (4xx), only on server errors (5xx) or rate limits (429)
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			return resp, nil
		}
		resp.Body.Close() // close before retry
	}
	return resp, err
}

// ---------------------------------------------------------------------------
// Medium API
// ---------------------------------------------------------------------------

func blogPublishMedium(token, title, content, publishStatus string, tags []string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("medium API token is required")
	}

	// Step 1: Get user ID
	req, err := http.NewRequest("GET", "https://api.medium.com/v1/me", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := blogHTTPDoWithRetry(req, 2)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read user response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("medium API error: %s", extractAPIErrorMessage(body, resp.StatusCode))
	}

	var userResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &userResp); err != nil {
		return "", fmt.Errorf("parse user response: %w", err)
	}
	if userResp.Data.ID == "" {
		return "", fmt.Errorf("could not get Medium user ID")
	}

	// Step 2: Create post
	postBody := map[string]interface{}{
		"title":         title,
		"contentFormat": "markdown",
		"content":       content,
		"publishStatus": publishStatus,
	}
	if len(tags) > 0 {
		// Medium allows max 5 tags
		if len(tags) > 5 {
			tags = tags[:5]
		}
		postBody["tags"] = tags
	}

	payload, err := json.Marshal(postBody)
	if err != nil {
		return "", fmt.Errorf("marshal post: %w", err)
	}

	postURL := fmt.Sprintf("https://api.medium.com/v1/users/%s/posts", userResp.Data.ID)
	req, err = http.NewRequest("POST", postURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create post request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err = blogHTTPDoWithRetry(req, 2)
	if err != nil {
		return "", fmt.Errorf("create post: %w", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read post response: %w", err)
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("medium API error: %s", extractAPIErrorMessage(body, resp.StatusCode))
	}

	var postResp struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", fmt.Errorf("parse post response: %w", err)
	}

	return postResp.Data.URL, nil
}

// ---------------------------------------------------------------------------
// GitHub API
// ---------------------------------------------------------------------------

var reBlogSanitize = regexp.MustCompile(`[^a-z0-9\-]`)

func blogSanitizeTitle(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	s = reBlogSanitize.ReplaceAllString(s, "")
	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

func blogPublishGitHub(token, repo, branch, dirPath, title, content string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("GitHub API token is required")
	}
	if repo == "" {
		return "", fmt.Errorf("GitHub repository (owner/repo) is required")
	}

	// Build the file path
	fileName := blogSanitizeTitle(title) + ".md"
	filePath := fileName
	if dirPath != "" {
		filePath = strings.TrimSuffix(dirPath, "/") + "/" + fileName
	}

	// Step 1: Check if file already exists (to get SHA for update)
	var existingSHA string
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", repo, filePath, branch)
	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return "", fmt.Errorf("create GET request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := blogHTTPDoWithRetry(req, 2)
	if err != nil {
		return "", fmt.Errorf("check existing file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var fileResp struct {
			SHA string `json:"sha"`
		}
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("read file-check response: %w", readErr)
		}
		if err := json.Unmarshal(body, &fileResp); err != nil {
			return "", fmt.Errorf("parse file-check response: %w", err)
		}
		existingSHA = fileResp.SHA
	} else if resp.StatusCode != 404 {
		// 404 is expected for new files; anything else is unexpected
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("check existing file: %s", extractAPIErrorMessage(body, resp.StatusCode))
	} else {
		// Drain the body for 404
		_, _ = io.ReadAll(resp.Body)
	}

	// Step 2: Create or update the file
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	putBody := map[string]interface{}{
		"message": fmt.Sprintf("Add post: %s", title),
		"content": encoded,
		"branch":  branch,
	}
	if existingSHA != "" {
		putBody["message"] = fmt.Sprintf("Update post: %s", title)
		putBody["sha"] = existingSHA
	}

	payload, err := json.Marshal(putBody)
	if err != nil {
		return "", fmt.Errorf("marshal PUT body: %w", err)
	}

	putURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, filePath)
	req, err = http.NewRequest("PUT", putURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create PUT request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err = blogHTTPDoWithRetry(req, 2)
	if err != nil {
		return "", fmt.Errorf("push to GitHub: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read GitHub response: %w", err)
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("GitHub API error: %s", extractAPIErrorMessage(body, resp.StatusCode))
	}

	var putResp struct {
		Content struct {
			HTMLURL string `json:"html_url"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &putResp); err != nil {
		return "", fmt.Errorf("parse GitHub response: %w", err)
	}

	return putResp.Content.HTMLURL, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (bp BlogPublisher) View() string {
	width := bp.width / 2
	if width < 56 {
		width = 56
	}
	if width > 74 {
		width = 74
	}
	innerW := width - 6

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Blog Publisher")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n\n")

	switch bp.screen {
	case blogScreenTarget:
		bp.viewTarget(&b, innerW)
	case blogScreenConfig:
		bp.viewConfig(&b, innerW)
	case blogScreenPublishing:
		bp.viewPublishing(&b, innerW)
	case blogScreenResult:
		bp.viewResult(&b, innerW)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (bp *BlogPublisher) viewTarget(b *strings.Builder, innerW int) {
	// Note info
	noteLabel := lipgloss.NewStyle().Foreground(text).Render("  Note: ")
	noteName := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(bp.noteTitle)
	b.WriteString(noteLabel + noteName)
	b.WriteString("\n\n")

	selLabel := lipgloss.NewStyle().Foreground(text).Render("  Choose target:")
	b.WriteString(selLabel)
	b.WriteString("\n\n")

	type targetItem struct {
		icon string
		name string
		desc string
	}
	targets := []targetItem{
		{IconLinkChar, "Medium", "Publish to Medium via REST API"},
		{IconGraphChar, "GitHub Blog", "Push markdown to a GitHub repository"},
	}

	for i, t := range targets {
		icon := lipgloss.NewStyle().Foreground(blue).Render(t.icon) + " "

		if i == bp.cursor {
			accent := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(ThemeAccentBar + " ")
			line := accent + icon + lipgloss.NewStyle().
				Foreground(peach).
				Bold(true).
				Render(t.name)
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
			b.WriteString("\n")
			desc := DimStyle.Render("    " + t.desc)
			b.WriteString(lipgloss.NewStyle().Background(surface0).Width(innerW).Render(desc))
		} else {
			b.WriteString("  " + icon + NormalItemStyle.Render(t.name))
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("    " + t.desc))
		}
		if i < len(targets)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  j/k: navigate  Enter: select  Esc: close"))
}

func (bp *BlogPublisher) viewConfig(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(text)
	valueStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	editingStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	// Show which target
	targetName := "Medium"
	if bp.target == "github" {
		targetName = "GitHub Blog"
	}
	targetLabel := lipgloss.NewStyle().Foreground(text).Render("  Target: ")
	targetValue := lipgloss.NewStyle().Foreground(green).Bold(true).Render(targetName)
	b.WriteString(targetLabel + targetValue)
	b.WriteString("\n")

	// Note title
	noteLabel := lipgloss.NewStyle().Foreground(text).Render("  Note:   ")
	noteName := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(bp.noteTitle)
	b.WriteString(noteLabel + noteName)
	b.WriteString("\n\n")

	fields := bp.configFields()

	for i, f := range fields {
		prefix := "  "
		if i == bp.cursor {
			prefix = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(ThemeAccentBar) + " "
		}

		displayVal := f.value
		if displayVal == "" {
			displayVal = "(none)"
		}

		isTokenField := (bp.target == "medium" && i == 0) || (bp.target == "github" && i == 0)
		if isTokenField && f.value != "" && !bp.editing {
			if len(f.value) > 4 {
				displayVal = "••••••" + f.value[len(f.value)-4:]
			} else {
				displayVal = "••••••"
			}
		}

		// Status field for Medium — show as toggleable
		isStatusField := bp.target == "medium" && i == 1

		if bp.editing && bp.cursor == i && !isStatusField {
			cursor := lipgloss.NewStyle().Foreground(peach).Render("\u2588")
			editDisplay := bp.editBuf
			if isTokenField && len(editDisplay) > 0 {
				editDisplay = strings.Repeat("•", len(editDisplay))
			}
			line := prefix + labelStyle.Render(f.label) + editingStyle.Render(editDisplay) + cursor
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else if i == bp.cursor {
			if isStatusField {
				statusStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
				line := prefix + labelStyle.Render(f.label) + statusStyle.Render(displayVal) +
					DimStyle.Render("  [Enter/Tab to cycle]")
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerW).
					Render(line))
			} else {
				line := prefix + labelStyle.Render(f.label) + valueStyle.Render(displayVal)
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerW).
					Render(line))
			}
		} else {
			if isStatusField {
				statusColor := overlay0
				switch f.value {
				case "public":
					statusColor = green
				case "draft":
					statusColor = yellow
				case "unlisted":
					statusColor = peach
				}
				b.WriteString(prefix + labelStyle.Render(f.label) +
					lipgloss.NewStyle().Foreground(statusColor).Render(displayVal))
			} else {
				b.WriteString(prefix + labelStyle.Render(f.label) + DimStyle.Render(displayVal))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Publish button
	publishRow := bp.configPublishRow()
	if bp.cursor == publishRow {
		btnStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(green).
			Bold(true).
			Padding(0, 2)
		b.WriteString("  " + btnStyle.Render("  Publish  "))
	} else {
		btnStyle := lipgloss.NewStyle().
			Foreground(green).
			Bold(true)
		b.WriteString("  " + btnStyle.Render("[ Publish ]"))
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	if bp.editing {
		b.WriteString(DimStyle.Render("  Enter: confirm  Esc: cancel  Ctrl+U: clear"))
	} else {
		helpText := "  j/k: navigate  Enter: edit/publish  Esc: back"
		if bp.target == "medium" {
			helpText = "  j/k: navigate  Enter: edit/publish  Tab: cycle status  Esc: back"
		}
		b.WriteString(DimStyle.Render(helpText))
	}
}

func (bp *BlogPublisher) viewPublishing(b *strings.Builder, innerW int) {
	spinStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
	targetName := "Medium"
	if bp.target == "github" {
		targetName = "GitHub"
	}
	b.WriteString(spinStyle.Render(fmt.Sprintf("  Publishing to %s...", targetName)))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Please wait"))
}

func (bp *BlogPublisher) viewResult(b *strings.Builder, innerW int) {
	if bp.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString(errStyle.Render("  Publication failed"))
		b.WriteString("\n\n")
		// Wrap long error messages
		errMsg := TruncateDisplay(bp.status, innerW-4)
		b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + errMsg))
	} else {
		okStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(okStyle.Render("  Published successfully!"))
		b.WriteString("\n\n")
		urlLabel := lipgloss.NewStyle().Foreground(text).Render("  URL: ")
		urlValue := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(bp.status)
		b.WriteString(urlLabel + urlValue)
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Esc/Enter: close"))
}
