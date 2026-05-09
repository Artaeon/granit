import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8787',
        changeOrigin: false
      },
      '/ws': {
        target: 'ws://localhost:8787',
        ws: true
      }
    }
  },
  // Vitest config — exposed under the same root so unit tests can
  // resolve the same `$lib/...` aliases SvelteKit uses at runtime.
  // jsdom is the right choice for the small set of utilities under
  // test (storage helpers + DOM-light pure functions); we don't run
  // component tests here, so we don't pull in @testing-library.
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,js}'],
    // Setup file installs an in-memory localStorage shim. Node
    // 22+ ships an experimental built-in localStorage that vitest
    // picks up over jsdom's; the experimental version drops writes
    // unless --localstorage-file is set. The setup replaces the
    // global before any test imports execute.
    setupFiles: ['./test-setup.ts'],
    // Speed-of-iteration matters more than parallel safety for our
    // tests, but jsdom-per-file isolation is still cheap enough that
    // shared globals (localStorage) reset cleanly between cases.
    globals: false
  }
});
