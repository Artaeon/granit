package serveapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/scripture/bible"
)

// handleBibleTranslations lists every bundled translation (WEB plus any
// extras dropped into internal/scripture/bible/*.json via
// scripts/fetch-bible-translations.sh). Cheap call backed by the
// in-memory map; the frontend pings this once to populate the
// translation-picker chip strip.
func (s *Server) handleBibleTranslations(w http.ResponseWriter, r *http.Request) {
	ts, err := bible.Translations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"translations": ts,
		"total":        len(ts),
	})
}

// passageCompareTranslation is one column in the side-by-side response.
// Verses come straight from the loaded Bible — same shape the reader
// already uses — plus the translation metadata needed to render the
// column header without a second round trip.
type passageCompareTranslation struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Abbreviation string         `json:"abbreviation"`
	Reference    string         `json:"reference"` // "John 3:16-17"
	Verses       []bible.Verse  `json:"verses"`
}

// handleBiblePassageCompare returns the SAME passage across multiple
// translations side by side. Used by TranslationDiff.svelte.
//
// Query:
//
//	?book=JHN              required — USFM code or display name
//	?chapter=3             required — 1-indexed
//	?verseFrom=16          optional — defaults to 1
//	?verseTo=17            optional — defaults to verseFrom (single verse)
//	                       or end-of-chapter if verseFrom is also unset
//	?translations=web,asv  optional — CSV of translation ids; empty =
//	                       every bundled translation in display order.
//	                       Unknown ids are skipped silently rather than
//	                       400'ing the whole request (so a client that
//	                       remembers a translation across an uninstall
//	                       still gets a usable response).
//
// Bookkeeping notes:
//   - The book/chapter pair is resolved per-translation. In practice
//     the canon is identical across all four bundled translations, but
//     scoping the lookup keeps us honest if some future translation
//     ships a different numbering (e.g. Hebrew vs English Psalm 9-10).
//   - Verse ranges that don't exist in a given translation produce an
//     empty verses array for that column — the UI can show "—" instead
//     of cratering on the missing data.
func (s *Server) handleBiblePassageCompare(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bookKey := strings.TrimSpace(q.Get("book"))
	if bookKey == "" {
		writeError(w, http.StatusBadRequest, "book required")
		return
	}
	chStr := strings.TrimSpace(q.Get("chapter"))
	chapter, err := strconv.Atoi(chStr)
	if err != nil || chapter < 1 {
		writeError(w, http.StatusBadRequest, "invalid chapter: "+chStr)
		return
	}
	verseFrom := 0
	if v := strings.TrimSpace(q.Get("verseFrom")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			verseFrom = n
		}
	}
	verseTo := 0
	if v := strings.TrimSpace(q.Get("verseTo")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			verseTo = n
		}
	}

	// Resolve the requested translation list against what's actually
	// bundled. Order matters — preserve caller order so the UI's
	// column layout matches the chip strip's selection sequence.
	want := []string{}
	for _, raw := range strings.Split(q.Get("translations"), ",") {
		t := strings.TrimSpace(strings.ToLower(raw))
		if t != "" {
			want = append(want, t)
		}
	}
	if len(want) == 0 {
		// Empty selection = every bundled translation, in display order.
		all, err := bible.Translations()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, t := range all {
			want = append(want, t.ID)
		}
	}

	cols := make([]passageCompareTranslation, 0, len(want))
	for _, id := range want {
		b, err := bible.Get(id)
		if err != nil {
			// Silently skip translations the server doesn't have —
			// the frontend can warn / re-fetch the bundled list if
			// it cares.
			continue
		}
		bk := bible.FindBook(b.ID, bookKey)
		if bk == nil {
			cols = append(cols, passageCompareTranslation{
				ID:           b.ID,
				Name:         b.Name,
				Abbreviation: b.Abbreviation,
				Reference:    "",
				Verses:       []bible.Verse{},
			})
			continue
		}
		ch := bk.GetChapter(chapter)
		if ch == nil {
			cols = append(cols, passageCompareTranslation{
				ID:           b.ID,
				Name:         b.Name,
				Abbreviation: b.Abbreviation,
				Reference:    "",
				Verses:       []bible.Verse{},
			})
			continue
		}

		// Clamp verse range to what this translation actually has.
		// verseFrom == 0 means "no range specified" → whole chapter.
		var verses []bible.Verse
		if verseFrom == 0 {
			verses = ch.Verses
		} else {
			to := verseTo
			if to == 0 {
				to = verseFrom
			}
			for i := range ch.Verses {
				v := &ch.Verses[i]
				if v.N >= verseFrom && v.N <= to {
					verses = append(verses, *v)
				}
			}
		}
		ref := ""
		if len(verses) > 0 {
			ref = formatPassageRef(bk.Name, chapter, verses[0].N, verses[len(verses)-1].N)
		}
		if verses == nil {
			verses = []bible.Verse{}
		}
		cols = append(cols, passageCompareTranslation{
			ID:           b.ID,
			Name:         b.Name,
			Abbreviation: b.Abbreviation,
			Reference:    ref,
			Verses:       verses,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"translations": cols,
		"book":         bookKey,
		"chapter":      chapter,
		"verseFrom":    verseFrom,
		"verseTo":      verseTo,
	})
}

// formatPassageRef mirrors bible.formatRef (which is package-private).
// Kept local so we don't have to expose the formatter just for this
// handler.
func formatPassageRef(name string, ch, sv, ev int) string {
	if sv == ev {
		return fmt.Sprintf("%s %d:%d", name, ch, sv)
	}
	return fmt.Sprintf("%s %d:%d-%d", name, ch, sv, ev)
}
