package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Matrix Client-Server API types
// ---------------------------------------------------------------------------

type matrixLoginRequest struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type matrixLoginResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	Error       string `json:"error,omitempty"`
	ErrCode     string `json:"errcode,omitempty"`
}

type matrixSyncResponse struct {
	NextBatch string                       `json:"next_batch"`
	Rooms     matrixSyncRooms              `json:"rooms"`
	Error     string                       `json:"error,omitempty"`
	ErrCode   string                       `json:"errcode,omitempty"`
}

type matrixSyncRooms struct {
	Join map[string]matrixJoinedRoom `json:"join"`
}

type matrixJoinedRoom struct {
	Timeline matrixTimeline `json:"timeline"`
	UnreadNotifications struct {
		NotificationCount int `json:"notification_count"`
	} `json:"unread_notifications"`
}

type matrixTimeline struct {
	Events []matrixEvent `json:"events"`
}

type matrixEvent struct {
	Type           string                 `json:"type"`
	Content        map[string]interface{} `json:"content"`
	Sender         string                 `json:"sender"`
	EventID        string                 `json:"event_id"`
	OriginServerTS int64                  `json:"origin_server_ts"`
}

type matrixRoomNameResponse struct {
	Name    string `json:"name"`
	Error   string `json:"error,omitempty"`
	ErrCode string `json:"errcode,omitempty"`
}

type matrixJoinedRoomsResponse struct {
	JoinedRooms []string `json:"joined_rooms"`
	Error       string   `json:"error,omitempty"`
	ErrCode     string   `json:"errcode,omitempty"`
}

type matrixMessagesResponse struct {
	Chunk []matrixEvent `json:"chunk"`
	End   string        `json:"end"`
	Error string        `json:"error,omitempty"`
}

type matrixMembersResponse struct {
	Chunk []matrixEvent `json:"chunk"`
	Error string        `json:"error,omitempty"`
}

type matrixSendResponse struct {
	EventID string `json:"event_id"`
	Error   string `json:"error,omitempty"`
	ErrCode string `json:"errcode,omitempty"`
}

type matrixErrorResponse struct {
	ErrCode string `json:"errcode"`
	Error   string `json:"error"`
}

// ---------------------------------------------------------------------------
// Tea messages for async operations
// ---------------------------------------------------------------------------

type matrixLoginResultMsg struct {
	token  string
	userID string
	err    error
}

type matrixSyncResultMsg struct {
	resp *matrixSyncResponse
	err  error
}

type matrixRoomsResultMsg struct {
	rooms []matrixRoom
	err   error
}

type matrixMessagesResultMsg struct {
	roomID   string
	messages []matrixChatMessage
	err      error
}

type matrixSendResultMsg struct {
	roomID  string
	eventID string
	err     error
}

type matrixReactionResultMsg struct {
	err error
}

type matrixSyncTickMsg struct{}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------

type matrixRoom struct {
	ID          string
	Name        string
	UnreadCount int
	LastMessage string
	LastTime    time.Time
	Members     []string
	Encrypted   bool
	Pinned      bool
}

type matrixChatMessage struct {
	Sender    string
	Body      string
	Timestamp time.Time
	EventID   string
	IsOwn     bool
	ReplyTo   string // event ID this is a reply to
	Reactions map[string]int // emoji -> count
}

type matrixFocusArea int

const (
	matrixFocusRooms matrixFocusArea = iota
	matrixFocusMessages
	matrixFocusInput
	matrixFocusLogin
	matrixFocusPrivacy
	matrixFocusNewDM
)

type matrixConnectionState int

const (
	matrixDisconnected matrixConnectionState = iota
	matrixConnecting
	matrixConnected
	matrixSyncing
)

// Available reaction emojis
var matrixReactionEmojis = []string{"\U0001F44D", "\u2764\uFE0F", "\U0001F602", "\U0001F389", "\U0001F914", "\U0001F440"}

// ---------------------------------------------------------------------------
// Matrix overlay component
// ---------------------------------------------------------------------------

type Matrix struct {
	active bool
	width  int
	height int

	// Connection
	homeserver  string
	username    string
	accessToken string
	userID      string
	connState   matrixConnectionState
	syncToken   string
	statusMsg   string

	// Login form
	loginFocus    int // 0=homeserver, 1=username, 2=password
	loginServer   string
	loginUser     string
	loginPass     string
	loginError    string

	// Rooms
	rooms       []matrixRoom
	roomCursor  int
	roomScroll  int
	roomSearch  string
	searching   bool
	pinnedRooms []string // room IDs that are pinned

	// Messages
	messages       map[string][]matrixChatMessage // roomID -> messages
	messageScroll  int
	messageInput   string
	messageCursor  int // selected message index for reactions/replies
	messageSelect  bool // whether a message is selected

	// Message search
	msgSearchMode   bool
	msgSearchQuery  string
	msgSearchIdx    int // current match index
	msgSearchMatches []int // indices of matching messages

	// Reply context
	replyToEvent string // event ID being replied to
	replyPreview string // preview text of the message being replied to

	// Reaction picker
	showReactionPicker bool
	reactionCursor     int

	// Focus
	focus matrixFocusArea

	// Privacy controls
	sendReadReceipts   bool
	sendTypingIndicators bool
	autoDeleteCache    bool
	showPrivacy        bool

	// New DM
	newDMInput string

	// Share note
	shareContent string

	// Sync
	syncRunning bool
	txnCounter  int

	// E2EE status cache per room
	e2eeStatus map[string]bool

	// Vault integration callbacks and data
	captureNote func(filename, content string) // callback to create a note in the vault
	vaultFiles  []string                       // list of vault file paths
	vaultRoot   string                         // vault root directory

	// Slash command results displayed inline
	slashResults []string

	// AI features
	ai                 MatrixAI
	currentNoteContent string
}

func NewMatrix() Matrix {
	return Matrix{
		messages:        make(map[string][]matrixChatMessage),
		e2eeStatus:      make(map[string]bool),
		connState:       matrixDisconnected,
		focus:           matrixFocusLogin,
		autoDeleteCache: true,
		ai:              NewMatrixAI(),
	}
}

func (mx *Matrix) ConfigureAI(provider, ollamaModel, ollamaURL, openaiKey, openaiModel string) {
	mx.ai.SetAIConfig(provider, ollamaModel, ollamaURL, openaiKey, openaiModel)
}

func (mx *Matrix) SetCurrentNoteContent(content string) {
	mx.currentNoteContent = content
}

func (mx *Matrix) SetSize(width, height int) {
	mx.width = width
	mx.height = height
}

func (mx *Matrix) IsActive() bool { return mx.active }

func (mx *Matrix) Open() {
	mx.active = true
	mx.statusMsg = ""
	if mx.accessToken != "" {
		mx.focus = matrixFocusRooms
		mx.connState = matrixConnected
	} else {
		mx.focus = matrixFocusLogin
	}
}

func (mx *Matrix) Close() {
	mx.active = false
	if mx.autoDeleteCache {
		mx.messages = make(map[string][]matrixChatMessage)
	}
}

func (mx *Matrix) Configure(homeserver, username, token string, readReceipts, typingIndicators, autoDelete bool) {
	mx.homeserver = strings.TrimRight(homeserver, "/")
	mx.username = username
	mx.accessToken = token
	mx.sendReadReceipts = readReceipts
	mx.sendTypingIndicators = typingIndicators
	mx.autoDeleteCache = autoDelete
	if token != "" {
		mx.connState = matrixConnected
		mx.focus = matrixFocusRooms
	}
}

func (mx *Matrix) GetAccessToken() string {
	return mx.accessToken
}

// SetShareContent sets content to be shared when user presses 's'
func (mx *Matrix) SetShareContent(content string) {
	mx.shareContent = content
}

// SetCaptureNote sets the callback for creating notes in the vault
func (mx *Matrix) SetCaptureNote(fn func(filename, content string)) {
	mx.captureNote = fn
}

// SetVaultFiles provides vault file list for slash commands
func (mx *Matrix) SetVaultFiles(files []string) {
	mx.vaultFiles = files
}

// SetVaultRoot provides vault root directory for slash commands
func (mx *Matrix) SetVaultRoot(root string) {
	mx.vaultRoot = root
}

// SetPinnedRooms loads pinned room IDs from config
func (mx *Matrix) SetPinnedRooms(pinned []string) {
	mx.pinnedRooms = pinned
	// Apply pinned status to rooms
	for i := range mx.rooms {
		mx.rooms[i].Pinned = false
		for _, pid := range mx.pinnedRooms {
			if mx.rooms[i].ID == pid {
				mx.rooms[i].Pinned = true
				break
			}
		}
	}
}

// GetPinnedRooms returns the current pinned room IDs
func (mx *Matrix) GetPinnedRooms() []string {
	return mx.pinnedRooms
}

// UnreadCount returns total unread messages across all rooms
func (mx *Matrix) UnreadCount() int {
	total := 0
	for _, room := range mx.rooms {
		total += room.UnreadCount
	}
	return total
}

// ---------------------------------------------------------------------------
// Markdown to HTML converter for note sharing
// ---------------------------------------------------------------------------

func matrixMarkdownToHTML(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inCodeBlock := false
	inList := false
	inBlockquote := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				out.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				if inList {
					out.WriteString("</ul>\n")
					inList = false
				}
				out.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			out.WriteString(strings.ReplaceAll(line, "<", "&lt;"))
			out.WriteString("\n")
			continue
		}

		// Close blockquote if needed
		if inBlockquote && !strings.HasPrefix(trimmed, ">") {
			out.WriteString("</blockquote>\n")
			inBlockquote = false
		}

		// Empty line
		if trimmed == "" {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "#### ") {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			out.WriteString("<h4>" + convertInlineMarkdown(trimmed[5:]) + "</h4>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			out.WriteString("<h3>" + convertInlineMarkdown(trimmed[4:]) + "</h3>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			out.WriteString("<h2>" + convertInlineMarkdown(trimmed[3:]) + "</h2>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			out.WriteString("<h1>" + convertInlineMarkdown(trimmed[2:]) + "</h1>\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			if !inBlockquote {
				if inList {
					out.WriteString("</ul>\n")
					inList = false
				}
				out.WriteString("<blockquote>")
				inBlockquote = true
			}
			out.WriteString(convertInlineMarkdown(trimmed[2:]) + "<br>")
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			out.WriteString("<li>" + convertInlineMarkdown(trimmed[2:]) + "</li>\n")
			continue
		}

		// Regular paragraph
		if inList {
			out.WriteString("</ul>\n")
			inList = false
		}
		out.WriteString("<p>" + convertInlineMarkdown(trimmed) + "</p>\n")
	}

	if inList {
		out.WriteString("</ul>\n")
	}
	if inCodeBlock {
		out.WriteString("</code></pre>\n")
	}
	if inBlockquote {
		out.WriteString("</blockquote>\n")
	}

	return out.String()
}

func convertInlineMarkdown(text string) string {
	result := text

	// Bold: **text** -> <strong>text</strong>
	for {
		start := strings.Index(result, "**")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		inner := result[start+2 : end]
		result = result[:start] + "<strong>" + inner + "</strong>" + result[end+2:]
	}

	// Italic: *text* -> <em>text</em>
	for {
		start := strings.Index(result, "*")
		if start == -1 {
			break
		}
		// Skip if it's inside a tag
		if start > 0 && result[start-1] == '<' {
			break
		}
		end := strings.Index(result[start+1:], "*")
		if end == -1 {
			break
		}
		end += start + 1
		inner := result[start+1 : end]
		if inner == "" {
			break
		}
		result = result[:start] + "<em>" + inner + "</em>" + result[end+1:]
	}

	// Inline code: `text` -> <code>text</code>
	for {
		start := strings.Index(result, "`")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1
		inner := result[start+1 : end]
		result = result[:start] + "<code>" + inner + "</code>" + result[end+1:]
	}

	// Links: [text](url) -> <a href="url">text</a>
	for {
		lbStart := strings.Index(result, "[")
		if lbStart == -1 {
			break
		}
		lbEnd := strings.Index(result[lbStart:], "](")
		if lbEnd == -1 {
			break
		}
		lbEnd += lbStart
		rpEnd := strings.Index(result[lbEnd+2:], ")")
		if rpEnd == -1 {
			break
		}
		rpEnd += lbEnd + 2
		linkText := result[lbStart+1 : lbEnd]
		linkURL := result[lbEnd+2 : rpEnd]
		result = result[:lbStart] + "<a href=\"" + linkURL + "\">" + linkText + "</a>" + result[rpEnd+1:]
	}

	return result
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func matrixHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func matrixSyncHTTPClient() *http.Client {
	return &http.Client{Timeout: 90 * time.Second}
}

func matrixDoRequest(method, url string, body interface{}, token string) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := matrixHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return respBody, nil
}

// ---------------------------------------------------------------------------
// Async tea.Cmd functions
// ---------------------------------------------------------------------------

func matrixLogin(homeserver, user, password string) tea.Cmd {
	return func() tea.Msg {
		url := strings.TrimRight(homeserver, "/") + "/_matrix/client/v3/login"
		reqBody := matrixLoginRequest{
			Type:     "m.login.password",
			User:     user,
			Password: password,
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			return matrixLoginResultMsg{err: fmt.Errorf("marshal: %w", err)}
		}

		resp, err := matrixHTTPClient().Post(url, "application/json", bytes.NewReader(data))
		if err != nil {
			return matrixLoginResultMsg{err: fmt.Errorf("cannot connect to %s: %w", homeserver, err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return matrixLoginResultMsg{err: fmt.Errorf("read response: %w", err)}
		}

		var loginResp matrixLoginResponse
		if err := json.Unmarshal(body, &loginResp); err != nil {
			return matrixLoginResultMsg{err: fmt.Errorf("parse response: %w", err)}
		}

		if loginResp.ErrCode != "" {
			return matrixLoginResultMsg{err: fmt.Errorf("%s: %s", loginResp.ErrCode, loginResp.Error)}
		}

		return matrixLoginResultMsg{
			token:  loginResp.AccessToken,
			userID: loginResp.UserID,
		}
	}
}

func matrixFetchRooms(homeserver, token string) tea.Cmd {
	return func() tea.Msg {
		url := strings.TrimRight(homeserver, "/") + "/_matrix/client/v3/joined_rooms"
		data, err := matrixDoRequest("GET", url, nil, token)
		if err != nil {
			return matrixRoomsResultMsg{err: err}
		}

		var joinedResp matrixJoinedRoomsResponse
		if err := json.Unmarshal(data, &joinedResp); err != nil {
			return matrixRoomsResultMsg{err: fmt.Errorf("parse rooms: %w", err)}
		}

		if joinedResp.Error != "" {
			return matrixRoomsResultMsg{err: fmt.Errorf("%s", joinedResp.Error)}
		}

		var rooms []matrixRoom
		for _, roomID := range joinedResp.JoinedRooms {
			room := matrixRoom{ID: roomID, Name: roomID}

			// Fetch room name
			nameURL := strings.TrimRight(homeserver, "/") + "/_matrix/client/v3/rooms/" + roomID + "/state/m.room.name"
			nameData, err := matrixDoRequest("GET", nameURL, nil, token)
			if err == nil {
				var nameResp matrixRoomNameResponse
				if json.Unmarshal(nameData, &nameResp) == nil && nameResp.Name != "" {
					room.Name = nameResp.Name
				}
			}

			// Check for encryption state event
			encURL := strings.TrimRight(homeserver, "/") + "/_matrix/client/v3/rooms/" + roomID + "/state/m.room.encryption"
			encData, err := matrixDoRequest("GET", encURL, nil, token)
			if err == nil {
				var errResp matrixErrorResponse
				if json.Unmarshal(encData, &errResp) == nil && errResp.ErrCode == "" {
					room.Encrypted = true
				}
			}

			rooms = append(rooms, room)
		}

		return matrixRoomsResultMsg{rooms: rooms}
	}
}

func matrixFetchMessages(homeserver, token, roomID string, limit int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/messages?dir=b&limit=%d",
			strings.TrimRight(homeserver, "/"), roomID, limit)

		data, err := matrixDoRequest("GET", url, nil, token)
		if err != nil {
			return matrixMessagesResultMsg{roomID: roomID, err: err}
		}

		var msgResp matrixMessagesResponse
		if err := json.Unmarshal(data, &msgResp); err != nil {
			return matrixMessagesResultMsg{roomID: roomID, err: fmt.Errorf("parse messages: %w", err)}
		}

		if msgResp.Error != "" {
			return matrixMessagesResultMsg{roomID: roomID, err: fmt.Errorf("%s", msgResp.Error)}
		}

		var messages []matrixChatMessage
		for _, ev := range msgResp.Chunk {
			if ev.Type == "m.room.message" {
				body, _ := ev.Content["body"].(string)
				if body == "" {
					continue
				}
				ts := time.Unix(0, ev.OriginServerTS*int64(time.Millisecond))

				// Check for reply
				replyTo := ""
				if relatesTo, ok := ev.Content["m.relates_to"].(map[string]interface{}); ok {
					if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
						if eid, ok := inReplyTo["event_id"].(string); ok {
							replyTo = eid
						}
					}
				}

				messages = append(messages, matrixChatMessage{
					Sender:    extractDisplayName(ev.Sender),
					Body:      body,
					Timestamp: ts,
					EventID:   ev.EventID,
					ReplyTo:   replyTo,
					Reactions: make(map[string]int),
				})
			} else if ev.Type == "m.reaction" {
				// Collect reactions
				if relatesTo, ok := ev.Content["m.relates_to"].(map[string]interface{}); ok {
					targetID, _ := relatesTo["event_id"].(string)
					emoji, _ := relatesTo["key"].(string)
					if targetID != "" && emoji != "" {
						// We'll apply these after sorting
						for i := range messages {
							if messages[i].EventID == targetID {
								if messages[i].Reactions == nil {
									messages[i].Reactions = make(map[string]int)
								}
								messages[i].Reactions[emoji]++
								break
							}
						}
					}
				}
			}
		}

		// Reverse to chronological order (API returns newest first)
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}

		return matrixMessagesResultMsg{roomID: roomID, messages: messages}
	}
}

func matrixSendMessage(homeserver, token, roomID, body string, txnID int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%d",
			strings.TrimRight(homeserver, "/"), roomID, txnID)

		content := map[string]string{
			"msgtype": "m.text",
			"body":    body,
		}

		data, err := matrixDoRequest("PUT", url, content, token)
		if err != nil {
			return matrixSendResultMsg{roomID: roomID, err: err}
		}

		var sendResp matrixSendResponse
		if err := json.Unmarshal(data, &sendResp); err != nil {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("parse send response: %w", err)}
		}

		if sendResp.ErrCode != "" {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("%s: %s", sendResp.ErrCode, sendResp.Error)}
		}

		return matrixSendResultMsg{roomID: roomID, eventID: sendResp.EventID}
	}
}

func matrixSendFormattedMessage(homeserver, token, roomID, body, htmlBody string, txnID int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%d",
			strings.TrimRight(homeserver, "/"), roomID, txnID)

		content := map[string]string{
			"msgtype":        "m.text",
			"body":           body,
			"format":         "org.matrix.custom.html",
			"formatted_body": htmlBody,
		}

		data, err := matrixDoRequest("PUT", url, content, token)
		if err != nil {
			return matrixSendResultMsg{roomID: roomID, err: err}
		}

		var sendResp matrixSendResponse
		if err := json.Unmarshal(data, &sendResp); err != nil {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("parse send response: %w", err)}
		}

		if sendResp.ErrCode != "" {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("%s: %s", sendResp.ErrCode, sendResp.Error)}
		}

		return matrixSendResultMsg{roomID: roomID, eventID: sendResp.EventID}
	}
}

func matrixSendReply(homeserver, token, roomID, body, replyToEventID string, txnID int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%d",
			strings.TrimRight(homeserver, "/"), roomID, txnID)

		content := map[string]interface{}{
			"msgtype": "m.text",
			"body":    body,
			"m.relates_to": map[string]interface{}{
				"m.in_reply_to": map[string]interface{}{
					"event_id": replyToEventID,
				},
			},
		}

		data, err := matrixDoRequest("PUT", url, content, token)
		if err != nil {
			return matrixSendResultMsg{roomID: roomID, err: err}
		}

		var sendResp matrixSendResponse
		if err := json.Unmarshal(data, &sendResp); err != nil {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("parse send response: %w", err)}
		}

		if sendResp.ErrCode != "" {
			return matrixSendResultMsg{roomID: roomID, err: fmt.Errorf("%s: %s", sendResp.ErrCode, sendResp.Error)}
		}

		return matrixSendResultMsg{roomID: roomID, eventID: sendResp.EventID}
	}
}

func matrixSendReaction(homeserver, token, roomID, eventID, emoji string, txnID int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.reaction/%d",
			strings.TrimRight(homeserver, "/"), roomID, txnID)

		content := map[string]interface{}{
			"m.relates_to": map[string]interface{}{
				"rel_type": "m.annotation",
				"event_id": eventID,
				"key":      emoji,
			},
		}

		_, err := matrixDoRequest("PUT", url, content, token)
		return matrixReactionResultMsg{err: err}
	}
}

func matrixSync(homeserver, token, since string) tea.Cmd {
	return func() tea.Msg {
		url := strings.TrimRight(homeserver, "/") + "/_matrix/client/v3/sync?timeout=30000"
		if since != "" {
			url += "&since=" + since
		} else {
			// For initial sync, only fetch recent events
			url += "&filter={\"room\":{\"timeline\":{\"limit\":10}}}"
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return matrixSyncResultMsg{err: fmt.Errorf("request: %w", err)}
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := matrixSyncHTTPClient().Do(req)
		if err != nil {
			return matrixSyncResultMsg{err: fmt.Errorf("sync: %w", err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return matrixSyncResultMsg{err: fmt.Errorf("read sync: %w", err)}
		}

		var syncResp matrixSyncResponse
		if err := json.Unmarshal(body, &syncResp); err != nil {
			return matrixSyncResultMsg{err: fmt.Errorf("parse sync: %w", err)}
		}

		if syncResp.ErrCode != "" {
			return matrixSyncResultMsg{err: fmt.Errorf("%s: %s", syncResp.ErrCode, syncResp.Error)}
		}

		return matrixSyncResultMsg{resp: &syncResp}
	}
}

func matrixSendReadReceipt(homeserver, token, roomID, eventID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/receipt/m.read/%s",
			strings.TrimRight(homeserver, "/"), roomID, eventID)
		matrixDoRequest("POST", url, map[string]interface{}{}, token)
		return nil
	}
}

func matrixSendTyping(homeserver, token, roomID, userID string, typing bool) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/typing/%s",
			strings.TrimRight(homeserver, "/"), roomID, userID)
		body := map[string]interface{}{
			"typing":  typing,
			"timeout": 30000,
		}
		matrixDoRequest("PUT", url, body, token)
		return nil
	}
}

// extractDisplayName gets a short display name from a full Matrix user ID (@user:server)
func extractDisplayName(userID string) string {
	if strings.HasPrefix(userID, "@") {
		parts := strings.SplitN(userID[1:], ":", 2)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return userID
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (mx Matrix) Update(msg tea.Msg) (Matrix, tea.Cmd) {
	if !mx.active {
		return mx, nil
	}

	switch msg := msg.(type) {
	case matrixLoginResultMsg:
		if msg.err != nil {
			mx.loginError = msg.err.Error()
			mx.connState = matrixDisconnected
		} else {
			mx.accessToken = msg.token
			mx.userID = msg.userID
			mx.connState = matrixConnected
			mx.focus = matrixFocusRooms
			mx.loginError = ""
			mx.loginPass = ""
			mx.statusMsg = "Logged in as " + extractDisplayName(msg.userID)
			// Fetch rooms after login
			return mx, matrixFetchRooms(mx.homeserver, mx.accessToken)
		}
		return mx, nil

	case matrixRoomsResultMsg:
		if msg.err != nil {
			mx.statusMsg = "Error loading rooms: " + msg.err.Error()
		} else {
			mx.rooms = msg.rooms
			// Apply pinned status
			for i := range mx.rooms {
				for _, pid := range mx.pinnedRooms {
					if mx.rooms[i].ID == pid {
						mx.rooms[i].Pinned = true
						break
					}
				}
			}
			mx.sortRooms()
			if len(mx.rooms) > 0 {
				mx.statusMsg = fmt.Sprintf("%d rooms loaded", len(mx.rooms))
				// Fetch messages for first room
				return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, mx.rooms[0].ID, 50)
			} else {
				mx.statusMsg = "No rooms joined"
			}
		}
		return mx, nil

	case matrixMessagesResultMsg:
		if msg.err != nil {
			mx.statusMsg = "Error loading messages: " + msg.err.Error()
		} else {
			// Mark own messages
			for i := range msg.messages {
				if msg.messages[i].Sender == extractDisplayName(mx.userID) {
					msg.messages[i].IsOwn = true
				}
			}
			mx.messages[msg.roomID] = msg.messages
			mx.messageScroll = 0
		}
		return mx, nil

	case matrixSendResultMsg:
		if msg.err != nil {
			mx.statusMsg = "Send error: " + msg.err.Error()
		} else {
			mx.statusMsg = "Message sent"
			// Clear reply context after sending
			mx.replyToEvent = ""
			mx.replyPreview = ""
			// Refresh messages for the room
			return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, msg.roomID, 50)
		}
		return mx, nil

	case matrixReactionResultMsg:
		if msg.err != nil {
			mx.statusMsg = "Reaction error: " + msg.err.Error()
		} else {
			mx.statusMsg = "Reaction sent"
			// Refresh to see the reaction
			rooms := mx.filteredRooms()
			if mx.roomCursor < len(rooms) {
				return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, rooms[mx.roomCursor].ID, 50)
			}
		}
		return mx, nil

	case matrixSyncResultMsg:
		if msg.err != nil {
			mx.connState = matrixConnected
			mx.statusMsg = "Sync error: " + msg.err.Error()
		} else if msg.resp != nil {
			mx.syncToken = msg.resp.NextBatch
			mx.connState = matrixConnected
			mx.processSyncResponse(msg.resp)
			// Continue syncing if still active
			if mx.active && mx.accessToken != "" {
				return mx, matrixSync(mx.homeserver, mx.accessToken, mx.syncToken)
			}
		}
		return mx, nil

	case matrixSyncTickMsg:
		if mx.active && mx.accessToken != "" && !mx.syncRunning {
			mx.syncRunning = true
			mx.connState = matrixSyncing
			return mx, matrixSync(mx.homeserver, mx.accessToken, mx.syncToken)
		}
		return mx, nil

	case matrixAISummaryMsg, matrixAIActionsMsg, matrixAIReplyMsg,
		matrixAITranslateMsg, matrixAINoteMsg, matrixAIContextMsg, matrixAITickMsg:
		var cmd tea.Cmd
		mx.ai, cmd = mx.ai.Update(msg)
		return mx, cmd

	case tea.KeyMsg:
		// If AI panel is active, route keys there first
		if mx.ai.IsActive() {
			var cmd tea.Cmd
			mx.ai, cmd = mx.ai.Update(msg)
			// Check if a reply was selected
			if reply := mx.ai.GetSelectedReply(); reply != "" {
				mx.messageInput = reply
			}
			// Check if AI panel was closed (Esc handled by AI)
			if !mx.ai.IsActive() {
				return mx, cmd
			}
			return mx, cmd
		}
		return mx.handleKeyMsg(msg)
	}

	return mx, nil
}

func (mx *Matrix) processSyncResponse(resp *matrixSyncResponse) {
	mx.syncRunning = false
	for roomID, joinedRoom := range resp.Rooms.Join {
		// Update unread counts
		for i := range mx.rooms {
			if mx.rooms[i].ID == roomID {
				mx.rooms[i].UnreadCount = joinedRoom.UnreadNotifications.NotificationCount
				break
			}
		}

		// Append new messages
		for _, ev := range joinedRoom.Timeline.Events {
			if ev.Type == "m.room.message" {
				body, _ := ev.Content["body"].(string)
				if body == "" {
					continue
				}
				ts := time.Unix(0, ev.OriginServerTS*int64(time.Millisecond))

				replyTo := ""
				if relatesTo, ok := ev.Content["m.relates_to"].(map[string]interface{}); ok {
					if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
						if eid, ok := inReplyTo["event_id"].(string); ok {
							replyTo = eid
						}
					}
				}

				msg := matrixChatMessage{
					Sender:    extractDisplayName(ev.Sender),
					Body:      body,
					Timestamp: ts,
					EventID:   ev.EventID,
					IsOwn:     ev.Sender == mx.userID,
					ReplyTo:   replyTo,
					Reactions: make(map[string]int),
				}

				// Check for duplicates
				existing := mx.messages[roomID]
				isDuplicate := false
				for _, m := range existing {
					if m.EventID == ev.EventID {
						isDuplicate = true
						break
					}
				}
				if !isDuplicate {
					mx.messages[roomID] = append(mx.messages[roomID], msg)
				}
			} else if ev.Type == "m.reaction" {
				if relatesTo, ok := ev.Content["m.relates_to"].(map[string]interface{}); ok {
					targetID, _ := relatesTo["event_id"].(string)
					emoji, _ := relatesTo["key"].(string)
					if targetID != "" && emoji != "" {
						msgs := mx.messages[roomID]
						for i := range msgs {
							if msgs[i].EventID == targetID {
								if msgs[i].Reactions == nil {
									msgs[i].Reactions = make(map[string]int)
								}
								msgs[i].Reactions[emoji]++
								break
							}
						}
					}
				}
			}
		}
	}
}

// sortRooms puts pinned rooms at the top
func (mx *Matrix) sortRooms() {
	pinned := make([]matrixRoom, 0)
	unpinned := make([]matrixRoom, 0)
	for _, r := range mx.rooms {
		if r.Pinned {
			pinned = append(pinned, r)
		} else {
			unpinned = append(unpinned, r)
		}
	}
	mx.rooms = append(pinned, unpinned...)
}

func (mx Matrix) handleKeyMsg(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "esc":
		if mx.showReactionPicker {
			mx.showReactionPicker = false
			return mx, nil
		}
		if mx.msgSearchMode {
			mx.msgSearchMode = false
			mx.msgSearchQuery = ""
			mx.msgSearchMatches = nil
			return mx, nil
		}
		if mx.showPrivacy {
			mx.showPrivacy = false
			mx.focus = matrixFocusRooms
			return mx, nil
		}
		if mx.focus == matrixFocusNewDM {
			mx.focus = matrixFocusRooms
			return mx, nil
		}
		if mx.searching {
			mx.searching = false
			mx.roomSearch = ""
			return mx, nil
		}
		if mx.messageSelect {
			mx.messageSelect = false
			return mx, nil
		}
		if mx.replyToEvent != "" {
			mx.replyToEvent = ""
			mx.replyPreview = ""
			return mx, nil
		}
		mx.Close()
		return mx, nil

	case "ctrl+m":
		mx.Close()
		return mx, nil

	// AI features
	case "ctrl+s", "ctrl+a", "ctrl+r", "ctrl+t", "ctrl+n", "ctrl+i":
		if mx.connState == matrixConnected || mx.connState == matrixSyncing {
			rooms := mx.filteredRooms()
			var msgs []matrixChatMessage
			if mx.roomCursor < len(rooms) {
				msgs = mx.messages[rooms[mx.roomCursor].ID]
			}
			mx.ai.SetSize(mx.width-4, mx.height-4)
			cmd := mx.ai.HandleKey(key, msgs, mx.currentNoteContent)
			if cmd != nil {
				return mx, cmd
			}
		}
		return mx, nil

	case "tab":
		if mx.focus == matrixFocusLogin {
			return mx, nil
		}
		if mx.showPrivacy {
			return mx, nil
		}
		if mx.showReactionPicker {
			return mx, nil
		}
		// Cycle: rooms -> messages -> input -> rooms
		switch mx.focus {
		case matrixFocusRooms:
			mx.focus = matrixFocusMessages
		case matrixFocusMessages:
			mx.focus = matrixFocusInput
		case matrixFocusInput:
			mx.focus = matrixFocusRooms
		}
		return mx, nil
	}

	// Login form handling
	if mx.focus == matrixFocusLogin {
		return mx.handleLoginKeys(msg)
	}

	// Privacy panel
	if mx.showPrivacy {
		return mx.handlePrivacyKeys(msg)
	}

	// New DM
	if mx.focus == matrixFocusNewDM {
		return mx.handleNewDMKeys(msg)
	}

	// Reaction picker
	if mx.showReactionPicker {
		return mx.handleReactionPickerKeys(msg)
	}

	// Room search
	if mx.searching {
		return mx.handleSearchKeys(msg)
	}

	// Message search
	if mx.msgSearchMode {
		return mx.handleMsgSearchKeys(msg)
	}

	// Focus-specific keys
	switch mx.focus {
	case matrixFocusRooms:
		return mx.handleRoomKeys(msg)
	case matrixFocusMessages:
		return mx.handleMessageKeys(msg)
	case matrixFocusInput:
		return mx.handleInputKeys(msg)
	}

	return mx, nil
}

func (mx Matrix) handleLoginKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "tab", "down":
		mx.loginFocus = (mx.loginFocus + 1) % 3
	case "shift+tab", "up":
		mx.loginFocus = (mx.loginFocus + 2) % 3
	case "enter":
		if mx.loginServer != "" && mx.loginUser != "" && mx.loginPass != "" {
			mx.connState = matrixConnecting
			mx.loginError = ""
			mx.homeserver = strings.TrimRight(mx.loginServer, "/")
			mx.username = mx.loginUser
			return mx, matrixLogin(mx.homeserver, mx.loginUser, mx.loginPass)
		} else {
			mx.loginError = "Please fill in all fields"
		}
	case "backspace":
		switch mx.loginFocus {
		case 0:
			if len(mx.loginServer) > 0 {
				mx.loginServer = mx.loginServer[:len(mx.loginServer)-1]
			}
		case 1:
			if len(mx.loginUser) > 0 {
				mx.loginUser = mx.loginUser[:len(mx.loginUser)-1]
			}
		case 2:
			if len(mx.loginPass) > 0 {
				mx.loginPass = mx.loginPass[:len(mx.loginPass)-1]
			}
		}
	default:
		char := key
		if len(char) == 1 {
			switch mx.loginFocus {
			case 0:
				mx.loginServer += char
			case 1:
				mx.loginUser += char
			case 2:
				mx.loginPass += char
			}
		}
	}
	return mx, nil
}

func (mx Matrix) handlePrivacyKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "1", "r":
		mx.sendReadReceipts = !mx.sendReadReceipts
	case "2", "t":
		mx.sendTypingIndicators = !mx.sendTypingIndicators
	case "3", "c":
		mx.autoDeleteCache = !mx.autoDeleteCache
	}
	return mx, nil
}

func (mx Matrix) handleNewDMKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		if mx.newDMInput != "" {
			mx.statusMsg = "DM creation not yet implemented for " + mx.newDMInput
			mx.focus = matrixFocusRooms
			mx.newDMInput = ""
		}
	case "backspace":
		if len(mx.newDMInput) > 0 {
			mx.newDMInput = mx.newDMInput[:len(mx.newDMInput)-1]
		}
	default:
		char := key
		if len(char) == 1 {
			mx.newDMInput += char
		}
	}
	return mx, nil
}

func (mx Matrix) handleSearchKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		mx.searching = false
	case "backspace":
		if len(mx.roomSearch) > 0 {
			mx.roomSearch = mx.roomSearch[:len(mx.roomSearch)-1]
		}
	default:
		char := key
		if len(char) == 1 {
			mx.roomSearch += char
		}
	}
	return mx, nil
}

func (mx Matrix) handleReactionPickerKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "h", "left":
		if mx.reactionCursor > 0 {
			mx.reactionCursor--
		}
	case "l", "right":
		if mx.reactionCursor < len(matrixReactionEmojis)-1 {
			mx.reactionCursor++
		}
	case "enter":
		// Send the reaction
		rooms := mx.filteredRooms()
		if mx.roomCursor < len(rooms) {
			msgs := mx.messages[rooms[mx.roomCursor].ID]
			if mx.messageCursor >= 0 && mx.messageCursor < len(msgs) {
				mx.txnCounter++
				emoji := matrixReactionEmojis[mx.reactionCursor]
				mx.showReactionPicker = false
				return mx, matrixSendReaction(
					mx.homeserver, mx.accessToken,
					rooms[mx.roomCursor].ID,
					msgs[mx.messageCursor].EventID,
					emoji, mx.txnCounter,
				)
			}
		}
		mx.showReactionPicker = false
	case "1":
		mx.reactionCursor = 0
		return mx.sendSelectedReaction()
	case "2":
		mx.reactionCursor = 1
		return mx.sendSelectedReaction()
	case "3":
		mx.reactionCursor = 2
		return mx.sendSelectedReaction()
	case "4":
		mx.reactionCursor = 3
		return mx.sendSelectedReaction()
	case "5":
		mx.reactionCursor = 4
		return mx.sendSelectedReaction()
	case "6":
		mx.reactionCursor = 5
		return mx.sendSelectedReaction()
	}
	return mx, nil
}

func (mx Matrix) sendSelectedReaction() (Matrix, tea.Cmd) {
	rooms := mx.filteredRooms()
	if mx.roomCursor < len(rooms) {
		msgs := mx.messages[rooms[mx.roomCursor].ID]
		if mx.messageCursor >= 0 && mx.messageCursor < len(msgs) {
			mx.txnCounter++
			emoji := matrixReactionEmojis[mx.reactionCursor]
			mx.showReactionPicker = false
			return mx, matrixSendReaction(
				mx.homeserver, mx.accessToken,
				rooms[mx.roomCursor].ID,
				msgs[mx.messageCursor].EventID,
				emoji, mx.txnCounter,
			)
		}
	}
	mx.showReactionPicker = false
	return mx, nil
}

// handleMsgSearchKeys handles message search mode input
func (mx Matrix) handleMsgSearchKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		// Finalize search, compute matches
		mx.updateMsgSearchMatches()
		if len(mx.msgSearchMatches) > 0 {
			mx.msgSearchIdx = 0
			mx.scrollToMessage(mx.msgSearchMatches[0])
		}
	case "backspace":
		if len(mx.msgSearchQuery) > 0 {
			mx.msgSearchQuery = mx.msgSearchQuery[:len(mx.msgSearchQuery)-1]
			mx.updateMsgSearchMatches()
		}
	case "n":
		// Next match (only if search is finalized, i.e. after enter)
		if len(mx.msgSearchMatches) > 0 {
			mx.msgSearchIdx = (mx.msgSearchIdx + 1) % len(mx.msgSearchMatches)
			mx.scrollToMessage(mx.msgSearchMatches[mx.msgSearchIdx])
		}
		return mx, nil
	case "N":
		// Previous match
		if len(mx.msgSearchMatches) > 0 {
			mx.msgSearchIdx--
			if mx.msgSearchIdx < 0 {
				mx.msgSearchIdx = len(mx.msgSearchMatches) - 1
			}
			mx.scrollToMessage(mx.msgSearchMatches[mx.msgSearchIdx])
		}
		return mx, nil
	default:
		char := key
		if len(char) == 1 {
			mx.msgSearchQuery += char
			mx.updateMsgSearchMatches()
		}
	}
	return mx, nil
}

func (mx *Matrix) updateMsgSearchMatches() {
	mx.msgSearchMatches = nil
	if mx.msgSearchQuery == "" {
		return
	}
	rooms := mx.filteredRooms()
	if mx.roomCursor >= len(rooms) {
		return
	}
	msgs := mx.messages[rooms[mx.roomCursor].ID]
	query := strings.ToLower(mx.msgSearchQuery)
	for i, msg := range msgs {
		if strings.Contains(strings.ToLower(msg.Body), query) ||
			strings.Contains(strings.ToLower(msg.Sender), query) {
			mx.msgSearchMatches = append(mx.msgSearchMatches, i)
		}
	}
}

func (mx *Matrix) scrollToMessage(msgIdx int) {
	rooms := mx.filteredRooms()
	if mx.roomCursor >= len(rooms) {
		return
	}
	msgs := mx.messages[rooms[mx.roomCursor].ID]
	if msgIdx >= 0 && msgIdx < len(msgs) {
		// Scroll so the message is visible (approximate: set scroll based on distance from end)
		mx.messageScroll = len(msgs) - msgIdx - 5
		if mx.messageScroll < 0 {
			mx.messageScroll = 0
		}
	}
}

func (mx Matrix) handleRoomKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	filtered := mx.filteredRooms()
	switch key {
	case "j", "down":
		if mx.roomCursor < len(filtered)-1 {
			mx.roomCursor++
			mx.messageScroll = 0
			if mx.roomCursor < len(filtered) {
				roomID := filtered[mx.roomCursor].ID
				if _, ok := mx.messages[roomID]; !ok {
					return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, roomID, 50)
				}
			}
		}
	case "k", "up":
		if mx.roomCursor > 0 {
			mx.roomCursor--
			mx.messageScroll = 0
			if mx.roomCursor < len(filtered) {
				roomID := filtered[mx.roomCursor].ID
				if _, ok := mx.messages[roomID]; !ok {
					return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, roomID, 50)
				}
			}
		}
	case "enter":
		mx.focus = matrixFocusInput
		if mx.roomCursor < len(filtered) {
			roomID := filtered[mx.roomCursor].ID
			// Send read receipt if enabled
			if mx.sendReadReceipts {
				msgs := mx.messages[roomID]
				if len(msgs) > 0 {
					lastMsg := msgs[len(msgs)-1]
					return mx, matrixSendReadReceipt(mx.homeserver, mx.accessToken, roomID, lastMsg.EventID)
				}
			}
		}
	case "/":
		mx.searching = true
		mx.roomSearch = ""
	case "s":
		// Share current note to selected room with HTML formatting
		if mx.shareContent != "" && mx.roomCursor < len(filtered) {
			roomID := filtered[mx.roomCursor].ID
			mx.txnCounter++
			plainBody := "Shared note from Granit:\n\n" + mx.shareContent
			if len(plainBody) > 4000 {
				plainBody = plainBody[:4000] + "\n\n[truncated]"
			}
			htmlBody := "<h3>Shared note from Granit</h3>\n" + matrixMarkdownToHTML(mx.shareContent)
			if len(htmlBody) > 8000 {
				htmlBody = htmlBody[:8000] + "\n<p><em>[truncated]</em></p>"
			}
			return mx, matrixSendFormattedMessage(mx.homeserver, mx.accessToken, roomID, plainBody, htmlBody, mx.txnCounter)
		} else if mx.shareContent == "" {
			mx.statusMsg = "No note content to share"
		}
	case "S":
		// Share selected text
		if mx.shareContent != "" && mx.roomCursor < len(filtered) {
			roomID := filtered[mx.roomCursor].ID
			mx.txnCounter++
			return mx, matrixSendMessage(mx.homeserver, mx.accessToken, roomID, mx.shareContent, mx.txnCounter)
		}
	case "p":
		mx.showPrivacy = true
		mx.focus = matrixFocusPrivacy
	case "P":
		// Toggle pin on selected room
		if mx.roomCursor < len(filtered) {
			roomID := filtered[mx.roomCursor].ID
			mx.togglePinRoom(roomID)
			mx.sortRooms()
			mx.statusMsg = "Room pin toggled"
		}
	case "n":
		mx.focus = matrixFocusNewDM
		mx.newDMInput = ""
	case "R":
		// Refresh rooms
		return mx, matrixFetchRooms(mx.homeserver, mx.accessToken)
	}
	return mx, nil
}

func (mx *Matrix) togglePinRoom(roomID string) {
	// Check if already pinned
	for i, pid := range mx.pinnedRooms {
		if pid == roomID {
			// Unpin
			mx.pinnedRooms = append(mx.pinnedRooms[:i], mx.pinnedRooms[i+1:]...)
			for j := range mx.rooms {
				if mx.rooms[j].ID == roomID {
					mx.rooms[j].Pinned = false
					break
				}
			}
			return
		}
	}
	// Pin
	mx.pinnedRooms = append(mx.pinnedRooms, roomID)
	for j := range mx.rooms {
		if mx.rooms[j].ID == roomID {
			mx.rooms[j].Pinned = true
			break
		}
	}
}

func (mx Matrix) handleMessageKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()

	rooms := mx.filteredRooms()
	var msgs []matrixChatMessage
	if mx.roomCursor < len(rooms) {
		msgs = mx.messages[rooms[mx.roomCursor].ID]
	}

	switch key {
	case "ctrl+d":
		mx.messageScroll += 10
	case "ctrl+u":
		mx.messageScroll -= 10
		if mx.messageScroll < 0 {
			mx.messageScroll = 0
		}
	case "j", "down":
		if mx.messageSelect && len(msgs) > 0 {
			if mx.messageCursor < len(msgs)-1 {
				mx.messageCursor++
			}
		} else {
			mx.messageScroll++
		}
	case "k", "up":
		if mx.messageSelect && mx.messageCursor > 0 {
			mx.messageCursor--
		} else if mx.messageScroll > 0 {
			mx.messageScroll--
		}
	case "G":
		mx.messageScroll = 0 // scroll to bottom (0 = bottom since we render from bottom)
	case "i", "enter":
		mx.focus = matrixFocusInput
		mx.messageSelect = false
	case "/":
		// Enter message search mode
		mx.msgSearchMode = true
		mx.msgSearchQuery = ""
		mx.msgSearchMatches = nil
	case "v":
		// Toggle message selection mode
		mx.messageSelect = !mx.messageSelect
		if mx.messageSelect && len(msgs) > 0 {
			mx.messageCursor = len(msgs) - 1
		}
	case "c":
		// Capture selected message as a note
		if mx.messageSelect && len(msgs) > 0 && mx.messageCursor < len(msgs) && mx.captureNote != nil {
			msg := msgs[mx.messageCursor]
			now := time.Now()
			filename := fmt.Sprintf("Matrix - %s - %s.md", msg.Sender, now.Format("2006-01-02"))
			roomName := ""
			if mx.roomCursor < len(rooms) {
				roomName = rooms[mx.roomCursor].Name
			}
			content := fmt.Sprintf("---\ntags: [matrix, captured]\nsource_room: %s\ntimestamp: %s\n---\n\n%s\n",
				roomName,
				msg.Timestamp.Format(time.RFC3339),
				msg.Body,
			)
			mx.captureNote(filename, content)
			mx.statusMsg = "Captured message to note: " + filename
		} else if !mx.messageSelect {
			mx.statusMsg = "Press v to select messages first, then c to capture"
		}
	case "C":
		// Capture entire conversation as a note
		if mx.captureNote != nil && mx.roomCursor < len(rooms) && len(msgs) > 0 {
			room := rooms[mx.roomCursor]
			now := time.Now()
			filename := fmt.Sprintf("Matrix Thread - %s - %s.md",
				room.Name, now.Format("2006-01-02"))

			// Gather participants
			participants := make(map[string]bool)
			var earliest, latest time.Time
			for i, m := range msgs {
				participants[m.Sender] = true
				if i == 0 || m.Timestamp.Before(earliest) {
					earliest = m.Timestamp
				}
				if i == 0 || m.Timestamp.After(latest) {
					latest = m.Timestamp
				}
			}
			partList := make([]string, 0, len(participants))
			for p := range participants {
				partList = append(partList, p)
			}

			var contentBuf strings.Builder
			contentBuf.WriteString("---\n")
			contentBuf.WriteString("tags: [matrix, captured, thread]\n")
			contentBuf.WriteString(fmt.Sprintf("room: %s\n", room.Name))
			contentBuf.WriteString(fmt.Sprintf("participants: [%s]\n", strings.Join(partList, ", ")))
			contentBuf.WriteString(fmt.Sprintf("date_start: %s\n", earliest.Format("2006-01-02")))
			contentBuf.WriteString(fmt.Sprintf("date_end: %s\n", latest.Format("2006-01-02")))
			contentBuf.WriteString("---\n\n")

			for _, m := range msgs {
				contentBuf.WriteString(fmt.Sprintf("**%s** (%s): %s\n\n",
					m.Sender, m.Timestamp.Format("15:04"), m.Body))
			}

			mx.captureNote(filename, contentBuf.String())
			mx.statusMsg = "Captured thread to note: " + filename
		}
	case "r":
		// Open reaction picker
		if mx.messageSelect && len(msgs) > 0 && mx.messageCursor < len(msgs) {
			mx.showReactionPicker = true
			mx.reactionCursor = 0
		} else if !mx.messageSelect {
			mx.statusMsg = "Press v to select a message first, then r to react"
		}
	case "R":
		if !mx.messageSelect {
			// Refresh rooms (same as room panel R)
			return mx, matrixFetchRooms(mx.homeserver, mx.accessToken)
		}
		// Reply to selected message
		if mx.messageSelect && len(msgs) > 0 && mx.messageCursor < len(msgs) {
			targetMsg := msgs[mx.messageCursor]
			mx.replyToEvent = targetMsg.EventID
			preview := targetMsg.Body
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			mx.replyPreview = targetMsg.Sender + ": " + preview
			mx.focus = matrixFocusInput
			mx.messageSelect = false
		}
	}
	return mx, nil
}

func (mx Matrix) handleInputKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		if mx.messageInput != "" {
			// Check for slash commands
			if strings.HasPrefix(mx.messageInput, "/") {
				result := mx.handleSlashCommand(mx.messageInput)
				mx.messageInput = ""
				return mx, result
			}

			filtered := mx.filteredRooms()
			if mx.roomCursor < len(filtered) {
				roomID := filtered[mx.roomCursor].ID
				mx.txnCounter++
				input := mx.messageInput
				mx.messageInput = ""
				var cmds []tea.Cmd

				if mx.replyToEvent != "" {
					cmds = append(cmds, matrixSendReply(mx.homeserver, mx.accessToken, roomID, input, mx.replyToEvent, mx.txnCounter))
				} else {
					cmds = append(cmds, matrixSendMessage(mx.homeserver, mx.accessToken, roomID, input, mx.txnCounter))
				}
				// Stop typing indicator
				if mx.sendTypingIndicators && mx.userID != "" {
					cmds = append(cmds, matrixSendTyping(mx.homeserver, mx.accessToken, roomID, mx.userID, false))
				}
				return mx, tea.Batch(cmds...)
			}
		}
	case "backspace":
		if len(mx.messageInput) > 0 {
			mx.messageInput = mx.messageInput[:len(mx.messageInput)-1]
		}
	default:
		char := key
		if len(char) == 1 {
			mx.messageInput += char
			// Send typing indicator
			if mx.sendTypingIndicators && mx.userID != "" {
				filtered := mx.filteredRooms()
				if mx.roomCursor < len(filtered) {
					roomID := filtered[mx.roomCursor].ID
					return mx, matrixSendTyping(mx.homeserver, mx.accessToken, roomID, mx.userID, true)
				}
			}
		}
	}
	return mx, nil
}

// ---------------------------------------------------------------------------
// Slash Commands
// ---------------------------------------------------------------------------

func (mx *Matrix) handleSlashCommand(input string) tea.Cmd {
	parts := strings.SplitN(strings.TrimSpace(input), " ", 2)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch cmd {
	case "/note":
		// Create a quick note from current room context
		if arg == "" {
			mx.statusMsg = "Usage: /note <title>"
			return nil
		}
		if mx.captureNote != nil {
			rooms := mx.filteredRooms()
			roomName := ""
			if mx.roomCursor < len(rooms) {
				roomName = rooms[mx.roomCursor].Name
			}
			filename := arg
			if !strings.HasSuffix(filename, ".md") {
				filename += ".md"
			}
			content := fmt.Sprintf("---\ntags: [matrix, note]\nsource_room: %s\ncreated: %s\n---\n\n",
				roomName, time.Now().Format(time.RFC3339))
			mx.captureNote(filename, content)
			mx.statusMsg = "Created note: " + filename
		} else {
			mx.statusMsg = "Note capture not available"
		}
		return nil

	case "/search":
		// Search vault notes
		if arg == "" {
			mx.statusMsg = "Usage: /search <query>"
			return nil
		}
		query := strings.ToLower(arg)
		var matches []string
		for _, f := range mx.vaultFiles {
			if strings.Contains(strings.ToLower(f), query) {
				matches = append(matches, f)
				if len(matches) >= 10 {
					break
				}
			}
		}
		if len(matches) == 0 {
			mx.slashResults = []string{"No notes found matching: " + arg}
		} else {
			mx.slashResults = make([]string, 0, len(matches)+1)
			mx.slashResults = append(mx.slashResults, fmt.Sprintf("Found %d notes:", len(matches)))
			mx.slashResults = append(mx.slashResults, matches...)
		}
		mx.statusMsg = fmt.Sprintf("Search: %d results for '%s'", len(matches), arg)
		return nil

	case "/share":
		// Share a specific note by name
		if arg == "" {
			mx.statusMsg = "Usage: /share <note-name>"
			return nil
		}
		query := strings.ToLower(arg)
		var bestMatch string
		for _, f := range mx.vaultFiles {
			if strings.Contains(strings.ToLower(f), query) {
				bestMatch = f
				break
			}
		}
		if bestMatch == "" {
			mx.statusMsg = "No note found matching: " + arg
			return nil
		}
		// We need the content - use the shareContent for now with a message
		rooms := mx.filteredRooms()
		if mx.roomCursor < len(rooms) {
			mx.txnCounter++
			shareMsg := fmt.Sprintf("Shared note: %s", bestMatch)
			return matrixSendMessage(mx.homeserver, mx.accessToken, rooms[mx.roomCursor].ID, shareMsg, mx.txnCounter)
		}
		return nil

	case "/pin":
		// Pin current room to sidebar
		rooms := mx.filteredRooms()
		if mx.roomCursor < len(rooms) {
			mx.togglePinRoom(rooms[mx.roomCursor].ID)
			mx.sortRooms()
			mx.statusMsg = "Room pin toggled"
		}
		return nil

	case "/task":
		// Create a task
		if arg == "" {
			mx.statusMsg = "Usage: /task <description>"
			return nil
		}
		if mx.captureNote != nil {
			taskLine := fmt.Sprintf("- [ ] %s\n", arg)
			mx.captureNote("Tasks.md", taskLine)
			mx.statusMsg = "Task added: " + arg
		} else {
			mx.statusMsg = "Note capture not available"
		}
		return nil

	case "/status":
		// Show connection status
		rooms := mx.filteredRooms()
		totalUnread := mx.UnreadCount()
		connStr := "Disconnected"
		switch mx.connState {
		case matrixConnected:
			connStr = "Connected"
		case matrixConnecting:
			connStr = "Connecting"
		case matrixSyncing:
			connStr = "Syncing"
		}
		mx.slashResults = []string{
			fmt.Sprintf("Connection: %s", connStr),
			fmt.Sprintf("Server: %s", mx.homeserver),
			fmt.Sprintf("User: %s", extractDisplayName(mx.userID)),
			fmt.Sprintf("Rooms: %d", len(rooms)),
			fmt.Sprintf("Unread: %d", totalUnread),
			fmt.Sprintf("Sync token: %s", truncateStr(mx.syncToken, 20)),
		}
		mx.statusMsg = "Status info displayed"
		return nil

	case "/help":
		mx.slashResults = []string{
			"Available commands:",
			"  /note <title>    - Create a quick note",
			"  /search <query>  - Search vault notes",
			"  /share <name>    - Share a note to the room",
			"  /pin             - Toggle pin on current room",
			"  /task <desc>     - Create a task in Tasks.md",
			"  /status          - Show connection info",
			"  /help            - Show this help",
		}
		mx.statusMsg = "Showing help"
		return nil

	default:
		mx.statusMsg = "Unknown command: " + cmd + " (try /help)"
		return nil
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (mx *Matrix) filteredRooms() []matrixRoom {
	if mx.roomSearch == "" {
		return mx.rooms
	}
	query := strings.ToLower(mx.roomSearch)
	var filtered []matrixRoom
	for _, r := range mx.rooms {
		if strings.Contains(strings.ToLower(r.Name), query) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// StartSync initiates background sync if connected
func (mx *Matrix) StartSync() tea.Cmd {
	if mx.accessToken == "" {
		return nil
	}
	return matrixSync(mx.homeserver, mx.accessToken, mx.syncToken)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (mx Matrix) View() string {
	width := mx.width * 3 / 4
	if width < 60 {
		width = 60
	}
	if width > 120 {
		width = 120
	}
	innerW := width - 6

	height := mx.height * 3 / 4
	if height < 20 {
		height = 20
	}
	if height > 50 {
		height = 50
	}

	var b strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	connStatus := mx.connectionBadge()
	title := titleStyle.Render("  " + IconLinkChar + " Matrix Chat")
	b.WriteString(title + "  " + connStatus)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	if mx.focus == matrixFocusLogin && mx.accessToken == "" {
		b.WriteString(mx.renderLoginForm(innerW, height-6))
	} else if mx.showPrivacy {
		b.WriteString(mx.renderPrivacyPanel(innerW, height-6))
	} else if mx.focus == matrixFocusNewDM {
		b.WriteString(mx.renderNewDMForm(innerW))
	} else {
		b.WriteString(mx.renderMainView(innerW, height-6))
	}

	// Status bar
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	// Slash command results
	if len(mx.slashResults) > 0 {
		sysStyle := lipgloss.NewStyle().Foreground(yellow).Italic(true)
		for _, line := range mx.slashResults {
			b.WriteString(sysStyle.Render("  " + line))
			b.WriteString("\n")
		}
		mx.slashResults = nil
	}

	if mx.statusMsg != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  " + mx.statusMsg))
	} else if mx.accessToken != "" {
		hints := "  Tab: switch  /: search  s: share  P: pin  ^S: AI summary  ^A: actions  ^R: reply  Esc: close"
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(hints))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 1).
		Width(width).
		Background(mantle)

	result := border.Render(b.String())

	// Overlay AI panel on top if active
	if mx.ai.IsActive() {
		aiView := mx.ai.View(width-2, height-4)
		result = result + "\n" + aiView
	}

	return result
}

func (mx Matrix) connectionBadge() string {
	switch mx.connState {
	case matrixConnected:
		return lipgloss.NewStyle().Foreground(green).Render("\u25CF Connected")
	case matrixConnecting:
		return lipgloss.NewStyle().Foreground(yellow).Render("\u25CC Connecting...")
	case matrixSyncing:
		return lipgloss.NewStyle().Foreground(blue).Render("\u21BB Syncing")
	default:
		return lipgloss.NewStyle().Foreground(red).Render("\u25CB Disconnected")
	}
}

func (mx Matrix) renderLoginForm(width, maxHeight int) string {
	var b strings.Builder

	b.WriteString("\n")
	headerStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString(headerStyle.Render("  Matrix Login"))
	b.WriteString("\n\n")

	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(dimStyle.Render("  Connect to your Matrix homeserver to chat."))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Your access token will be stored in config."))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		value string
		mask  bool
	}{
		{"Homeserver URL", mx.loginServer, false},
		{"Username", mx.loginUser, false},
		{"Password", mx.loginPass, true},
	}

	labelStyle := lipgloss.NewStyle().Foreground(text).Width(16)
	activeLabel := lipgloss.NewStyle().Foreground(mauve).Bold(true).Width(16)
	inputStyle := lipgloss.NewStyle().Foreground(blue)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)

	for i, f := range fields {
		ls := labelStyle
		if i == mx.loginFocus {
			ls = activeLabel
		}

		displayValue := f.value
		if f.mask && displayValue != "" {
			displayValue = strings.Repeat("*", len(displayValue))
		}

		cursor := ""
		if i == mx.loginFocus {
			cursor = cursorStyle.Render("\u2502")
		}

		b.WriteString("  " + ls.Render(f.label+":") + " " + inputStyle.Render(displayValue) + cursor)
		b.WriteString("\n")
	}

	if mx.loginError != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  Error: " + mx.loginError))
	}

	if mx.connState == matrixConnecting {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  Connecting..."))
	}

	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("  Enter: login  Tab: next field  Esc: close"))

	return b.String()
}

func (mx Matrix) renderPrivacyPanel(width, maxHeight int) string {
	var b strings.Builder

	b.WriteString("\n")
	headerStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString(headerStyle.Render("  Privacy Controls"))
	b.WriteString("\n\n")

	items := []struct {
		key   string
		label string
		value bool
		desc  string
	}{
		{"1", "Send Read Receipts", mx.sendReadReceipts, "Let others know you've read messages"},
		{"2", "Typing Indicators", mx.sendTypingIndicators, "Show when you're typing"},
		{"3", "Auto-delete Local Cache", mx.autoDeleteCache, "Clear messages when closing overlay"},
	}

	for _, item := range items {
		toggle := lipgloss.NewStyle().Foreground(red).Render("\u25CB OFF")
		if item.value {
			toggle = lipgloss.NewStyle().Foreground(green).Render("\u25CF ON")
		}

		keyStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		labelStyle := lipgloss.NewStyle().Foreground(text)
		descStyle := lipgloss.NewStyle().Foreground(overlay0)

		b.WriteString("  " + keyStyle.Render("["+item.key+"]") + " " + labelStyle.Render(item.label) + "  " + toggle)
		b.WriteString("\n")
		b.WriteString("      " + descStyle.Render(item.desc))
		b.WriteString("\n\n")
	}

	// E2EE status for current room
	rooms := mx.filteredRooms()
	if mx.roomCursor < len(rooms) {
		room := rooms[mx.roomCursor]
		e2eeIcon := lipgloss.NewStyle().Foreground(red).Render("Unencrypted")
		if room.Encrypted {
			e2eeIcon = lipgloss.NewStyle().Foreground(green).Render("E2EE Enabled")
		}
		b.WriteString("  " + lipgloss.NewStyle().Foreground(text).Render("Room encryption: ") + e2eeIcon)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  Esc: back"))

	return b.String()
}

func (mx Matrix) renderNewDMForm(width int) string {
	var b strings.Builder

	b.WriteString("\n")
	headerStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString(headerStyle.Render("  New Direct Message"))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(text)
	inputStyle := lipgloss.NewStyle().Foreground(blue)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)

	b.WriteString("  " + labelStyle.Render("Username: ") + inputStyle.Render(mx.newDMInput) + cursorStyle.Render("\u2502"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  Enter @user:server  |  Enter: start DM  |  Esc: cancel"))

	return b.String()
}

func (mx Matrix) renderMainView(width, maxHeight int) string {
	// Split: rooms 30%, messages 70%
	roomWidth := width * 3 / 10
	if roomWidth < 18 {
		roomWidth = 18
	}
	msgWidth := width - roomWidth - 3 // 3 for separator

	roomPanel := mx.renderRoomList(roomWidth, maxHeight)
	msgPanel := mx.renderMessagePanel(msgWidth, maxHeight)

	// Join panels side by side
	separator := lipgloss.NewStyle().Foreground(surface1).Render("\u2502")
	roomLines := strings.Split(roomPanel, "\n")
	msgLines := strings.Split(msgPanel, "\n")

	// Normalize heights
	maxLines := maxHeight
	for len(roomLines) < maxLines {
		roomLines = append(roomLines, strings.Repeat(" ", roomWidth))
	}
	for len(msgLines) < maxLines {
		msgLines = append(msgLines, strings.Repeat(" ", msgWidth))
	}

	var b strings.Builder
	lineCount := maxLines
	if lineCount > len(roomLines) {
		lineCount = len(roomLines)
	}
	if lineCount > len(msgLines) {
		lineCount = len(msgLines)
	}

	for i := 0; i < lineCount; i++ {
		rLine := roomLines[i]
		mLine := ""
		if i < len(msgLines) {
			mLine = msgLines[i]
		}
		// Pad room line to exact width
		rLineW := lipgloss.Width(rLine)
		if rLineW < roomWidth {
			rLine += strings.Repeat(" ", roomWidth-rLineW)
		}
		b.WriteString(rLine + " " + separator + " " + mLine)
		if i < lineCount-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (mx Matrix) renderRoomList(width, height int) string {
	var b strings.Builder

	focusStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimFocus := lipgloss.NewStyle().Foreground(overlay0)

	header := "Rooms"
	if mx.focus == matrixFocusRooms {
		b.WriteString(focusStyle.Render(" " + header))
	} else {
		b.WriteString(dimFocus.Render(" " + header))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", width)))
	b.WriteString("\n")

	// Search bar
	if mx.searching {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("/")
		b.WriteString(" " + prompt + lipgloss.NewStyle().Foreground(blue).Render(mx.roomSearch) +
			lipgloss.NewStyle().Foreground(mauve).Render("\u2502"))
		b.WriteString("\n")
	}

	rooms := mx.filteredRooms()
	if len(rooms) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(" No rooms"))
		return b.String()
	}

	listHeight := height - 5 // header + separator + action bar
	if mx.searching {
		listHeight--
	}
	if listHeight < 3 {
		listHeight = 3
	}

	start := mx.roomScroll
	if mx.roomCursor >= start+listHeight {
		start = mx.roomCursor - listHeight + 1
	}
	if mx.roomCursor < start {
		start = mx.roomCursor
	}
	end := start + listHeight
	if end > len(rooms) {
		end = len(rooms)
	}

	for i := start; i < end; i++ {
		room := rooms[i]
		name := room.Name
		if len(name) > width-6 {
			name = name[:width-9] + "..."
		}

		prefix := "  "
		if i == mx.roomCursor {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
		}

		// Pin indicator
		pinIcon := ""
		if room.Pinned {
			if IconFileChar == "~" { // ascii mode
				pinIcon = lipgloss.NewStyle().Foreground(yellow).Render("* ")
			} else {
				pinIcon = lipgloss.NewStyle().Foreground(yellow).Render("\U0001F4CC ")
			}
		}

		nameStyle := lipgloss.NewStyle().Foreground(text)
		if i == mx.roomCursor && mx.focus == matrixFocusRooms {
			nameStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
		}

		unreadBadge := ""
		if room.UnreadCount > 0 {
			unreadBadge = " " + lipgloss.NewStyle().
				Foreground(crust).
				Background(peach).
				Bold(true).
				Render(fmt.Sprintf(" %d ", room.UnreadCount))
		}

		encIcon := ""
		if room.Encrypted {
			encIcon = lipgloss.NewStyle().Foreground(green).Render(" E")
		}

		b.WriteString(prefix + pinIcon + nameStyle.Render(name) + unreadBadge + encIcon)
		b.WriteString("\n")
	}

	// Room action bar
	for i := len(rooms); i < start+listHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", width)))
	b.WriteString("\n")
	actionStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(actionStyle.Render(" [s]hare [P]in [p]rivacy"))

	return b.String()
}

func (mx Matrix) renderMessagePanel(width, height int) string {
	var b strings.Builder

	rooms := mx.filteredRooms()
	if mx.roomCursor >= len(rooms) {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(" Select a room"))
		return b.String()
	}

	room := rooms[mx.roomCursor]

	// Room name header
	focusStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimFocus := lipgloss.NewStyle().Foreground(overlay0)

	roomName := room.Name
	if len(roomName) > width-2 {
		roomName = roomName[:width-5] + "..."
	}
	if mx.focus == matrixFocusMessages || mx.focus == matrixFocusInput {
		b.WriteString(focusStyle.Render(" " + roomName))
	} else {
		b.WriteString(dimFocus.Render(" " + roomName))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", width)))
	b.WriteString("\n")

	// Message search bar
	if mx.msgSearchMode {
		searchPrompt := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("Search: ")
		searchText := lipgloss.NewStyle().Foreground(blue).Render(mx.msgSearchQuery)
		searchCursor := lipgloss.NewStyle().Foreground(mauve).Render("\u2502")
		matchCount := ""
		if len(mx.msgSearchMatches) > 0 {
			matchCount = lipgloss.NewStyle().Foreground(overlay0).Render(
				fmt.Sprintf(" (%d/%d)", mx.msgSearchIdx+1, len(mx.msgSearchMatches)))
		}
		b.WriteString(" " + searchPrompt + searchText + searchCursor + matchCount)
		b.WriteString("\n")
	}

	// Reaction picker
	if mx.showReactionPicker {
		pickerBg := lipgloss.NewStyle().Background(surface0).Padding(0, 1)
		var emojis []string
		for i, emoji := range matrixReactionEmojis {
			if i == mx.reactionCursor {
				emojis = append(emojis, lipgloss.NewStyle().Background(mauve).Foreground(crust).Render(" "+emoji+" "))
			} else {
				emojis = append(emojis, " "+emoji+" ")
			}
		}
		b.WriteString(pickerBg.Render("React: " + strings.Join(emojis, "")))
		b.WriteString("\n")
	}

	// Messages area
	msgs := mx.messages[room.ID]
	inputHeight := 3 // separator + input + e2ee line
	if mx.replyToEvent != "" {
		inputHeight++ // reply preview line
	}
	msgHeight := height - 4 - inputHeight
	if mx.msgSearchMode {
		msgHeight--
	}
	if mx.showReactionPicker {
		msgHeight--
	}
	if msgHeight < 3 {
		msgHeight = 3
	}

	if len(msgs) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Italic(true).Render(" No messages yet"))
		b.WriteString("\n")
		for i := 1; i < msgHeight; i++ {
			b.WriteString("\n")
		}
	} else {
		// Render messages from bottom up
		var renderedLines []string
		for i, msg := range msgs {
			line := mx.renderMessage(msg, width, i)
			renderedLines = append(renderedLines, line)

			// Render reactions under messages
			if len(msg.Reactions) > 0 {
				reactionLine := mx.renderReactions(msg.Reactions)
				renderedLines = append(renderedLines, reactionLine)
			}
		}

		// Apply scroll
		totalLines := len(renderedLines)
		visibleEnd := totalLines - mx.messageScroll
		if visibleEnd < 0 {
			visibleEnd = 0
		}
		visibleStart := visibleEnd - msgHeight
		if visibleStart < 0 {
			visibleStart = 0
		}

		linesWritten := 0
		for i := visibleStart; i < visibleEnd && i < totalLines; i++ {
			b.WriteString(renderedLines[i])
			b.WriteString("\n")
			linesWritten++
		}
		for i := linesWritten; i < msgHeight; i++ {
			b.WriteString("\n")
		}
	}

	// Input area
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2500", width)))
	b.WriteString("\n")

	// Reply preview
	if mx.replyToEvent != "" {
		replyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString(replyStyle.Render(" \u2502 Replying to: " + mx.replyPreview))
		b.WriteString("\n")
	}

	inputPrompt := lipgloss.NewStyle().Foreground(overlay0).Render(" > ")
	inputText := mx.messageInput
	cursor := ""
	if mx.focus == matrixFocusInput {
		cursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("\u2502")
		inputPrompt = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" > ")
	}

	maxInputLen := width - 6
	if len(inputText) > maxInputLen {
		inputText = inputText[len(inputText)-maxInputLen:]
	}

	b.WriteString(inputPrompt + lipgloss.NewStyle().Foreground(text).Render(inputText) + cursor)
	b.WriteString("\n")

	// E2EE status + message mode hints
	e2eeLabel := lipgloss.NewStyle().Foreground(overlay0).Render(" Unencrypted")
	if room.Encrypted {
		e2eeLabel = lipgloss.NewStyle().Foreground(green).Render(" E2EE Ready")
	}
	modeHint := ""
	if mx.focus == matrixFocusMessages {
		if mx.messageSelect {
			modeHint = lipgloss.NewStyle().Foreground(yellow).Render("  [SELECT] c:capture C:thread r:react R:reply")
		} else {
			modeHint = lipgloss.NewStyle().Foreground(overlay0).Render("  v:select /:search")
		}
	}
	b.WriteString(e2eeLabel + modeHint)

	return b.String()
}

func (mx Matrix) renderMessage(msg matrixChatMessage, maxWidth int, idx int) string {
	timeStr := msg.Timestamp.Format("15:04")
	timeStyle := lipgloss.NewStyle().Foreground(overlay0)
	senderStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(text)

	if msg.IsOwn {
		senderStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	}

	// Highlight if selected
	if mx.messageSelect && idx == mx.messageCursor {
		bodyStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)
		senderStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	}

	// Highlight search matches
	if mx.msgSearchMode && mx.msgSearchQuery != "" {
		for _, matchIdx := range mx.msgSearchMatches {
			if matchIdx == idx {
				bodyStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)
				break
			}
		}
	}

	sender := msg.Sender
	if len(sender) > 12 {
		sender = sender[:12]
	}

	body := msg.Body
	// Reply indicator
	replyPrefix := ""
	if msg.ReplyTo != "" {
		replyPrefix = lipgloss.NewStyle().Foreground(overlay0).Render("\u2502 ") // vertical bar for reply
	}

	// Truncate long messages
	availW := maxWidth - len(timeStr) - len(sender) - 8
	if msg.ReplyTo != "" {
		availW -= 2
	}
	if availW < 10 {
		availW = 10
	}
	if len(body) > availW {
		body = body[:availW-3] + "..."
	}
	// Replace newlines with spaces for single-line display
	body = strings.ReplaceAll(body, "\n", " ")

	selectMarker := " "
	if mx.messageSelect && idx == mx.messageCursor {
		selectMarker = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(">")
	}

	return selectMarker + replyPrefix + timeStyle.Render("["+timeStr+"]") + " " +
		senderStyle.Render(sender+":") + " " +
		bodyStyle.Render(body)
}

func (mx Matrix) renderReactions(reactions map[string]int) string {
	if len(reactions) == 0 {
		return ""
	}
	var parts []string
	for emoji, count := range reactions {
		pill := lipgloss.NewStyle().
			Background(surface0).
			Foreground(text).
			Padding(0, 0).
			Render(fmt.Sprintf(" %s %d ", emoji, count))
		parts = append(parts, pill)
	}
	return "   " + strings.Join(parts, " ")
}
