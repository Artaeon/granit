// Package push handles Web Push notification subscriptions and
// delivery. The granit web app subscribes a service worker via the
// browser PushManager; the resulting subscription endpoint + keys
// are POST'd to the server and persisted under
// <vault>/.granit/push-subs.json. A separate scheduler in
// internal/serveapi reads upcoming events and uses Send() to fire
// reminders at the configured offset before the event start.
//
// Why opt-in: the subscription record contains a unique browser
// endpoint and ECDH keys. Storing those in a vault dir is fine for
// a single-tenant tool, but a user who doesn't want push should
// never have to expose them. The frontend gates the subscribe call
// behind an explicit "Enable mobile reminders" toggle.
//
// VAPID key pair lives at <vault>/.granit/push-keys.json. Generated
// once on first call; persisted so reinstalled subscribers don't
// have to re-subscribe (the keys are part of the subscription
// signature). Permissions: 0o600.
package push

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/artaeon/granit/internal/atomicio"
)

// VAPID claim subject — must be a mailto: or https:// URL per the
// Web Push spec. We use a generic placeholder so the SDK doesn't
// reject the request; some push services are stricter than others
// about wanting a real value here. Operators can override via
// env var if they want their real address in push payloads.
const defaultVAPIDSubject = "mailto:granit@example.com"

// Keys are the VAPID key pair for this vault.
type Keys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
	Subject string `json:"subject,omitempty"`
}

// Subscription mirrors the shape PushManager.subscribe() returns
// in the browser: an endpoint URL plus a P-256 ECDH public key (p256dh)
// and an authentication secret (auth). Stored verbatim — the
// webpush library accepts the same shape directly.
type Subscription struct {
	Endpoint  string    `json:"endpoint"`
	Keys      KeyBundle `json:"keys"`
	CreatedAt string    `json:"created_at,omitempty"`
	// Label is a friendly identifier the user can attach to a
	// device ("iPhone", "work laptop"). Optional.
	Label string `json:"label,omitempty"`
	// Paused: when true, the subscription stays in the file but
	// SendAll skips it. Lets users disable notifications on a
	// device temporarily (overnight, while focusing) without
	// having to re-grant browser permission to re-enable.
	Paused bool `json:"paused,omitempty"`
}

type KeyBundle struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// Manager wraps the VAPID keys + subscription list with thread-safe
// load/save. One instance per vault. The scheduler holds a
// reference and calls Send for each pending reminder.
type Manager struct {
	vaultRoot string
	mu        sync.Mutex
	keys      *Keys
	subs      []Subscription
	loaded    bool
}

func New(vaultRoot string) *Manager {
	return &Manager{vaultRoot: vaultRoot}
}

func (m *Manager) keysPath() string {
	return filepath.Join(m.vaultRoot, ".granit", "push-keys.json")
}

func (m *Manager) subsPath() string {
	return filepath.Join(m.vaultRoot, ".granit", "push-subs.json")
}

// EnsureKeys returns the VAPID key pair, generating + persisting
// one on first call. Subsequent calls return the cached pair.
// The caller is expected to be ready to handle the (rare) case
// where key generation fails — the most common cause is a
// permissions issue on the .granit dir.
func (m *Manager) EnsureKeys() (*Keys, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys != nil {
		return m.keys, nil
	}
	data, err := os.ReadFile(m.keysPath())
	if err == nil {
		var k Keys
		if err := json.Unmarshal(data, &k); err == nil && k.Public != "" && k.Private != "" {
			m.keys = &k
			return m.keys, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("push: read keys: %w", err)
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("push: generate VAPID: %w", err)
	}
	k := &Keys{Public: pub, Private: priv, Subject: defaultVAPIDSubject}
	dir := filepath.Dir(m.keysPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("push: mkdir: %w", err)
	}
	out, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := atomicio.WriteState(m.keysPath(), out); err != nil {
		return nil, fmt.Errorf("push: write keys: %w", err)
	}
	m.keys = k
	return m.keys, nil
}

// PublicKey returns just the VAPID public key — what the browser
// needs to subscribe. Lazy-generates if not present.
func (m *Manager) PublicKey() (string, error) {
	k, err := m.EnsureKeys()
	if err != nil {
		return "", err
	}
	return k.Public, nil
}

// loadSubs is called under m.mu.
func (m *Manager) loadSubs() error {
	if m.loaded {
		return nil
	}
	data, err := os.ReadFile(m.subsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.subs = []Subscription{}
			m.loaded = true
			return nil
		}
		return fmt.Errorf("push: read subs: %w", err)
	}
	var s []Subscription
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("push: parse subs: %w", err)
	}
	m.subs = s
	m.loaded = true
	return nil
}

// saveSubs is called under m.mu.
func (m *Manager) saveSubs() error {
	out, err := json.MarshalIndent(m.subs, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.subsPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicio.WriteState(m.subsPath(), out)
}

// Subscribe records a new subscription. If an existing record has
// the same endpoint, it's replaced (browsers re-subscribe on key
// rotation; we don't want stale duplicates). Sets CreatedAt if
// the caller didn't.
func (m *Manager) Subscribe(s Subscription) error {
	if s.Endpoint == "" || s.Keys.P256dh == "" || s.Keys.Auth == "" {
		return errors.New("push: subscribe requires endpoint + keys")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadSubs(); err != nil {
		return err
	}
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	out := m.subs[:0]
	for _, ex := range m.subs {
		if ex.Endpoint == s.Endpoint {
			continue
		}
		out = append(out, ex)
	}
	out = append(out, s)
	m.subs = out
	return m.saveSubs()
}

// SetPaused flips the Paused flag on a subscription. Returns
// os.ErrNotExist when no record matches (caller can ignore — this
// is idempotent enough for the UX). The flag is checked at
// send-time so a paused subscription is silently skipped.
func (m *Manager) SetPaused(endpoint string, paused bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadSubs(); err != nil {
		return err
	}
	found := false
	for i := range m.subs {
		if m.subs[i].Endpoint == endpoint {
			m.subs[i].Paused = paused
			found = true
			break
		}
	}
	if !found {
		return errors.New("push: subscription not found")
	}
	return m.saveSubs()
}

// Unsubscribe removes a subscription by endpoint. Returns nil if
// no record matched (idempotent).
func (m *Manager) Unsubscribe(endpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadSubs(); err != nil {
		return err
	}
	out := m.subs[:0]
	for _, ex := range m.subs {
		if ex.Endpoint == endpoint {
			continue
		}
		out = append(out, ex)
	}
	m.subs = out
	return m.saveSubs()
}

// Subscriptions returns a copy of the current subscription list.
func (m *Manager) Subscriptions() ([]Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.loadSubs(); err != nil {
		return nil, err
	}
	out := make([]Subscription, len(m.subs))
	copy(out, m.subs)
	return out, nil
}

// Payload is what the SW receives. Keep small — push services cap
// payload size around 4 KB and some platforms drop oversized.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
	// IconHref overrides the default icon (the granit logo).
	IconHref string `json:"icon,omitempty"`
}

// SendAll delivers `payload` to every stored subscription. Returns
// the count of successful deliveries + any errors. Failed delivery
// to a subscription with a 410-Gone or 404 response triggers
// auto-unsubscribe — those endpoints are dead permanently. Other
// failures are surfaced but the subscription stays.
func (m *Manager) SendAll(payload Payload) (int, []error) {
	keys, err := m.EnsureKeys()
	if err != nil {
		return 0, []error{err}
	}
	subs, err := m.Subscriptions()
	if err != nil {
		return 0, []error{err}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, []error{fmt.Errorf("push: marshal payload: %w", err)}
	}
	subject := keys.Subject
	if subject == "" {
		subject = defaultVAPIDSubject
	}
	successes := 0
	var errs []error
	var stale []string
	for _, s := range subs {
		// Skip paused subscriptions silently — the user wants to
		// stop receiving without unsubscribing. Counts as neither
		// success nor failure.
		if s.Paused {
			continue
		}
		ws := &webpush.Subscription{
			Endpoint: s.Endpoint,
			Keys: webpush.Keys{
				P256dh: s.Keys.P256dh,
				Auth:   s.Keys.Auth,
			},
		}
		resp, err := webpush.SendNotification(body, ws, &webpush.Options{
			Subscriber:      subject,
			VAPIDPublicKey:  keys.Public,
			VAPIDPrivateKey: keys.Private,
			TTL:             60, // seconds; reminder beyond 60s late is irrelevant
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("push to %s: %w", truncate(s.Endpoint), err))
			continue
		}
		// 410 Gone or 404 Not Found = endpoint is permanently dead;
		// auto-unsubscribe so we don't keep hammering it.
		if resp != nil {
			if resp.StatusCode == 410 || resp.StatusCode == 404 {
				stale = append(stale, s.Endpoint)
			}
			_ = resp.Body.Close()
		}
		successes++
	}
	for _, e := range stale {
		_ = m.Unsubscribe(e)
	}
	return successes, errs
}

func truncate(s string) string {
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}
