package publish

import (
	"html/template"
)

// Three templates — page (everything wraps in this), index (note list), note
// (single rendered note + backlinks). Kept inline so a fresh user gets a
// working publish without copying template files; users can override by
// dropping templates of the same name into .granit/publish/templates/.
//
// We deliberately keep the markup minimal and semantic: <header>, <main>,
// <footer> with one CSS class each. The CSS in style.go does the heavy
// lifting. A user who wants a wildly different look can rewrite style.css
// without touching the templates.

const tplBase = `<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.PageTitle}}{{if .SiteTitle}} — {{.SiteTitle}}{{end}}</title>
<link rel="stylesheet" href="{{.RelRoot}}style.css">
{{if .Description}}<meta name="description" content="{{.Description}}">{{end}}
{{if .Author}}<meta name="author" content="{{.Author}}">{{end}}
{{if .NoIndex}}<meta name="robots" content="noindex,nofollow">{{end}}
{{if .CanonicalURL}}<link rel="canonical" href="{{.CanonicalURL}}">{{end}}
<meta property="og:title" content="{{.PageTitle}}">
{{if .Description}}<meta property="og:description" content="{{.Description}}">{{end}}
<meta property="og:type" content="{{.OGType}}">
<meta property="og:site_name" content="{{.SiteTitle}}">
{{if .CanonicalURL}}<meta property="og:url" content="{{.CanonicalURL}}">{{end}}
{{if .OGImage}}<meta property="og:image" content="{{.OGImage}}">
{{if .OGImageType}}<meta property="og:image:type" content="{{.OGImageType}}">{{end}}
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">{{end}}
<meta name="twitter:card" content="{{if .OGImage}}summary_large_image{{else}}summary{{end}}">
<meta name="twitter:title" content="{{.PageTitle}}">
{{if .Description}}<meta name="twitter:description" content="{{.Description}}">{{end}}
{{if .OGImage}}<meta name="twitter:image" content="{{.OGImage}}">{{end}}
{{if .FeedLink}}{{.FeedLink}}{{end}}
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
{{if .HasMath}}<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css" integrity="sha384-nB0miv6/jRmo5UMMR1wu3Gz6NLsoTkbqJghGIsx//Rlm+ZU03BU6SQNC66uf4l5+" crossorigin="anonymous">{{end}}</head>
<body>
<header class="site">
  <a href="{{.RelRoot}}index.html">{{.SiteTitle}}</a>
  <nav>
    <a href="{{.RelRoot}}index.html">Index</a>
    {{if .HasGraph}}<a href="{{.RelRoot}}graph.html">Graph</a>{{end}}
    {{if .HasTags}}<a href="{{.RelRoot}}tags/index.html">Tags</a>{{end}}
  </nav>
</header>
<main class="wrap">
{{.Body}}
</main>
<footer class="site">
  {{if .Footer}}<div>{{.Footer}}</div>{{end}}
  {{if or .HasImpressum .HasDatenschutz}}<div class="legal-links">
  {{if .HasImpressum}}<a href="{{.ImpressumURL}}">Impressum</a>{{end}}
  {{if .HasDatenschutz}}<a href="{{.DatenschutzURL}}">Datenschutz</a>{{end}}
  </div>{{end}}
  {{if .Branding}}<div class="branding">{{.Branding}}</div>{{end}}
</footer>
{{if .CookieBanner}}<div class="cookie-banner" id="cookie-banner" role="dialog" aria-label="Cookie notice">
  <p>{{.CookieMessage}}</p>
  <button type="button" id="cookie-accept">OK</button>
</div>
<script>
  (function(){
    try {
      if (localStorage.getItem('granit-cookie-accepted') === '1') return;
      var b = document.getElementById('cookie-banner');
      var btn = document.getElementById('cookie-accept');
      if (!b || !btn) return;
      b.classList.add('visible');
      btn.addEventListener('click', function(){
        try { localStorage.setItem('granit-cookie-accepted', '1'); } catch(e) {}
        b.classList.remove('visible');
      });
    } catch(e) {}
  })();
</script>{{end}}
{{if .Search}}<span id="search-index-url" data-url="{{.RelRoot}}search-index.json"></span>
<script src="{{.RelRoot}}search.js"></script>{{end}}
{{if .HasMath}}<script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.js" integrity="sha384-7zkQWkzuo3B5mTepMUcHkMB5jZaolc2xDwL6VFqjFALcbeS9Ggm/Yr2r3Dy4lfFg" crossorigin="anonymous"></script>
<script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/contrib/auto-render.min.js" integrity="sha384-43gviWU0YVjaDtb/GhzOouOXtZMP/7XUzwPTstBeZFe/+rCMvRwr4yROQP43s0Xk" crossorigin="anonymous"
  onload="renderMathInElement(document.body, {delimiters:[{left:'$$',right:'$$',display:true},{left:'$',right:'$',display:false}]});"></script>{{end}}
{{if .HasMermaid}}<script type="module">
import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
mermaid.initialize({ startOnLoad: true, theme: 'neutral' });
</script>{{end}}
</body>
</html>`

const tplIndex = `<h1>{{.SiteTitle}}</h1>
{{if .Intro}}<p class="meta">{{.Intro}}</p>{{end}}
{{if .Search}}<div class="search">
  <input type="search" placeholder="Search…" autofocus>
  <ul class="search-results"></ul>
</div>{{end}}
<ul class="note-list">
{{range .Notes}}<li>
  <a href="{{.URL}}"><span class="title">{{.Title}}</span></a>
  {{if .Summary}}<span class="summary">{{.Summary}}</span>{{end}}
</li>
{{end}}</ul>`

const tplNote = `<article class="note">
<h1>{{.Title}}</h1>
{{if or .Date .Tags .ReadingTime}}<div class="meta">
{{if .Date}}{{.Date}}{{end}}
{{if .ReadingTime}}<span class="reading-time">{{.ReadingTime}}</span>{{end}}
{{range .Tags}} <a class="tag" href="{{$.RelRoot}}tags/{{.Slug}}.html">#{{.Name}}</a>{{end}}
</div>{{end}}
{{if .Outline}}<details class="outline" open>
  <summary>Contents</summary>
  <ul>
  {{range .Outline}}<li class="lvl-{{.Level}}"><a href="#{{.Slug}}">{{.Text}}</a></li>{{end}}
  </ul>
</details>{{end}}
<div class="note-body">
{{.Content}}
</div>
{{if or .Prev .Next}}<nav class="prev-next">
  {{if .Prev}}<a class="prev" href="{{.Prev.URL}}"><span class="dir">← Previous</span><span class="t">{{.Prev.Title}}</span></a>{{end}}
  {{if .Next}}<a class="next" href="{{.Next.URL}}"><span class="dir">Next →</span><span class="t">{{.Next.Title}}</span></a>{{end}}
</nav>{{end}}
{{if .Backlinks}}<aside class="backlinks">
  <h2>Linked from</h2>
  <ul>
  {{range .Backlinks}}<li><a href="{{.URL}}">{{.Title}}</a></li>{{end}}
  </ul>
</aside>{{end}}
</article>`

const tplTagIndex = `<h1>Tags</h1>
<ul class="note-list">
{{range .Tags}}<li>
  <a href="{{.Slug}}.html"><span class="title">#{{.Name}}</span></a>
  <span class="summary">{{.Count}} note{{if ne .Count 1}}s{{end}}</span>
</li>
{{end}}</ul>`

const tplTagPage = `<h1>#{{.Name}}</h1>
<ul class="note-list">
{{range .Notes}}<li>
  <a href="{{$.RelRoot}}{{.URL}}"><span class="title">{{.Title}}</span></a>
  {{if .Summary}}<span class="summary">{{.Summary}}</span>{{end}}
</li>
{{end}}</ul>`

const tplNotFound = `<h1>404 — Not found</h1>
<p class="meta">The page you were looking for doesn't exist on this site. It may have been moved, renamed, or never existed.</p>
<p><a href="{{.RelRoot}}index.html">← Back to the index</a></p>`

const tplHero = `<section class="hero">
  <h1 class="hero-title">{{.SiteTitle}}</h1>
  {{if .Intro}}<p class="hero-intro">{{.Intro}}</p>{{end}}
  {{if .Search}}<div class="search">
    <input type="search" placeholder="Search…">
    <ul class="search-results"></ul>
  </div>{{end}}
</section>
{{if .Notes}}<section class="hero-notes">
  <h2 class="hero-section-title">All notes</h2>
  <div class="hero-grid">
  {{range .Notes}}<a class="hero-card" href="{{.URL}}">
    <span class="hero-card-title">{{.Title}}</span>
    {{if .Summary}}<span class="hero-card-summary">{{.Summary}}</span>{{end}}
  </a>
  {{end}}</div>
</section>{{end}}`

const tplGraph = `<h1>Graph</h1>
<p class="meta">{{.NoteCount}} notes, {{.EdgeCount}} links — hover a node to highlight, click to navigate.</p>
<div class="graph-container">
{{.SVG}}
</div>`

// templates carries the parsed Go templates the builder hands off to render.
// Lazily parsed once per Build() call; errors are programmer errors (bad
// template syntax) so we panic on parse failure.
type templates struct {
	base     *template.Template
	index    *template.Template
	note     *template.Template
	tagIndex *template.Template
	tagPage  *template.Template
	graph    *template.Template
	notFound *template.Template
	hero     *template.Template
}

func mustParseTemplates() *templates {
	parse := func(name, src string) *template.Template {
		t, err := template.New(name).Parse(src)
		if err != nil {
			panic("publish: template parse: " + err.Error())
		}
		return t
	}
	return &templates{
		base:     parse("base", tplBase),
		index:    parse("index", tplIndex),
		note:     parse("note", tplNote),
		tagIndex: parse("tagindex", tplTagIndex),
		tagPage:  parse("tagpage", tplTagPage),
		graph:    parse("graph", tplGraph),
		notFound: parse("404", tplNotFound),
		hero:     parse("hero", tplHero),
	}
}
