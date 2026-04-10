package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEncryption_RoundTrip(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("my-secret-passphrase")

	plaintext := "This is a secret note.\nWith multiple lines.\n"

	encrypted, err := e.EncryptContent(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == "" {
		t.Fatal("expected non-empty ciphertext")
	}
	if encrypted == plaintext {
		t.Error("encrypted should not equal plaintext")
	}

	decrypted, err := e.DecryptContent(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("round-trip failed: got %q", decrypted)
	}
}

func TestEncryption_WrongPassphrase(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("correct-passphrase")

	encrypted, err := e.EncryptContent("secret data")
	if err != nil {
		t.Fatal(err)
	}

	e.SetPassphrase("wrong-passphrase")
	_, err = e.DecryptContent(encrypted)
	if err == nil {
		t.Error("decryption with wrong passphrase should fail")
	}
}

func TestEncryption_NoPassphrase(t *testing.T) {
	e := NewEncryption()

	_, err := e.EncryptContent("test")
	if err == nil {
		t.Error("encrypt without passphrase should fail")
	}

	_, err = e.DecryptContent("test")
	if err == nil {
		t.Error("decrypt without passphrase should fail")
	}
}

func TestEncryption_InvalidBase64(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("passphrase")

	_, err := e.DecryptContent("not-valid-base64!!!")
	if err == nil {
		t.Error("invalid base64 should fail")
	}
}

func TestEncryption_TooShortCiphertext(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("passphrase")

	_, err := e.DecryptContent("AQID") // very short base64
	if err == nil {
		t.Error("too-short ciphertext should fail")
	}
}

func TestEncryption_UniquePerEncrypt(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("passphrase")

	enc1, _ := e.EncryptContent("same text")
	enc2, _ := e.EncryptContent("same text")

	if enc1 == enc2 {
		t.Error("same plaintext should produce different ciphertexts (random salt/nonce)")
	}
}

func TestEncryption_IsEncrypted(t *testing.T) {
	e := NewEncryption()

	if !e.IsEncrypted("note.md.enc") {
		t.Error("note.md.enc should be encrypted")
	}
	if e.IsEncrypted("note.md") {
		t.Error("note.md should not be encrypted")
	}
	if e.IsEncrypted("note.enc") {
		t.Error("note.enc should not be encrypted (no .md)")
	}
}

func TestEncryption_EncryptedName(t *testing.T) {
	e := NewEncryption()

	if got := e.EncryptedName("note.md"); got != "note.md.enc" {
		t.Errorf("expected note.md.enc, got %q", got)
	}
	// Already encrypted — should not double-encode
	if got := e.EncryptedName("note.md.enc"); got != "note.md.enc" {
		t.Errorf("should not double-encode, got %q", got)
	}
}

func TestEncryption_DecryptedName(t *testing.T) {
	e := NewEncryption()

	if got := e.DecryptedName("note.md.enc"); got != "note.md" {
		t.Errorf("expected note.md, got %q", got)
	}
	// Not encrypted — return as-is
	if got := e.DecryptedName("note.md"); got != "note.md" {
		t.Errorf("should return unchanged, got %q", got)
	}
}

func TestEncryption_EmptyContent(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("passphrase")

	encrypted, err := e.EncryptContent("")
	if err != nil {
		t.Fatalf("encrypting empty should work: %v", err)
	}

	decrypted, err := e.DecryptContent(encrypted)
	if err != nil {
		t.Fatalf("decrypting empty should work: %v", err)
	}
	if decrypted != "" {
		t.Errorf("expected empty, got %q", decrypted)
	}
}

// Regression: a passphrase containing multi-byte runes (accented chars,
// emoji, CJK) used to be silently rejected by the input handler because
// the default case only accepted single-byte ASCII. Now the handler reads
// from msg.Runes so any printable rune is accepted.
func TestEncryption_PassphraseInputAcceptsMultibyte(t *testing.T) {
	e := NewEncryption()
	e.active = true
	e.mode = encModeEnterPassphrase

	// Simulate the user typing a 3-rune multi-byte passphrase.
	for _, r := range []rune{'c', 'a', 'f', 'é', '🔥'} {
		e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if e.input != "café🔥" {
		t.Errorf("expected input %q, got %q", "café🔥", e.input)
	}
}

// Regression: special-key strings like "esc"/"enter" must NOT leak into
// the passphrase input as if they were three printable characters.
func TestEncryption_PassphraseInputIgnoresSpecialKeyNames(t *testing.T) {
	e := NewEncryption()
	e.active = true
	e.mode = encModeEnterPassphrase

	// "tab" is a special key with no Runes; bubbletea would emit it with an
	// empty Runes slice. Our handler must not append "t","a","b".
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyTab, Runes: nil})
	if e.input != "" {
		t.Errorf("special key leaked into input: %q", e.input)
	}
}

// Regression: a passphrase set with multi-byte runes must round-trip
// through encrypt/decrypt without loss.
func TestEncryption_MultibytePassphraseRoundTrip(t *testing.T) {
	e := NewEncryption()
	e.SetPassphrase("café 🔥 naïve") // multi-byte passphrase

	plaintext := "secret message"
	ciphertext, err := e.EncryptContent(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Use a fresh decryptor with the same passphrase.
	d := NewEncryption()
	d.SetPassphrase("café 🔥 naïve")
	got, err := d.DecryptContent(ciphertext)
	if err != nil {
		t.Fatalf("decrypt with same multi-byte passphrase failed: %v", err)
	}
	if got != plaintext {
		t.Errorf("plaintext lost: got %q, want %q", got, plaintext)
	}
}
