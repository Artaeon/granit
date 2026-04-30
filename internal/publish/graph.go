package publish

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// graphNode is one note in the graph layout. Pos is computed by the
// force-directed solver below; Slug + Title come from the Note that
// produced it and feed both the SVG label and the click-through URL.
type graphNode struct {
	Slug   string
	Title  string
	X, Y   float64
	Degree int // number of edges incident on this node
}

type graphEdge struct {
	From, To int // indices into graphNodes
}

// renderGraphSVG builds a force-directed graph from the given notes and
// returns it as a self-contained SVG string. Layout: Fruchterman-Reingold,
// 200 iterations with linear cooling. Output is sized to a fixed 1200×800
// viewport and uses viewBox so the browser scales it smoothly to any
// container width.
//
// Aesthetic: small filled circles (radius scaled by degree so well-connected
// notes pop), gray edges, black labels, no color. Hover state via CSS
// inside the SVG so the linked-from-here halo highlights without JS.
//
// Returns an empty string if there are fewer than 2 notes — a single dot
// graph isn't useful, and the caller skips emitting graph.html in that
// case.
func renderGraphSVG(notes []*Note) string {
	if len(notes) < 2 {
		return ""
	}

	// Build nodes + edges from the note set's outlinks.
	idx := make(map[string]int, len(notes))
	nodes := make([]graphNode, 0, len(notes))
	for i, n := range notes {
		idx[n.RelPath] = i
		nodes = append(nodes, graphNode{Slug: n.Slug, Title: n.Title})
	}
	var edges []graphEdge
	for i, n := range notes {
		for _, target := range n.Outlinks {
			if j, ok := idx[target]; ok && j != i {
				// Dedup edges: store i<j only.
				a, b := i, j
				if a > b {
					a, b = b, a
				}
				edges = append(edges, graphEdge{From: a, To: b})
			}
		}
	}
	// Dedup edges (since each link appears once per source).
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	dedup := edges[:0]
	for i, e := range edges {
		if i == 0 || e != edges[i-1] {
			dedup = append(dedup, e)
		}
	}
	edges = dedup
	for _, e := range edges {
		nodes[e.From].Degree++
		nodes[e.To].Degree++
	}

	layoutFruchtermanReingold(nodes, edges)

	return svgString(nodes, edges)
}

// layoutFruchtermanReingold mutates nodes[i].X/Y in place, running ~200
// iterations of the classic FR force-directed algorithm. Repulsive force
// between every pair of nodes is k²/d; attractive force along edges is
// d²/k where k is the ideal edge length. Temperature decays linearly so
// late iterations make small adjustments — gives a stable layout that
// doesn't drift between runs (we seed rand deterministically).
func layoutFruchtermanReingold(nodes []graphNode, edges []graphEdge) {
	const (
		W            = 1000.0
		H            = 700.0
		iterations   = 200
		gravityScale = 0.05 // pulls nodes toward the centre, prevents drift-off
	)
	n := len(nodes)
	if n == 0 {
		return
	}
	// Deterministic placement so identical input → identical SVG. Helps
	// git diffs of generated sites stay readable.
	r := rand.New(rand.NewSource(int64(n) * 9301))
	for i := range nodes {
		nodes[i].X = r.Float64()*W - W/2
		nodes[i].Y = r.Float64()*H - H/2
	}

	area := W * H
	k := math.Sqrt(area / float64(n))
	temperature := W / 10

	disp := make([]struct{ X, Y float64 }, n)

	for iter := 0; iter < iterations; iter++ {
		// Reset displacement.
		for i := range disp {
			disp[i].X = 0
			disp[i].Y = 0
		}
		// Repulsive forces (every pair).
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				dx := nodes[i].X - nodes[j].X
				dy := nodes[i].Y - nodes[j].Y
				d := math.Hypot(dx, dy)
				if d < 0.01 {
					d = 0.01
					dx = r.Float64() * 0.1
					dy = r.Float64() * 0.1
				}
				force := k * k / d
				ux, uy := dx/d, dy/d
				disp[i].X += ux * force
				disp[i].Y += uy * force
				disp[j].X -= ux * force
				disp[j].Y -= uy * force
			}
		}
		// Attractive forces along edges.
		for _, e := range edges {
			dx := nodes[e.From].X - nodes[e.To].X
			dy := nodes[e.From].Y - nodes[e.To].Y
			d := math.Hypot(dx, dy)
			if d < 0.01 {
				continue
			}
			force := d * d / k
			ux, uy := dx/d, dy/d
			disp[e.From].X -= ux * force
			disp[e.From].Y -= uy * force
			disp[e.To].X += ux * force
			disp[e.To].Y += uy * force
		}
		// Gravity toward origin.
		for i := range nodes {
			disp[i].X -= nodes[i].X * gravityScale
			disp[i].Y -= nodes[i].Y * gravityScale
		}
		// Apply with temperature cap.
		for i := range nodes {
			d := math.Hypot(disp[i].X, disp[i].Y)
			if d > 0 {
				cap := math.Min(d, temperature)
				nodes[i].X += disp[i].X / d * cap
				nodes[i].Y += disp[i].Y / d * cap
			}
		}
		// Linear cooling.
		temperature = (W / 10) * (1 - float64(iter)/float64(iterations))
	}

	// Normalise into a [40, W-40] × [40, H-40] viewport with margin so labels
	// don't get clipped at the edges.
	const margin = 40.0
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, nd := range nodes {
		if nd.X < minX {
			minX = nd.X
		}
		if nd.Y < minY {
			minY = nd.Y
		}
		if nd.X > maxX {
			maxX = nd.X
		}
		if nd.Y > maxY {
			maxY = nd.Y
		}
	}
	rangeX := maxX - minX
	rangeY := maxY - minY
	if rangeX < 1 {
		rangeX = 1
	}
	if rangeY < 1 {
		rangeY = 1
	}
	for i := range nodes {
		nodes[i].X = margin + (nodes[i].X-minX)/rangeX*(W-2*margin)
		nodes[i].Y = margin + (nodes[i].Y-minY)/rangeY*(H-2*margin)
	}
}

// svgString renders the laid-out nodes + edges as an SVG document. The CSS
// inside the SVG handles hover highlighting — keeps the page JS-free.
func svgString(nodes []graphNode, edges []graphEdge) string {
	const W = 1000
	const H = 700
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet" class="graph-svg">`, W, H)
	b.WriteString(`<style>
    .graph-svg { width: 100%; height: auto; max-height: 80vh; }
    .graph-svg .edge { stroke: var(--rule); stroke-width: 1; }
    .graph-svg .node circle { fill: var(--fg); stroke: var(--bg); stroke-width: 2; transition: r 0.15s; }
    .graph-svg .node:hover circle { r: 9; }
    .graph-svg .node text { font: 10px -apple-system, sans-serif; fill: var(--fg); pointer-events: none; }
    .graph-svg .node:hover text { font-weight: 600; }
    .graph-svg .node a { text-decoration: none; }
  </style>`)

	// Edges first so nodes paint over them.
	for _, e := range edges {
		fmt.Fprintf(&b, `<line class="edge" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			nodes[e.From].X, nodes[e.From].Y,
			nodes[e.To].X, nodes[e.To].Y)
	}
	// Nodes. Radius scales with degree (3 → 7 px). Each is wrapped in an
	// <a> so a click navigates to the note.
	for _, nd := range nodes {
		radius := 3.0 + math.Min(4, float64(nd.Degree)*0.5)
		fmt.Fprintf(&b, `<g class="node"><a href="notes/%s.html"><circle cx="%.1f" cy="%.1f" r="%.1f"/><text x="%.1f" y="%.1f">%s</text></a></g>`,
			escapeAttr(nd.Slug), nd.X, nd.Y, radius,
			nd.X+radius+3, nd.Y+3, escapeText(truncate(nd.Title, 32)))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// escapeAttr / escapeText do the minimum HTML-attribute / text escaping
// needed for SVG output. Avoids pulling in html/template here just for two
// values per node.
func escapeAttr(s string) string {
	r := strings.NewReplacer(`"`, `&quot;`, `&`, `&amp;`, `<`, `&lt;`, `>`, `&gt;`)
	return r.Replace(s)
}

func escapeText(s string) string {
	r := strings.NewReplacer(`&`, `&amp;`, `<`, `&lt;`, `>`, `&gt;`)
	return r.Replace(s)
}
