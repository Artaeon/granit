import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      // Build output is embedded by granit/internal/serveapi at compile time.
      pages: '../internal/serveapi/dist',
      assets: '../internal/serveapi/dist',
      fallback: 'index.html',
      precompress: false
    }),
    alias: {
      $lib: 'src/lib'
    }
  }
};

export default config;
