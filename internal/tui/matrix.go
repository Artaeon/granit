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
}

type matrixChatMessage struct {
	Sender    string
	Body      string
	Timestamp time.Time
	EventID   string
	IsOwn     bool
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

	// Messages
	messages      map[string][]matrixChatMessage // roomID -> messages
	messageScroll int
	messageInput  string

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
}

func NewMatrix() Matrix {
	return Matrix{
		messages:   make(map[string][]matrixChatMessage),
		e2eeStatus: make(map[string]bool),
		connState:  matrixDisconnected,
		focus:      matrixFocusLogin,
		autoDeleteCache: true,
	}
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
			if ev.Type != "m.room.message" {
				continue
			}
			body, _ := ev.Content["body"].(string)
			if body == "" {
				continue
			}
			ts := time.Unix(0, ev.OriginServerTS*int64(time.Millisecond))
			messages = append(messages, matrixChatMessage{
				Sender:    extractDisplayName(ev.Sender),
				Body:      body,
				Timestamp: ts,
				EventID:   ev.EventID,
			})
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
			// Refresh messages for the room
			return mx, matrixFetchMessages(mx.homeserver, mx.accessToken, msg.roomID, 50)
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

	case tea.KeyMsg:
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
			if ev.Type != "m.room.message" {
				continue
			}
			body, _ := ev.Content["body"].(string)
			if body == "" {
				continue
			}
			ts := time.Unix(0, ev.OriginServerTS*int64(time.Millisecond))
			msg := matrixChatMessage{
				Sender:    extractDisplayName(ev.Sender),
				Body:      body,
				Timestamp: ts,
				EventID:   ev.EventID,
				IsOwn:     ev.Sender == mx.userID,
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
		}
	}
}

func (mx Matrix) handleKeyMsg(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "esc":
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
		mx.Close()
		return mx, nil

	case "ctrl+m":
		mx.Close()
		return mx, nil

	case "tab":
		if mx.focus == matrixFocusLogin {
			return mx, nil
		}
		if mx.showPrivacy {
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

	// Room search
	if mx.searching {
		return mx.handleSearchKeys(msg)
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
		// Share current note to selected room
		if mx.shareContent != "" && mx.roomCursor < len(filtered) {
			roomID := filtered[mx.roomCursor].ID
			mx.txnCounter++
			shareText := "Shared note from Granit:\n\n" + mx.shareContent
			if len(shareText) > 4000 {
				shareText = shareText[:4000] + "\n\n[truncated]"
			}
			return mx, matrixSendMessage(mx.homeserver, mx.accessToken, roomID, shareText, mx.txnCounter)
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
	case "n":
		mx.focus = matrixFocusNewDM
		mx.newDMInput = ""
	case "R":
		// Refresh rooms
		return mx, matrixFetchRooms(mx.homeserver, mx.accessToken)
	}
	return mx, nil
}

func (mx Matrix) handleMessageKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+d":
		mx.messageScroll += 10
	case "ctrl+u":
		mx.messageScroll -= 10
		if mx.messageScroll < 0 {
			mx.messageScroll = 0
		}
	case "j", "down":
		mx.messageScroll++
	case "k", "up":
		if mx.messageScroll > 0 {
			mx.messageScroll--
		}
	case "G":
		mx.messageScroll = 0 // scroll to bottom (0 = bottom since we render from bottom)
	case "i", "enter":
		mx.focus = matrixFocusInput
	}
	return mx, nil
}

func (mx Matrix) handleInputKeys(msg tea.KeyMsg) (Matrix, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		if mx.messageInput != "" {
			filtered := mx.filteredRooms()
			if mx.roomCursor < len(filtered) {
				roomID := filtered[mx.roomCursor].ID
				mx.txnCounter++
				input := mx.messageInput
				mx.messageInput = ""
				var cmds []tea.Cmd
				cmds = append(cmds, matrixSendMessage(mx.homeserver, mx.accessToken, roomID, input, mx.txnCounter))
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
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
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
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	if mx.statusMsg != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  " + mx.statusMsg))
	} else if mx.accessToken != "" {
		hints := "  Tab: switch  /: search  s: share note  p: privacy  n: new DM  R: refresh  Esc: close"
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(hints))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 1).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (mx Matrix) connectionBadge() string {
	switch mx.connState {
	case matrixConnected:
		return lipgloss.NewStyle().Foreground(green).Render("● Connected")
	case matrixConnecting:
		return lipgloss.NewStyle().Foreground(yellow).Render("◌ Connecting...")
	case matrixSyncing:
		return lipgloss.NewStyle().Foreground(blue).Render("↻ Syncing")
	default:
		return lipgloss.NewStyle().Foreground(red).Render("○ Disconnected")
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
			cursor = cursorStyle.Render("│")
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
		toggle := lipgloss.NewStyle().Foreground(red).Render("○ OFF")
		if item.value {
			toggle = lipgloss.NewStyle().Foreground(green).Render("● ON")
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

	b.WriteString("  " + labelStyle.Render("Username: ") + inputStyle.Render(mx.newDMInput) + cursorStyle.Render("│"))
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
	separator := lipgloss.NewStyle().Foreground(surface1).Render("│")
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
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Search bar
	if mx.searching {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("/")
		b.WriteString(" " + prompt + lipgloss.NewStyle().Foreground(blue).Render(mx.roomSearch) +
			lipgloss.NewStyle().Foreground(mauve).Render("│"))
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
		if len(name) > width-4 {
			name = name[:width-7] + "..."
		}

		prefix := "  "
		if i == mx.roomCursor {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
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

		b.WriteString(prefix + nameStyle.Render(name) + unreadBadge + encIcon)
		b.WriteString("\n")
	}

	// Room action bar
	for i := len(rooms); i < start+listHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	actionStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(actionStyle.Render(" [s]hare [p]rivacy"))

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
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Messages area
	msgs := mx.messages[room.ID]
	inputHeight := 3 // separator + input + e2ee line
	msgHeight := height - 4 - inputHeight
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
		for _, msg := range msgs {
			line := mx.renderMessage(msg, width)
			renderedLines = append(renderedLines, line)
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
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	inputPrompt := lipgloss.NewStyle().Foreground(overlay0).Render(" > ")
	inputText := mx.messageInput
	cursor := ""
	if mx.focus == matrixFocusInput {
		cursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("│")
		inputPrompt = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" > ")
	}

	maxInputLen := width - 6
	if len(inputText) > maxInputLen {
		inputText = inputText[len(inputText)-maxInputLen:]
	}

	b.WriteString(inputPrompt + lipgloss.NewStyle().Foreground(text).Render(inputText) + cursor)
	b.WriteString("\n")

	// E2EE status
	e2eeLabel := lipgloss.NewStyle().Foreground(overlay0).Render(" Unencrypted")
	if room.Encrypted {
		e2eeLabel = lipgloss.NewStyle().Foreground(green).Render(" E2EE Ready")
	}
	b.WriteString(e2eeLabel)

	return b.String()
}

func (mx Matrix) renderMessage(msg matrixChatMessage, maxWidth int) string {
	timeStr := msg.Timestamp.Format("15:04")
	timeStyle := lipgloss.NewStyle().Foreground(overlay0)
	senderStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(text)

	if msg.IsOwn {
		senderStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	}

	sender := msg.Sender
	if len(sender) > 12 {
		sender = sender[:12]
	}

	body := msg.Body
	// Truncate long messages
	availW := maxWidth - len(timeStr) - len(sender) - 8
	if availW < 10 {
		availW = 10
	}
	if len(body) > availW {
		body = body[:availW-3] + "..."
	}
	// Replace newlines with spaces for single-line display
	body = strings.ReplaceAll(body, "\n", " ")

	return " " + timeStyle.Render("["+timeStr+"]") + " " +
		senderStyle.Render(sender+":") + " " +
		bodyStyle.Render(body)
}
