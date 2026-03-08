package tui

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateDocument creates a multi-paragraph markdown string with headings,
// wikilinks, and body text suitable for benchmarking tokenizers and TF-IDF.
func generateDocument(paragraphs int) string {
	var sb strings.Builder
	sb.WriteString("# Document Title\n\n")
	body := "Machine learning algorithms process large datasets to identify patterns and make predictions. " +
		"Neural networks consist of interconnected layers of nodes that transform input signals through weighted connections. " +
		"Gradient descent optimization adjusts model parameters to minimize the loss function during training."
	for i := 0; i < paragraphs; i++ {
		if i > 0 && i%3 == 0 {
			sb.WriteString(fmt.Sprintf("## Section %d\n\n", i/3))
		}
		sb.WriteString(body + "\n\n")
	}
	return sb.String()
}

// buildDocSet creates n documents with varying content for TF-IDF benchmarks.
func buildDocSet(n int) map[string]string {
	topics := []string{
		"Machine learning and artificial intelligence are transforming how we build software systems. Deep learning models achieve state of the art results on many tasks.",
		"Distributed systems require careful consideration of consistency and availability tradeoffs. The CAP theorem establishes fundamental limits on system design.",
		"Functional programming emphasizes immutability and pure functions for building reliable software. Haskell and Erlang pioneered many of these concepts.",
		"Database indexing strategies significantly impact query performance. B-tree indexes handle range queries efficiently while hash indexes excel at point lookups.",
		"Container orchestration platforms like Kubernetes manage deployment scaling and networking for microservices architectures across clusters of machines.",
		"Cryptographic protocols protect data in transit and at rest. Public key infrastructure enables secure communication between parties who have not previously exchanged keys.",
		"Compiler optimization passes transform intermediate representations to generate efficient machine code. Loop unrolling and constant folding are common techniques.",
		"Network protocols define the rules for communication between devices. TCP provides reliable ordered delivery while UDP trades reliability for lower latency.",
		"Operating systems manage hardware resources and provide abstractions for application development. Process scheduling and memory management are core responsibilities.",
		"Version control systems track changes to source code over time. Git uses a directed acyclic graph of commits to represent project history efficiently.",
	}

	docs := make(map[string]string, n)
	for i := 0; i < n; i++ {
		base := topics[i%len(topics)]
		// Add unique content per document to avoid identical entries.
		content := fmt.Sprintf("# Document %d\n\n%s\n\nThis is document number %d with additional unique content to differentiate it from other documents in the corpus. The analysis covers several important aspects of the topic.", i, base, i)
		docs[fmt.Sprintf("doc-%04d.md", i)] = content
	}
	return docs
}

// ---------------------------------------------------------------------------
// TF-IDF benchmarks
// ---------------------------------------------------------------------------

func BenchmarkTFIDFTokenize(b *testing.B) {
	cases := []struct {
		name       string
		paragraphs int
	}{
		{"short", 2},
		{"medium", 10},
		{"long", 50},
	}
	for _, tc := range cases {
		content := generateDocument(tc.paragraphs)
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = tfidfTokenize(content)
			}
		})
	}
}

func BenchmarkBuildTFIDF(b *testing.B) {
	for _, size := range []int{50, 200} {
		docs := buildDocSet(size)
		b.Run(fmt.Sprintf("docs=%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = BuildTFIDF(docs)
			}
		})
	}
}

func BenchmarkFindSimilar(b *testing.B) {
	docs := buildDocSet(200)
	index := BuildTFIDF(docs)
	queryPath := "doc-0000.md"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FindSimilar(index, queryPath, 10)
	}
}

func BenchmarkCosineSimilarity(b *testing.B) {
	// Build two realistic TF-IDF vectors.
	vecA := map[string]float64{
		"machine": 0.35, "learning": 0.32, "neural": 0.28, "network": 0.25,
		"training": 0.22, "model": 0.20, "data": 0.18, "algorithm": 0.15,
		"gradient": 0.12, "optimization": 0.10, "loss": 0.08, "function": 0.06,
		"layer": 0.05, "input": 0.04, "output": 0.03, "weight": 0.02,
	}
	vecB := map[string]float64{
		"machine": 0.30, "learning": 0.28, "deep": 0.25, "network": 0.22,
		"architecture": 0.20, "convolution": 0.18, "data": 0.15, "feature": 0.12,
		"extraction": 0.10, "classification": 0.08, "recognition": 0.06, "training": 0.05,
		"batch": 0.04, "normalization": 0.03, "dropout": 0.02, "activation": 0.01,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cosineSimilarity(vecA, vecB)
	}
}

// ---------------------------------------------------------------------------
// HTML to Markdown benchmarks
// ---------------------------------------------------------------------------

func BenchmarkHTMLToMarkdown(b *testing.B) {
	simpleHTML := `<html><body>
<h1>Hello World</h1>
<p>This is a <strong>simple</strong> paragraph with <em>emphasis</em> and a <a href="https://example.com">link</a>.</p>
<p>Another paragraph with some text.</p>
</body></html>`

	complexHTML := `<html><body>
<h1>Complex Document</h1>
<p>Introduction paragraph with <strong>bold</strong>, <em>italic</em>, and <code>inline code</code>.</p>
<h2>Code Example</h2>
<pre><code class="language-go">func main() {
    fmt.Println("Hello, World!")
}</code></pre>
<h2>Data Table</h2>
<table>
<thead><tr><th>Name</th><th>Value</th><th>Description</th></tr></thead>
<tbody>
<tr><td>Alpha</td><td>1.0</td><td>First parameter</td></tr>
<tr><td>Beta</td><td>2.5</td><td>Second parameter</td></tr>
<tr><td>Gamma</td><td>0.3</td><td>Third parameter</td></tr>
</tbody>
</table>
<h2>Lists</h2>
<ul>
<li>First item with <a href="https://example.com/1">a link</a></li>
<li>Second item with <strong>bold text</strong></li>
<li>Third item with <em>emphasis</em></li>
</ul>
<ol>
<li>Step one</li>
<li>Step two</li>
<li>Step three</li>
</ol>
<blockquote><p>This is a blockquote with multiple sentences. It contains important information that should be preserved.</p></blockquote>
<h3>Sub-section</h3>
<p>Final paragraph with a <a href="/relative/path">relative link</a> and an <img src="https://example.com/image.png" alt="example image"/>.</p>
</body></html>`

	cases := []struct {
		name string
		html string
	}{
		{"simple", simpleHTML},
		{"complex", complexHTML},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = htmlToMarkdown(tc.html, "https://example.com")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Folding benchmarks
// ---------------------------------------------------------------------------

func BenchmarkFoldAll(b *testing.B) {
	// Build a 500-line document with headings at regular intervals.
	lines := make([]string, 500)
	for i := 0; i < 500; i++ {
		switch {
		case i%50 == 0:
			lines[i] = fmt.Sprintf("# Heading Level 1 — Section %d", i/50)
		case i%25 == 0:
			lines[i] = fmt.Sprintf("## Heading Level 2 — Sub-section %d", i/25)
		case i%100 == 10:
			lines[i] = "```go"
		case i%100 == 15:
			lines[i] = "```"
		default:
			lines[i] = "This is a line of content in the document that provides body text for the section."
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := NewFoldState()
		fs.FoldAll(lines)
	}
}

func BenchmarkHeadingLevel(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{
		{"h1", "# Top Level Heading"},
		{"h3", "### Third Level Heading"},
		{"h6", "###### Sixth Level Heading"},
		{"not_heading", "This is just a regular line of text"},
		{"hash_no_space", "###NoSpaceAfterHash"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = headingLevel(tc.input)
			}
		})
	}
}
