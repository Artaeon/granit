package serveapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"golang.org/x/crypto/argon2"
)

// authStore is the on-disk schema at <vault>/.granit/web-auth.json.
//
// We keep the password hash + active sessions in one file so a single
// atomic write covers both. Sessions persist across server restarts so
// users don't have to re-login every time the binary cycles. Last-used
// timestamps drive the expiry sweep in maybeExpireSessions.
type authStore struct {
	// PasswordHash is the encoded argon2id digest, "argon2id$v=19$m=N,t=N,p=N$<salt>$<hash>".
	// Empty when no password has been set yet — first launch.
	PasswordHash string `json:"password_hash"`

	// Sessions tracks every long-lived session token a successful login
	// has handed out. We store hashes (not the tokens themselves) so a
	// stolen file alone can't impersonate the user — the attacker would
	// also need the original token from the client's localStorage.
	Sessions []sessionRecord `json:"sessions"`

	// SetupAt timestamps the first password setup so the UI can show
	// "Account created on …" and the audit log is intact.
	SetupAt time.Time `json:"setup_at,omitempty"`
}

type sessionRecord struct {
	// TokenHash is sha256(token) hex-encoded. Tokens themselves never
	// touch disk after being handed back to the client.
	TokenHash string    `json:"token_hash"`
	Label     string    `json:"label,omitempty"`     // user-supplied (e.g. "iPhone")
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

// sessionTTL is how long an inactive session lives before being swept.
// 60 days is long enough that the user shouldn't get bumped out daily;
// short enough that an abandoned device doesn't have indefinite access.
const sessionTTL = 60 * 24 * time.Hour

// authState wraps the file-backed store plus an in-memory cache of
// known-good tokens (looked up on every authed request — must be fast).
type authState struct {
	mu       sync.RWMutex
	path     string
	store    authStore
	tokenSet map[string]time.Time // token (raw) → last seen; rebuilt only at login
}

func newAuthState(vaultRoot string) (*authState, error) {
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	a := &authState{
		path:     filepath.Join(dir, "web-auth.json"),
		tokenSet: map[string]time.Time{},
	}
	if err := a.load(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *authState) load() error {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // empty store
		}
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return json.Unmarshal(data, &a.store)
}

func (a *authState) save() error {
	a.mu.RLock()
	data, err := json.MarshalIndent(a.store, "", "  ")
	a.mu.RUnlock()
	if err != nil {
		return err
	}
	return atomicio.WriteState(a.path, data)
}

// HasPassword answers "is setup complete" — drives the UI's setup-vs-login fork.
func (a *authState) HasPassword() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store.PasswordHash != ""
}

// SetPassword stores a new password hash. Used both on first setup and
// when the user changes their password later. Wipes existing sessions
// when a previous hash exists — change-of-password should kick old
// devices off (security expectation).
func (a *authState) SetPassword(password string, isInitial bool) error {
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if !isInitial && a.store.PasswordHash != "" {
		// Password change → invalidate all existing sessions.
		a.store.Sessions = nil
	}
	a.store.PasswordHash = hash
	if a.store.SetupAt.IsZero() {
		a.store.SetupAt = time.Now()
	}
	a.mu.Unlock()
	return a.save()
}

// VerifyPassword runs the argon2id comparison in constant time relative
// to the input — variable time relative to the algorithm parameters.
func (a *authState) VerifyPassword(password string) bool {
	a.mu.RLock()
	encoded := a.store.PasswordHash
	a.mu.RUnlock()
	if encoded == "" {
		return false
	}
	return verifyPassword(password, encoded)
}

// CreateSession generates a new token, records its hash, persists, and
// returns the raw token to the caller. Caller hands it to the client.
func (a *authState) CreateSession(label string) (string, error) {
	tok, err := randomToken()
	if err != nil {
		return "", err
	}
	rec := sessionRecord{
		TokenHash: sha256Hex(tok),
		Label:     label,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	a.mu.Lock()
	a.store.Sessions = append(pruneExpired(a.store.Sessions), rec)
	a.tokenSet[tok] = time.Now()
	a.mu.Unlock()
	return tok, a.save()
}

// IsValidToken recognizes session tokens. Touches the in-memory cache
// first so the hot path stays cheap; falls through to a file check on
// the first request after a server restart (when the in-mem cache is
// empty but the persisted hash record matches).
func (a *authState) IsValidToken(tok string) bool {
	if tok == "" {
		return false
	}
	a.mu.RLock()
	if _, ok := a.tokenSet[tok]; ok {
		a.mu.RUnlock()
		a.touch(tok)
		return true
	}
	hash := sha256Hex(tok)
	for _, s := range a.store.Sessions {
		if subtle.ConstantTimeCompare([]byte(s.TokenHash), []byte(hash)) != 1 {
			continue
		}
		if time.Since(s.LastUsed) > sessionTTL {
			a.mu.RUnlock()
			return false // expired but not yet swept; sweeper will drop it
		}
		// Promote to write lock to populate the in-memory cache.
		// Drop+re-acquire is the only path Go's RWMutex offers; the
		// race window between Unlock and Lock can let two requests
		// both reach the populate path. Re-check once we hold the
		// write lock so the populate is idempotent (cheap; the entry
		// either already exists or we add it).
		a.mu.RUnlock()
		a.mu.Lock()
		if _, already := a.tokenSet[tok]; !already {
			a.tokenSet[tok] = time.Now()
		}
		a.mu.Unlock()
		a.touch(tok)
		return true
	}
	a.mu.RUnlock()
	return false
}

// RevokeToken kills one session — the "logout" path.
func (a *authState) RevokeToken(tok string) {
	hash := sha256Hex(tok)
	a.mu.Lock()
	delete(a.tokenSet, tok)
	out := a.store.Sessions[:0]
	for _, s := range a.store.Sessions {
		if subtle.ConstantTimeCompare([]byte(s.TokenHash), []byte(hash)) != 1 {
			out = append(out, s)
		}
	}
	a.store.Sessions = out
	a.mu.Unlock()
	_ = a.save()
}

// RevokeAllSessions is the "log out everywhere" / nuclear option.
func (a *authState) RevokeAllSessions() {
	a.mu.Lock()
	a.store.Sessions = nil
	a.tokenSet = map[string]time.Time{}
	a.mu.Unlock()
	_ = a.save()
}

// SessionCount is the count of active sessions (for UI display).
func (a *authState) SessionCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.store.Sessions)
}

// touch updates LastUsed for the matching session record. Done lazily
// (every authed request triggers a save would be too expensive — only
// every ~10 minutes matters for the expiry sweeper).
func (a *authState) touch(tok string) {
	hash := sha256Hex(tok)
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.store.Sessions {
		if subtle.ConstantTimeCompare([]byte(a.store.Sessions[i].TokenHash), []byte(hash)) == 1 {
			if time.Since(a.store.Sessions[i].LastUsed) > 10*time.Minute {
				a.store.Sessions[i].LastUsed = time.Now()
				go func() { _ = a.save() }()
			}
			return
		}
	}
}

func pruneExpired(in []sessionRecord) []sessionRecord {
	out := in[:0]
	cutoff := time.Now().Add(-sessionTTL)
	for _, s := range in {
		if s.LastUsed.After(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

// ----- argon2id helpers -----

const (
	argonTime    = 1
	argonMemKB   = 64 * 1024 // 64 MiB — typical for interactive logins
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemKB, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemKB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[1], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memKB, time32, threads int
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memKB, &time32, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(time32), uint32(memKB), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ----- token helpers -----

// randomToken returns 256 bits of entropy as 64 hex chars — long enough
// that brute-force is never the weak link, short enough to fit in a
// localStorage value comfortably.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
