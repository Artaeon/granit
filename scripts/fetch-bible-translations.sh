#!/usr/bin/env bash
# fetch-bible-translations.sh — populate optional bible-translation
# JSON files alongside web.json so the scripture page can render
# side-by-side passage diffs across multiple translations.
#
# WHY THIS SCRIPT EXISTS
# ----------------------
# By default the granit binary embeds exactly one bible translation,
# the World English Bible (web.json — public-domain, modern English).
# The backend in internal/scripture/bible supports an arbitrary number
# of sibling translation JSONs; it loads anything matching `*.json` in
# that directory (excluding strongs.json and tagged_*.json, which use
# their own schemas).
#
# This script's job is to drop additional public-domain translations
# into internal/scripture/bible/ in the SAME JSON shape as web.json,
# so the next build picks them up via go:embed.
#
# Target translations (all public domain, all English):
#
#   asv.json   American Standard Version          (1901)
#   kjv.json   King James Version                 (1611/1769)
#   bbe.json   Bible in Basic English             (1965)
#
# Adding more later is just a matter of dropping another file with the
# same schema — no code changes required.
#
# DELIBERATELY NOT AUTO-DOWNLOADING
# ---------------------------------
# Public-domain bible JSON dumps move around constantly: GitHub repos
# get renamed, files get repathed, mirrors disappear. Rather than
# shipping URLs that go stale, this script documents the canonical
# sources + expected schema and leaves the actual download command for
# you to fill in once you've verified the source against your own
# tree-of-trust. Better to ship a stub than a broken curl.
#
# CANONICAL SOURCES (verify before relying on)
# --------------------------------------------
# eBible.org publishes all three translations as raw text + USFM:
#   WEB:  https://ebible.org/web/
#   ASV:  https://ebible.org/asv/
#   KJV:  https://ebible.org/eng-kjv2006/   (note: 2006 typography refresh)
#   BBE:  https://ebible.org/bbe/
#
# GitHub mirrors (each has its own schema — DO NOT assume drop-in
# compatibility; you'll likely need a small jq pass to coerce):
#   - scrollmapper/bible_databases         (SQLite + JSON dumps)
#   - bibleapi/bibleapi-bibles-json        (per-translation JSON, very
#                                            close to our shape)
#   - thiagobodruk/bible                   (JSON per language)
#   - gratis-bible/bible                   (USFM source; needs conversion)
#
# OUTPUT SCHEMA (must match internal/scripture/bible/bible.go)
# ------------------------------------------------------------
#
#   {
#     "id":           "asv",                          // lowercase code, matches filename stem
#     "name":         "American Standard Version",
#     "abbreviation": "ASV",
#     "license":      "Public Domain",
#     "year":         1901,
#     "source":       "https://ebible.org/asv/",
#     "books": [
#       {
#         "code":      "GEN",                         // 3-letter USFM, MUST match web.json
#         "name":      "Genesis",
#         "testament": "OT",                          // "OT" or "NT"
#         "chapters": [
#           {
#             "number": 1,
#             "verses": [
#               { "n": 1, "text": "In the beginning..." },
#               { "n": 2, "text": "..." }
#             ]
#           }
#         ]
#       }
#     ]
#   }
#
# Notes:
#   * `id` is optional in the JSON — if absent the loader uses the
#     filename stem (e.g. asv.json → id "asv"). Better to set it
#     explicitly so the data is self-describing.
#   * `year`, `source` are optional but nice to have for the UI.
#   * Book codes MUST be 3-letter USFM and identical to web.json's
#     codes (GEN, EXO, …, JHN, ROM, …, REV) so cross-translation
#     lookups land on the same book.
#   * Verse numbers MUST be 1-indexed.
#   * The full 66-book canon should be present; missing books just
#     produce empty columns in /bible/passage-compare for those refs.

set -euo pipefail

# --- where this script writes (relative to repo root) -----------------
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/internal/scripture/bible"

ASV_OUT="${OUT_DIR}/asv.json"
KJV_OUT="${OUT_DIR}/kjv.json"
BBE_OUT="${OUT_DIR}/bbe.json"

echo "Repo root:   ${REPO_ROOT}"
echo "Output dir:  ${OUT_DIR}"
echo

if [ ! -d "${OUT_DIR}" ]; then
    echo "error: ${OUT_DIR} does not exist — wrong checkout?" >&2
    exit 1
fi

# Sanity-check the tools any reasonable implementation will use.
# Hard-fail early rather than halfway through.
for tool in curl jq; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "error: required tool '${tool}' is not installed" >&2
        exit 1
    fi
done

# --- TODO #1: fetch + reshape the ASV ---------------------------------
#
# Strategy A: bibleapi/bibleapi-bibles-json hosts per-translation JSON
# dumps that are close to our shape. Pin to a specific commit SHA so
# the embedded bytes are reproducible:
#
#   ASV_RAW_URL="https://raw.githubusercontent.com/bibleapi/bibleapi-bibles-json/<COMMIT_SHA>/asv.json"
#   curl -fsSL "${ASV_RAW_URL}" -o /tmp/asv-raw.json
#
# Their schema typically nests verses under {book, chapter, verse,
# text}. You'll likely need a jq pass to fold into our chapter-list
# shape. Sketch (adjust to the upstream's actual keys):
#
#   jq -r '
#     reduce .[] as $row ({};
#       .books[$row.book_id // $row.book].name //= $row.book_name |
#       .books[$row.book_id // $row.book].code //= $row.book_id |
#       .books[$row.book_id // $row.book].testament //=
#         (if ($row.book_id | tonumber) <= 39 then "OT" else "NT" end) |
#       .books[$row.book_id // $row.book].chapters[$row.chapter | tostring].number = ($row.chapter|tonumber) |
#       .books[$row.book_id // $row.book].chapters[$row.chapter | tostring].verses
#         += [{"n": ($row.verse|tonumber), "text": $row.text}]
#     )
#     | { id: "asv", name: "American Standard Version", abbreviation: "ASV",
#         license: "Public Domain", year: 1901, source: "https://ebible.org/asv/",
#         books: ( .books | to_entries | map(.value)
#                  | map(.chapters = (.chapters | to_entries | map(.value))) ) }
#   ' /tmp/asv-raw.json > "${ASV_OUT}"
#
# Strategy B: download eBible.org's USFM zip and convert via a small
# Python tool (usfm-grammar or pythonbible). Slower but produces the
# cleanest output. Out of scope for this stub.
#
echo "TODO: fetch + reshape ASV → ${ASV_OUT}"

# --- TODO #2: fetch + reshape the KJV ---------------------------------
#
# Same playbook as the ASV. The KJV is the most-mirrored bible on
# GitHub, so you have lots of options. Watch for two pitfalls:
#
#   * Some "KJV" dumps actually ship the 1769 Cambridge typography
#     refresh with modernised spelling — fine for casual reading, but
#     note it in the `source` / `year` field so users can tell.
#   * Apocrypha books sometimes leak in. Strip anything whose USFM
#     code isn't in web.json's 66-book canon, or the loader will
#     report a book mismatch when /bible/passage-compare can't find
#     the same book in WEB.
#
#   KJV_RAW_URL="https://raw.githubusercontent.com/bibleapi/bibleapi-bibles-json/<COMMIT_SHA>/kjv.json"
#   curl -fsSL "${KJV_RAW_URL}" -o /tmp/kjv-raw.json
#   jq '...' /tmp/kjv-raw.json > "${KJV_OUT}"
#
echo "TODO: fetch + reshape KJV → ${KJV_OUT}"

# --- TODO #3: fetch + reshape the BBE ---------------------------------
#
# Bible in Basic English — 1965, public domain, intentionally limited
# vocabulary. Less commonly mirrored than KJV/ASV but still available
# from eBible.org and a couple of GitHub mirrors. Same shape, same
# pitfalls; just slot in your verified URL.
#
#   BBE_RAW_URL="https://..."
#   curl -fsSL "${BBE_RAW_URL}" -o /tmp/bbe-raw.json
#   jq '...' /tmp/bbe-raw.json > "${BBE_OUT}"
#
echo "TODO: fetch + reshape BBE → ${BBE_OUT}"

# --- post-write validation --------------------------------------------
#
# Once you've populated any of the JSONs, this jq sanity-check confirms
# the loader will accept them. Run it manually for each file:
#
#   jq -e '
#     .books|length == 66 and
#     all(.books[]; .code|test("^[A-Z0-9]{3}$") and (.chapters|length > 0))
#   ' "${ASV_OUT}" >/dev/null && echo "${ASV_OUT}: ok"
#
# A non-zero exit means the schema is off; the granit binary will
# refuse to load the file (and log an error at startup).

# --- Reminder: rebuild after populating -------------------------------
#
# go:embed reads the files at compile time, so after this script
# writes new JSONs you need a fresh `make build` (or `go build ./...`)
# for the bytes to land in the binary. The translation chip strip in
# the scripture page only shows what's currently embedded.

echo
echo "Next steps:"
echo "  1. Fill in the TODOs above with verified URLs / commit pins."
echo "  2. Re-run this script (or run individual jq blocks by hand)."
echo "  3. make build   # so go:embed picks up the new JSONs"
echo "  4. Restart granit and check /api/v1/bible/translations"
echo "     — every populated translation should appear in the list."
echo
echo "OPTIONAL — keep the populated JSONs out of git (~5-10 MB each)."
echo "These files are not pre-committed, so .gitignore is the easy lever:"
echo "  for t in asv kjv bbe; do"
echo "    grep -qxF \"internal/scripture/bible/\${t}.json\" .gitignore \\"
echo "      || echo \"internal/scripture/bible/\${t}.json\" >> .gitignore"
echo "  done"
