// Vitest setup — runs once before each test file.
//
// Why this exists: Node 22+ ships an experimental built-in
// localStorage that vitest 4 picks up over jsdom's, and the
// experimental version doesn't persist data across calls (it
// requires a --localstorage-file path that vitest doesn't set).
// That broke the storage / proposalCache tests under the default
// `environment: 'jsdom'` configuration.
//
// Replacing localStorage with a tiny in-memory Storage shim
// before any test runs guarantees deterministic behaviour
// matching the browser contract these tests model.

class MemoryStorage implements Storage {
  private map = new Map<string, string>();
  get length() {
    return this.map.size;
  }
  clear() {
    this.map.clear();
  }
  getItem(key: string) {
    return this.map.has(key) ? (this.map.get(key) as string) : null;
  }
  key(index: number) {
    return Array.from(this.map.keys())[index] ?? null;
  }
  removeItem(key: string) {
    this.map.delete(key);
  }
  setItem(key: string, value: string) {
    this.map.set(key, String(value));
  }
}

// Use Object.defineProperty so we can also overwrite the read-only
// global on some Node versions. The cast lets us assign over the
// experimental built-in without TypeScript complaining.
Object.defineProperty(globalThis, 'localStorage', {
  value: new MemoryStorage(),
  writable: true,
  configurable: true
});
