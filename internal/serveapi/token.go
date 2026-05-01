package serveapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateToken reads <vault>/.granit/everything-token if present, or
// generates a fresh 32-byte token, writes it (chmod 600), and returns it.
func LoadOrCreateToken(vaultRoot string) (string, error) {
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir .granit: %w", err)
	}
	path := filepath.Join(dir, "everything-token")
	if data, err := os.ReadFile(path); err == nil {
		tok := strings.TrimSpace(string(data))
		if len(tok) >= 16 {
			return tok, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read token: %w", err)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	tok := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write token: %w", err)
	}
	return tok, nil
}
