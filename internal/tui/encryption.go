package tui

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	encModeMenu             = 0
	encModeEnterPassphrase  = 1
	encModeConfirmPassphrase = 2

	encActionNone    = 0
	encActionEncrypt = 1
	encActionDecrypt = 2
	encActionSetKey  = 3

	encSaltLen       = 16
	encNonceLen      = 12
	encKeyLen        = 32 // AES-256
	encPBKDFIter     = 100000
)

// ---------------------------------------------------------------------------
// Result type — consumed by app.go after encrypt/decrypt
// ---------------------------------------------------------------------------

// EncryptionResult carries the outcome of an encrypt or decrypt operation back
// to the main application loop.
type EncryptionResult struct {
	Action  int    // encActionEncrypt or encActionDecrypt
	Content string // encrypted (base64) or decrypted (plaintext) content
}

// ---------------------------------------------------------------------------
// Encryption overlay
// ---------------------------------------------------------------------------

// Encryption provides a session-scoped note encryption manager with an
// interactive TUI overlay for passphrase entry and menu selection.
// Crypto: AES-256-GCM with a PBKDF2-like iterated SHA-256 key derivation.
// No external (x/crypto) dependencies.
type Encryption struct {
	active bool
	width  int
	height int

	// Session key cache — never written to disk.
	passphrase string
	hasKey     bool

	// UI state
	mode    int    // encModeMenu / encModeEnterPassphrase / encModeConfirmPassphrase
	input   string // passphrase input buffer
	confirm string // confirmation buffer
	cursor  int    // menu cursor
	message string // status / error message

	// Action to perform after passphrase entry
	pendingAction int // encActionNone .. encActionSetKey

	// Result available for app.go to consume
	result      EncryptionResult
	resultReady bool
}

// menu entries shown in the main menu view.
var encMenuItems = []struct {
	label string
	desc  string
}{
	{"Encrypt Current Note", "Encrypt and save as .md.enc"},
	{"Decrypt Current Note", "Decrypt a .md.enc file back to .md"},
	{"Set Passphrase", "Set or change the session passphrase"},
	{"Lock Vault", "Clear the cached passphrase from memory"},
}

// NewEncryption creates a new Encryption overlay in its default state.
func NewEncryption() Encryption {
	return Encryption{}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// IsActive reports whether the overlay is currently visible.
func (e *Encryption) IsActive() bool {
	return e.active
}

// Open makes the encryption overlay visible and resets transient UI state.
func (e *Encryption) Open() {
	e.active = true
	e.mode = encModeMenu
	e.cursor = 0
	e.input = ""
	e.confirm = ""
	e.message = ""
	e.pendingAction = encActionNone
	e.resultReady = false
}

// Close hides the overlay.
func (e *Encryption) Close() {
	e.active = false
	e.input = ""
	e.confirm = ""
}

// SetSize updates the available terminal dimensions.
func (e *Encryption) SetSize(w, h int) {
	e.width = w
	e.height = h
}

// HasKey reports whether a passphrase has been set for this session.
func (e *Encryption) HasKey() bool {
	return e.hasKey
}

// SetPassphrase sets the session passphrase (kept in memory only).
func (e *Encryption) SetPassphrase(passphrase string) {
	e.passphrase = passphrase
	e.hasKey = passphrase != ""
}

// GetResult returns the pending result and clears it. The bool indicates
// whether a result was available.
func (e *Encryption) GetResult() (EncryptionResult, bool) {
	if !e.resultReady {
		return EncryptionResult{}, false
	}
	r := e.result
	e.result = EncryptionResult{}
	e.resultReady = false
	return r, true
}

// ---------------------------------------------------------------------------
// Crypto helpers  (stdlib only — no x/crypto)
// ---------------------------------------------------------------------------

// deriveKey produces a 32-byte key from passphrase + salt using iterated
// HMAC-SHA256 (a simplified PBKDF2 variant that avoids x/crypto).
func deriveKey(passphrase string, salt []byte) []byte {
	// PBKDF2-HMAC-SHA256, single block (dk_len <= hash_len).
	// U_1 = PRF(password, salt || INT_32_BE(1))
	// U_i = PRF(password, U_{i-1})
	// dk  = U_1 ^ U_2 ^ ... ^ U_c
	prf := hmac.New(sha256.New, []byte(passphrase))

	// salt || blockIndex (big-endian 1)
	input := make([]byte, len(salt)+4)
	copy(input, salt)
	input[len(salt)+0] = 0
	input[len(salt)+1] = 0
	input[len(salt)+2] = 0
	input[len(salt)+3] = 1

	prf.Write(input)
	u := prf.Sum(nil) // U_1

	dk := make([]byte, len(u))
	copy(dk, u)

	for i := 1; i < encPBKDFIter; i++ {
		prf.Reset()
		prf.Write(u)
		u = prf.Sum(nil)
		for j := range dk {
			dk[j] ^= u[j]
		}
	}

	return dk[:encKeyLen]
}

// EncryptContent encrypts plaintext using AES-256-GCM and returns a base64
// string of (salt[16] || nonce[12] || ciphertext[...]).
func (e *Encryption) EncryptContent(plaintext string) (string, error) {
	if !e.hasKey || e.passphrase == "" {
		return "", errors.New("no passphrase set — set one first")
	}

	// Generate random salt.
	salt := make([]byte, encSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	key := deriveKey(e.passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	// Generate random nonce.
	nonce := make([]byte, encNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Assemble: salt + nonce + ciphertext
	blob := make([]byte, 0, encSaltLen+encNonceLen+len(ciphertext))
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)

	return base64.StdEncoding.EncodeToString(blob), nil
}

// DecryptContent decodes a base64 string and decrypts the AES-256-GCM payload
// back to plaintext.
func (e *Encryption) DecryptContent(encoded string) (string, error) {
	if !e.hasKey || e.passphrase == "" {
		return "", errors.New("no passphrase set — set one first")
	}

	blob, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("invalid base64: %w", err)
	}

	minLen := encSaltLen + encNonceLen + 1
	if len(blob) < minLen {
		return "", errors.New("ciphertext too short")
	}

	salt := blob[:encSaltLen]
	nonce := blob[encSaltLen : encSaltLen+encNonceLen]
	ciphertext := blob[encSaltLen+encNonceLen:]

	key := deriveKey(e.passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("decryption failed — wrong passphrase or corrupted data")
	}

	return string(plaintext), nil
}

// ---------------------------------------------------------------------------
// Filename helpers
// ---------------------------------------------------------------------------

// IsEncrypted reports whether filename ends with ".md.enc".
func (e *Encryption) IsEncrypted(filename string) bool {
	return strings.HasSuffix(filename, ".md.enc")
}

// EncryptedName converts "note.md" to "note.md.enc".
func (e *Encryption) EncryptedName(filename string) string {
	if strings.HasSuffix(filename, ".md.enc") {
		return filename
	}
	if strings.HasSuffix(filename, ".md") {
		return filename + ".enc"
	}
	return filename + ".md.enc"
}

// DecryptedName converts "note.md.enc" to "note.md".
func (e *Encryption) DecryptedName(filename string) string {
	if strings.HasSuffix(filename, ".md.enc") {
		return strings.TrimSuffix(filename, ".enc")
	}
	return filename
}

// ---------------------------------------------------------------------------
// Update  (value receiver — matches project overlay convention)
// ---------------------------------------------------------------------------

// Update processes key events for the encryption overlay.
func (e Encryption) Update(msg tea.Msg) (Encryption, tea.Cmd) {
	if !e.active {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		switch e.mode {
		// ----- Menu mode -----
		case encModeMenu:
			switch key {
			case "esc":
				e.active = false
				e.message = ""
				return e, nil
			case "up", "k":
				if e.cursor > 0 {
					e.cursor--
				}
			case "down", "j":
				if e.cursor < len(encMenuItems)-1 {
					e.cursor++
				}
			case "enter":
				e.message = ""
				switch e.cursor {
				case 0: // Encrypt current note
					if !e.hasKey {
						e.pendingAction = encActionEncrypt
						e.mode = encModeEnterPassphrase
						e.input = ""
						e.confirm = ""
						e.message = "Enter passphrase to encrypt"
					} else {
						e.pendingAction = encActionEncrypt
						e.doAction()
					}
				case 1: // Decrypt current note
					if !e.hasKey {
						e.pendingAction = encActionDecrypt
						e.mode = encModeEnterPassphrase
						e.input = ""
						e.message = "Enter passphrase to decrypt"
					} else {
						e.pendingAction = encActionDecrypt
						e.doAction()
					}
				case 2: // Set passphrase
					e.pendingAction = encActionSetKey
					e.mode = encModeEnterPassphrase
					e.input = ""
					e.confirm = ""
					e.message = "Enter new passphrase"
				case 3: // Lock vault
					e.passphrase = ""
					e.hasKey = false
					e.message = "Passphrase cleared — vault locked"
				}
			}

		// ----- Passphrase entry -----
		case encModeEnterPassphrase:
			switch key {
			case "esc":
				e.mode = encModeMenu
				e.input = ""
				e.confirm = ""
				e.message = ""
				e.pendingAction = encActionNone
			case "enter":
				if len(e.input) == 0 {
					e.message = "Passphrase cannot be empty"
					return e, nil
				}
				if e.pendingAction == encActionSetKey || (!e.hasKey && e.pendingAction == encActionEncrypt) {
					// Need confirmation for new passphrase
					e.mode = encModeConfirmPassphrase
					e.confirm = ""
					e.message = "Confirm passphrase"
				} else {
					// Decrypt or encrypt with existing key flow — just use it
					e.passphrase = e.input
					e.hasKey = true
					e.doAction()
					e.input = ""
				}
			case "backspace":
				if len(e.input) > 0 {
					e.input = e.input[:len(e.input)-1]
				}
			default:
				if len(key) == 1 && key[0] >= 32 {
					e.input += key
				} else if key == "space" {
					e.input += " "
				}
			}

		// ----- Confirm passphrase -----
		case encModeConfirmPassphrase:
			switch key {
			case "esc":
				e.mode = encModeEnterPassphrase
				e.confirm = ""
				e.message = "Enter passphrase"
			case "enter":
				if e.confirm != e.input {
					e.message = "Passphrases do not match — try again"
					e.mode = encModeEnterPassphrase
					e.input = ""
					e.confirm = ""
					return e, nil
				}
				e.passphrase = e.input
				e.hasKey = true
				e.input = ""
				e.confirm = ""

				if e.pendingAction == encActionSetKey {
					e.message = "Passphrase set successfully"
					e.pendingAction = encActionNone
					e.mode = encModeMenu
				} else {
					e.doAction()
				}
			case "backspace":
				if len(e.confirm) > 0 {
					e.confirm = e.confirm[:len(e.confirm)-1]
				}
			default:
				if len(key) == 1 && key[0] >= 32 {
					e.confirm += key
				} else if key == "space" {
					e.confirm += " "
				}
			}
		}
	}

	return e, nil
}

// doAction marks the pending action result as ready so app.go can pick it up.
// The actual file read/write is handled by app.go — we only signal intent.
func (e *Encryption) doAction() {
	e.result = EncryptionResult{
		Action: e.pendingAction,
	}
	e.resultReady = true
	e.pendingAction = encActionNone
	e.mode = encModeMenu
	e.input = ""
	e.confirm = ""

	switch e.result.Action {
	case encActionEncrypt:
		e.message = "Ready to encrypt — processing..."
	case encActionDecrypt:
		e.message = "Ready to decrypt — processing..."
	}
}

// ---------------------------------------------------------------------------
// View  (value receiver — matches project overlay convention)
// ---------------------------------------------------------------------------

// View renders the encryption overlay.
func (e Encryption) View() string {
	width := e.width / 2
	if width < 52 {
		width = 52
	}
	if width > 72 {
		width = 72
	}

	var b strings.Builder

	// Title
	lockIcon := lipgloss.NewStyle().Foreground(mauve).Render(IconEditChar)
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + lockIcon + " Encryption")

	statusText := ""
	if e.hasKey {
		statusText = lipgloss.NewStyle().Foreground(green).Render("  [unlocked]")
	} else {
		statusText = lipgloss.NewStyle().Foreground(red).Render("  [locked]")
	}

	b.WriteString(title + statusText)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	switch e.mode {
	case encModeMenu:
		e.viewMenu(&b, width)
	case encModeEnterPassphrase:
		e.viewPassphraseInput(&b, width, false)
	case encModeConfirmPassphrase:
		e.viewPassphraseInput(&b, width, true)
	}

	// Status message
	if e.message != "" {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
		b.WriteString("\n")

		msgStyle := lipgloss.NewStyle().Foreground(yellow)
		if strings.Contains(e.message, "successfully") || strings.Contains(e.message, "unlocked") {
			msgStyle = lipgloss.NewStyle().Foreground(green)
		} else if strings.Contains(e.message, "fail") || strings.Contains(e.message, "match") || strings.Contains(e.message, "empty") {
			msgStyle = lipgloss.NewStyle().Foreground(red)
		}
		b.WriteString("  " + msgStyle.Render(e.message))
	}

	// Help bar
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	switch e.mode {
	case encModeMenu:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"j/k", "navigate"}, {"Enter", "select"}, {"Esc", "close"},
		}))
	case encModeEnterPassphrase, encModeConfirmPassphrase:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "confirm"}, {"Esc", "back"},
		}))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewMenu renders the main menu items.
func (e Encryption) viewMenu(b *strings.Builder, width int) {
	b.WriteString("\n")

	for i, item := range encMenuItems {
		icon := " "
		if i == 0 {
			icon = lipgloss.NewStyle().Foreground(green).Render("\u25b8") // encrypt
		} else if i == 1 {
			icon = lipgloss.NewStyle().Foreground(blue).Render("\u25b8") // decrypt
		} else if i == 2 {
			icon = lipgloss.NewStyle().Foreground(peach).Render("\u25b8") // set key
		} else if i == 3 {
			icon = lipgloss.NewStyle().Foreground(red).Render("\u25b8") // lock
		}

		if i == e.cursor {
			label := lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(width - 10).
				Render("  " + icon + " " + item.label)
			b.WriteString(label)
		} else {
			label := "  " + icon + " " + lipgloss.NewStyle().Foreground(text).Render(item.label)
			b.WriteString(label)
		}

		// Description below the label
		desc := DimStyle.Render("      " + item.desc)
		b.WriteString("\n" + desc)

		if i < len(encMenuItems)-1 {
			b.WriteString("\n")
		}
	}
}

// viewPassphraseInput renders the passphrase text entry field.
func (e Encryption) viewPassphraseInput(b *strings.Builder, width int, isConfirm bool) {
	b.WriteString("\n")

	promptLabel := "Passphrase"
	inputVal := e.input
	if isConfirm {
		promptLabel = "Confirm"
		inputVal = e.confirm
	}

	prompt := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render("  " + promptLabel + ": ")

	// Render dots for each character
	dots := strings.Repeat("\u25cf", len(inputVal))
	if dots == "" {
		dots = lipgloss.NewStyle().Foreground(overlay0).Render("(type your passphrase)")
	} else {
		dots = lipgloss.NewStyle().Foreground(text).Render(dots)
	}

	// Cursor
	cursor := lipgloss.NewStyle().
		Background(text).
		Foreground(mantle).
		Render(" ")

	b.WriteString(prompt + dots + cursor)
	b.WriteString("\n")

	// Show character count
	charCount := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf("  %d characters", len(inputVal)))
	b.WriteString("\n" + charCount)

	// Strength hint (only when entering, not confirming)
	if !isConfirm && len(inputVal) > 0 {
		strength, strengthColor := passphraseStrength(inputVal)
		strengthLabel := lipgloss.NewStyle().
			Foreground(strengthColor).
			Render("  Strength: " + strength)
		b.WriteString("\n" + strengthLabel)
	}
}

// passphraseStrength returns a label and color for the passphrase quality.
func passphraseStrength(pass string) (string, lipgloss.Color) {
	score := 0

	if len(pass) >= 8 {
		score++
	}
	if len(pass) >= 12 {
		score++
	}
	if len(pass) >= 20 {
		score++
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, c := range pass {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if hasUpper {
		score++
	}
	if hasLower {
		score++
	}
	if hasDigit {
		score++
	}
	if hasSpecial {
		score++
	}

	switch {
	case score <= 2:
		return "weak", red
	case score <= 4:
		return "fair", yellow
	case score <= 5:
		return "good", peach
	default:
		return "strong", green
	}
}
