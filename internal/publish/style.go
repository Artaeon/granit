package publish

// defaultCSS is the baseline black-and-white minimal stylesheet, intentionally
// dropped in as a single short file the user can read end-to-end. The aesthetic:
// white background, black text, gray for secondary information, no color
// (apart from very subtle borders). Designed to load without any web fonts so
// pages render at first paint with no FOUT.
//
// Users can override entirely by writing their own .granit/publish/theme.css —
// the builder picks that up if present.
const defaultCSS = `:root {
  --bg: #ffffff;
  --fg: #111111;
  --muted: #666666;
  --rule: #e5e5e5;
  --code-bg: #f6f6f6;
  --max: 720px;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0e0e0e;
    --fg: #f3f3f3;
    --muted: #9b9b9b;
    --rule: #262626;
    --code-bg: #1a1a1a;
  }
}
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; }
body {
  background: var(--bg);
  color: var(--fg);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
  font-size: 17px;
  line-height: 1.65;
  -webkit-font-smoothing: antialiased;
}
.wrap { max-width: var(--max); margin: 0 auto; padding: 3rem 1.5rem 6rem; }
header.site {
  border-bottom: 1px solid var(--rule);
  padding: 1.25rem 1.5rem;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  max-width: var(--max);
  margin: 0 auto;
}
header.site a { color: var(--fg); text-decoration: none; font-weight: 600; letter-spacing: -0.01em; }
header.site nav a {
  margin-left: 1.25rem;
  font-weight: 400;
  color: var(--muted);
  font-size: 0.9rem;
}
header.site nav a:hover { color: var(--fg); }
h1, h2, h3, h4, h5, h6 { font-weight: 600; line-height: 1.25; letter-spacing: -0.015em; margin-top: 2.25rem; margin-bottom: 0.85rem; }
h1 { font-size: 2.1rem; margin-top: 1rem; }
h2 { font-size: 1.55rem; }
h3 { font-size: 1.2rem; }
p { margin: 0 0 1rem; }
a { color: var(--fg); text-decoration: underline; text-underline-offset: 2px; text-decoration-thickness: 1px; }
a:hover { text-decoration-thickness: 2px; }
ul, ol { margin: 0 0 1rem; padding-left: 1.4rem; }
li { margin-bottom: 0.25rem; }
hr { border: none; border-top: 1px solid var(--rule); margin: 2.5rem 0; }
blockquote { border-left: 2px solid var(--rule); margin: 1rem 0; padding: 0.25rem 0 0.25rem 1.25rem; color: var(--muted); }
code {
  font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  background: var(--code-bg);
  padding: 0.1rem 0.35rem;
  border-radius: 3px;
  font-size: 0.9em;
}
pre {
  background: var(--code-bg);
  padding: 1rem;
  overflow-x: auto;
  border-radius: 4px;
  font-size: 0.88rem;
  line-height: 1.5;
}
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; margin: 1rem 0; font-size: 0.95rem; }
th, td { border-bottom: 1px solid var(--rule); padding: 0.5rem 0.75rem; text-align: left; }
th { font-weight: 600; }
img { max-width: 100%; height: auto; }

.meta { color: var(--muted); font-size: 0.85rem; margin-top: -0.25rem; margin-bottom: 1.5rem; }
.meta .tag { display: inline-block; margin-right: 0.5rem; padding: 0.05rem 0.5rem; border: 1px solid var(--rule); border-radius: 99px; font-size: 0.78rem; color: var(--muted); text-decoration: none; }
.meta .tag:hover { border-color: var(--fg); color: var(--fg); }

.note-list { list-style: none; padding-left: 0; }
.note-list li { padding: 0.6rem 0; border-bottom: 1px solid var(--rule); }
.note-list li:last-child { border-bottom: none; }
.note-list a { text-decoration: none; }
.note-list a:hover { text-decoration: underline; }
.note-list .title { font-weight: 500; }
.note-list .summary { display: block; color: var(--muted); font-size: 0.88rem; margin-top: 0.15rem; }

.backlinks {
  margin-top: 4rem;
  padding-top: 1.5rem;
  border-top: 1px solid var(--rule);
}
.backlinks h2 { font-size: 0.95rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); margin-top: 0; }
.backlinks ul { list-style: none; padding-left: 0; }
.backlinks li { margin: 0.4rem 0; font-size: 0.92rem; }

footer.site { border-top: 1px solid var(--rule); padding: 1.5rem; text-align: center; color: var(--muted); font-size: 0.85rem; max-width: var(--max); margin: 0 auto; }

.search { margin: 1rem 0 2rem; }
.search input { width: 100%; padding: 0.6rem 0.8rem; font-size: 1rem; border: 1px solid var(--rule); background: var(--bg); color: var(--fg); border-radius: 4px; font-family: inherit; }
.search input:focus { outline: none; border-color: var(--fg); }
.search-results { list-style: none; padding-left: 0; margin-top: 0.5rem; }
.search-results li { padding: 0.4rem 0; border-bottom: 1px solid var(--rule); font-size: 0.92rem; }
.search-empty { color: var(--muted); font-size: 0.88rem; padding: 0.5rem 0; }

/* ── Hero homepage layout (opt-in via homepageStyle: "hero") ─────── */
.hero {
  text-align: center;
  padding: 3rem 0 2.5rem;
  border-bottom: 1px solid var(--rule);
  margin-bottom: 2.5rem;
}
.hero-title {
  font-size: 2.8rem;
  font-weight: 700;
  letter-spacing: -0.025em;
  margin: 0 0 1rem;
  line-height: 1.1;
}
.hero-intro {
  font-size: 1.1rem;
  color: var(--muted);
  max-width: 540px;
  margin: 0 auto 1.5rem;
  line-height: 1.55;
}
.hero .search { max-width: 480px; margin: 0 auto; }
.hero-section-title {
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--muted);
  font-weight: 600;
  margin: 0 0 1rem;
}
.hero-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 1rem;
}
.hero-card {
  display: flex;
  flex-direction: column;
  padding: 1rem 1.1rem;
  border: 1px solid var(--rule);
  border-radius: 4px;
  text-decoration: none;
  transition: border-color 0.15s, transform 0.15s;
  min-height: 5rem;
}
.hero-card:hover {
  border-color: var(--fg);
  transform: translateY(-1px);
}
.hero-card-title {
  font-weight: 600;
  font-size: 0.98rem;
  letter-spacing: -0.01em;
  margin-bottom: 0.3rem;
  color: var(--fg);
}
.hero-card-summary {
  font-size: 0.85rem;
  color: var(--muted);
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* Mermaid diagrams — render area styling is mostly handled by the JS
   library but we set the centred / B&W defaults so it doesn't clash
   with the rest of the page. */
.mermaid {
  text-align: center;
  margin: 1.5rem 0;
}

/* ── Per-note Contents (outline) ─────────────────────────────────── */
.outline {
  border: 1px solid var(--rule);
  border-radius: 4px;
  padding: 0.4rem 0.9rem 0.6rem;
  margin: 0 0 2rem;
  font-size: 0.92rem;
}
.outline summary {
  cursor: pointer;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-size: 0.75rem;
  padding: 0.25rem 0;
  list-style: none;
}
.outline summary::-webkit-details-marker { display: none; }
.outline summary::before { content: "▾  "; color: var(--muted); }
.outline:not([open]) summary::before { content: "▸  "; }
.outline ul { list-style: none; padding-left: 0; margin: 0.4rem 0 0; }
.outline li { margin: 0.15rem 0; }
.outline li.lvl-3 { padding-left: 1rem; }
.outline li.lvl-4 { padding-left: 2rem; }
.outline a { color: var(--muted); text-decoration: none; }
.outline a:hover { color: var(--fg); text-decoration: underline; }

/* ── Prev/Next note navigation ───────────────────────────────────── */
.prev-next {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  margin: 3rem 0 1rem;
  padding-top: 1.5rem;
  border-top: 1px solid var(--rule);
}
.prev-next a {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0.75rem 1rem;
  border: 1px solid var(--rule);
  border-radius: 4px;
  text-decoration: none;
  font-size: 0.92rem;
  transition: border-color 0.15s;
}
.prev-next a:hover { border-color: var(--fg); }
.prev-next .next { text-align: right; }
.prev-next .dir { color: var(--muted); font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.05em; }
.prev-next .t { font-weight: 500; margin-top: 0.15rem; }

/* ── Graph page ──────────────────────────────────────────────────── */
.graph-container {
  border: 1px solid var(--rule);
  border-radius: 4px;
  padding: 1rem;
  margin: 1rem 0;
  background: var(--bg);
}

/* ── Code blocks (chroma output uses inline styles via WithClasses(false))
   so the .85em + line-height applies to wrapped <pre><code> output. ─── */
pre code { display: block; }

/* Wider page when graph or table is present so the SVG / table columns
   don't get squeezed. Targeted via :has() — degrades to default max-width
   on browsers without :has() support. */
.wrap:has(.graph-container),
.wrap:has(table) { max-width: 1000px; }

/* Wide tables get an internal scroll container instead of horizontally
   blowing out the page on narrow viewports. */
.note-body table { display: block; overflow-x: auto; max-width: 100%; }

/* ── Legal pages footer + cookie banner ──────────────────────────── */
footer.site .legal-links { margin-top: 0.4rem; }
footer.site .legal-links a {
  color: var(--muted);
  text-decoration: none;
  margin: 0 0.4rem;
  font-size: 0.82rem;
}
footer.site .legal-links a:hover { color: var(--fg); text-decoration: underline; }
footer.site .branding {
  margin-top: 0.6rem;
  font-size: 0.78rem;
  color: var(--muted);
  letter-spacing: 0.01em;
}
footer.site .branding a {
  color: var(--muted);
  text-decoration: none;
  border-bottom: 1px dotted var(--rule);
  padding-bottom: 1px;
}
footer.site .branding a:hover {
  color: var(--fg);
  border-bottom-color: var(--fg);
}

.cookie-banner {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background: var(--fg);
  color: var(--bg);
  padding: 0.85rem 1.25rem;
  display: none; /* shown by JS after checking localStorage */
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  font-size: 0.88rem;
  box-shadow: 0 -2px 12px rgba(0,0,0,0.08);
  z-index: 100;
}
.cookie-banner.visible { display: flex; }
.cookie-banner p { margin: 0; flex: 1; line-height: 1.45; }
.cookie-banner a { color: var(--bg); text-decoration: underline; }
.cookie-banner button {
  background: var(--bg);
  color: var(--fg);
  border: none;
  padding: 0.45rem 1rem;
  font-size: 0.88rem;
  font-family: inherit;
  cursor: pointer;
  border-radius: 3px;
  font-weight: 500;
}
.cookie-banner button:hover { opacity: 0.85; }

/* ── Reading time chip on note meta ──────────────────────────────── */
.meta .reading-time { color: var(--muted); }
.meta .reading-time::before { content: "·"; margin: 0 0.4rem; }

/* ── Mobile (≤640px) ─────────────────────────────────────────────── */
@media (max-width: 640px) {
  body { font-size: 16px; line-height: 1.6; }
  .wrap { padding: 1.5rem 1rem 4rem; }
  header.site {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.9rem 1rem;
  }
  header.site nav { display: flex; flex-wrap: wrap; gap: 0.25rem; }
  header.site nav a { margin-left: 0; margin-right: 0.85rem; }
  h1 { font-size: 1.7rem; }
  h2 { font-size: 1.3rem; }
  h3 { font-size: 1.1rem; }
  /* Prev/Next stacks vertically; saves the user from cramped 2-column
     squeeze on narrow phones. */
  .prev-next { flex-direction: column; gap: 0.5rem; }
  .prev-next .next { text-align: left; }
  /* Tags wrap to multiple lines without overflow. */
  .meta .tag { display: inline-block; margin-bottom: 0.25rem; }
  /* Search input + tap targets need to be at least 44px tall. */
  .search input { padding: 0.75rem 0.85rem; font-size: 16px; /* avoid iOS zoom-on-focus */ }
  /* Cookie banner stacks button below text. */
  .cookie-banner { flex-direction: column; align-items: stretch; padding: 0.85rem 1rem; }
  .cookie-banner button { width: 100%; padding: 0.65rem; }
  /* Outline panel padding tightens. */
  .outline { padding: 0.4rem 0.75rem 0.5rem; font-size: 0.88rem; }
}

/* Very narrow (≤380px) — extra tightening for budget Android phones */
@media (max-width: 380px) {
  .wrap { padding: 1rem 0.75rem 3rem; }
  h1 { font-size: 1.5rem; }
  header.site { padding: 0.75rem; }
}
`

// defaultSearchJS is a tiny ~30-line vanilla-JS fuzzy filter that consumes
// search-index.json (emitted by the builder). No frameworks, no bundlers —
// pure ES5 so it runs everywhere GitHub Pages serves to. Hidden behind a
// progressive-enhancement: the search input is only injected when JS is on.
const defaultSearchJS = `(function(){
  var idxEl = document.getElementById('search-index-url');
  if (!idxEl) return;
  var url = idxEl.getAttribute('data-url');
  var input = document.querySelector('.search input');
  var results = document.querySelector('.search-results');
  if (!input || !results) return;
  var docs = null;
  fetch(url).then(function(r){return r.json();}).then(function(data){ docs = data; });
  function norm(s){ return (s||'').toLowerCase(); }
  input.addEventListener('input', function(){
    if (!docs) return;
    var q = norm(input.value).trim();
    results.innerHTML = '';
    if (!q) return;
    var hits = [];
    for (var i=0;i<docs.length;i++){
      var d = docs[i];
      var score = 0;
      if (norm(d.title).indexOf(q) !== -1) score += 10;
      if (norm(d.body).indexOf(q) !== -1) score += 1;
      if (score) hits.push({d:d, score:score});
    }
    hits.sort(function(a,b){return b.score - a.score;});
    hits.slice(0, 20).forEach(function(h){
      var li = document.createElement('li');
      var a = document.createElement('a');
      a.href = h.d.url;
      a.textContent = h.d.title;
      li.appendChild(a);
      results.appendChild(li);
    });
    if (!hits.length) {
      var li = document.createElement('li');
      li.className = 'search-empty';
      li.textContent = 'No matches.';
      results.appendChild(li);
    }
  });
})();`
