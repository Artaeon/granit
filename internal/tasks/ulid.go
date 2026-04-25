package tasks

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ulidEntropyLock guards the monotonic entropy source. ULID's
// monotonic generator isn't goroutine-safe on its own — a lock
// here makes NewID safe to call from any reconciliation pass.
var (
	ulidEntropyLock sync.Mutex
	ulidEntropy     = ulid.Monotonic(rand.Reader, 0)
)

// NewID mints a fresh ULID for a task. ULIDs are 26-char
// Crockford-base32 strings, time-sortable (so sidecars sort
// chronologically by creation), and have ~80 bits of randomness so
// collisions at granit-scale (millions of tasks across all users)
// are statistically impossible.
//
// IDs live only in the sidecar — never inlined in markdown.
func NewID() string {
	ulidEntropyLock.Lock()
	defer ulidEntropyLock.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
