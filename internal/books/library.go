package books

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BooksDirName is the vault-relative subdirectory the library
// scans for EPUBs. Kept human-readable so the user sees their
// books in a normal file manager (.granit/ is reserved for
// granit's own metadata, EPUB content is user content).
const BooksDirName = "Books"

// Summary is the lightweight shelf row — enough to render a
// cover-grid card without parsing the full EPUB. Built from a
// one-time scan; the heavyweight Open() is reserved for the
// reader page.
type Summary struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Authors []string `json:"authors,omitempty"`
	// HasCover lets the shelf decide whether to render a cover
	// image or a typographic fallback. Avoids a 404 round-trip.
	HasCover bool `json:"hasCover"`
	// Path is vault-relative so the UI can group by folder
	// (Books/Fiction/, Books/Theology/) and sort alphabetically
	// within each.
	Path string `json:"path"`
	// Bytes is the file size on disk. Lets the shelf show "1.2 MB"
	// next to the title — useful when deciding what to read on a
	// long flight where bandwidth matters.
	Bytes int64 `json:"bytes"`
	// TotalChapters is the spine length, captured at scan time
	// while we already have the EPUB open. Stashing it here is the
	// difference between O(1) and O(N×Open) when the list handler
	// renders a "ch X of Y" caption per row.
	TotalChapters int `json:"totalChapters"`
}

// Detail is the reader-page payload — full spine + TOC, with
// per-chapter labels resolved from the TOC. The chapter HTML
// itself ships separately so the initial response stays small.
type Detail struct {
	Summary
	Chapters []ChapterMeta `json:"chapters"`
	TOC      []TOCEntry    `json:"toc"`
}

// ChapterMeta is the spine row enriched with a TOC-derived label.
// When no TOC entry resolves to this spine index, Label falls
// back to "Chapter N" so the reader's chapter list always has
// readable labels.
type ChapterMeta struct {
	Index int    `json:"index"`
	Label string `json:"label"`
	Linear bool   `json:"linear"`
}

// Scan walks <vaultRoot>/Books/ and returns one Summary per .epub
// file. Each EPUB is opened just long enough to read its
// title/authors/cover-flag — readers who care about cold-cache
// performance can wrap this in a memo, but the shelf is small
// enough (typically <100 books) that re-scanning per request is
// fine.
//
// Returns nil + nil if the Books folder doesn't exist (the user
// hasn't opted in yet). Per-file errors are silently skipped —
// one corrupt EPUB shouldn't break the shelf.
func Scan(vaultRoot string) ([]Summary, error) {
	root := filepath.Join(vaultRoot, BooksDirName)
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var out []Summary
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // tolerate one bad subtree
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".epub") {
			return nil
		}
		s, err := summaryFromFile(vaultRoot, p)
		if err != nil {
			return nil
		}
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// FindByID scans the library and returns the first Summary whose
// id matches. Used by the per-book endpoints to resolve `:id` →
// filesystem path. Linear scan is fine: shelves are small.
func FindByID(vaultRoot, id string) (Summary, string, error) {
	all, err := Scan(vaultRoot)
	if err != nil {
		return Summary{}, "", err
	}
	for _, s := range all {
		if s.ID == id {
			return s, filepath.Join(vaultRoot, s.Path), nil
		}
	}
	return Summary{}, "", os.ErrNotExist
}

// LoadDetail opens an EPUB by id and returns reader-ready metadata
// (summary + chapter list + TOC). Cheap enough — manifest parsing
// is small; chapter bodies aren't loaded.
func LoadDetail(vaultRoot, id string) (*Detail, *EPUB, error) {
	s, abs, err := FindByID(vaultRoot, id)
	if err != nil {
		return nil, nil, err
	}
	e, err := Open(abs)
	if err != nil {
		return nil, nil, err
	}
	d := &Detail{Summary: s}
	d.TOC = e.TOC
	d.Chapters = chaptersFromSpine(e.Spine, e.TOC)
	return d, e, nil
}

// chaptersFromSpine builds the per-chapter label list. Walks the
// TOC once to collect spine→label mappings, then fills the spine
// in order with "Chapter N" for any spine row the TOC doesn't
// cover (front matter, copyright pages, the back-matter index
// — these are spine rows that often don't surface in the TOC but
// the reader still needs a stable label for the chapter list).
func chaptersFromSpine(spine []SpineItem, toc []TOCEntry) []ChapterMeta {
	labels := make(map[int]string)
	var walk func([]TOCEntry)
	walk = func(es []TOCEntry) {
		for _, e := range es {
			if e.SpineIdx >= 0 && labels[e.SpineIdx] == "" {
				labels[e.SpineIdx] = e.Title
			}
			walk(e.Children)
		}
	}
	walk(toc)
	out := make([]ChapterMeta, len(spine))
	for i, s := range spine {
		label := labels[i]
		if label == "" {
			label = "Chapter " + itoa(i+1)
		}
		out[i] = ChapterMeta{Index: i, Label: label, Linear: s.Linear}
	}
	return out
}

// summaryFromFile is Scan's per-file worker — opens the EPUB, reads
// the metadata + cover presence, and rolls a stable id from the
// title + path.
func summaryFromFile(vaultRoot, abs string) (Summary, error) {
	e, err := Open(abs)
	if err != nil {
		return Summary{}, err
	}
	defer e.Close()
	rel, _ := filepath.Rel(vaultRoot, abs)
	if rel == "" {
		rel = abs
	}
	stat, _ := os.Stat(abs)
	var bytes int64
	if stat != nil {
		bytes = stat.Size()
	}
	return Summary{
		ID:            SlugID(e.Title, abs),
		Title:         fallbackTitle(e.Title, abs),
		Authors:       e.Authors,
		HasCover:      e.CoverID != "",
		Path:          filepath.ToSlash(rel),
		Bytes:         bytes,
		TotalChapters: len(e.Spine),
	}, nil
}

func fallbackTitle(t, abs string) string {
	if t = strings.TrimSpace(t); t != "" {
		return t
	}
	return strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
}

// itoa avoids the strconv import for a one-line internal helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
