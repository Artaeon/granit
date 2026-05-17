#!/usr/bin/env bash
# fetch-strongs.sh — populate the Strong's lexicon + tagged bible
# data files that internal/scripture/bible expects.
#
# WHY THIS SCRIPT EXISTS
# ----------------------
# The bible reader supports a "tap a Greek/Hebrew word to see a word
# study" feature. That feature needs two big-ish JSON files:
#
#   internal/scripture/bible/strongs.json       (~50MB)
#       Strong's Concordance lexicon — Greek (G####) + Hebrew (H####)
#       entries with lemma, transliteration, definition, KJV gloss,
#       derivation.
#
#   internal/scripture/bible/tagged_kjv.json    (~30MB)
#       The KJV with every translatable word annotated with its
#       underlying Strong's code, so the reader knows what to look up.
#
# Both files are PUBLIC DOMAIN but neither is checked in (they'd
# double the repo size). Placeholder "{}" files sit in their place so
# `go:embed` succeeds — the loaders detect the placeholder and report
# "not bundled" gracefully. Run this script when you want the real
# feature enabled; rebuild afterward so the new bytes get embedded.
#
# CANONICAL SOURCES
# -----------------
#   Strong's lexicon (Greek + Hebrew):
#     openscriptures/strongs on GitHub
#       https://github.com/openscriptures/strongs
#     Public-domain, community-maintained, BBE-licensed XML + JSON
#     dumps. The JSON layouts under their `*-dictionary-json/`
#     directories are close to what we want; you may need to merge
#     the Greek + Hebrew halves and re-key into a single map.
#
#   Tagged KJV:
#     scrollmapper/bible_databases on GitHub
#       https://github.com/scrollmapper/bible_databases
#     Includes a Strong's-tagged KJV in several formats (SQLite,
#     JSON). The JSON variant under `formats/json/KJV.json` is a
#     reasonable starting point but doesn't ship per-word Strong's
#     tags by default — you'll likely need the OSIS or SQLite source
#     and a small conversion step.
#
# This script DELIBERATELY does not auto-download. URLs change, repo
# layouts shift, and the user gets to choose which exact dataset to
# commit to. Fill in the TODOs below with the URLs you've verified
# against your tree-of-trust, then re-run.
#
# OUTPUT SCHEMAS (what the loaders expect)
# ----------------------------------------
# strongs.json:
#   {
#     "G1722": {
#       "lemma": "ἐν",
#       "translit": "en",
#       "strongs_def": "a primary preposition denoting (fixed) position…",
#       "kjv_def": "in, on, at, by",
#       "derivation": "a primary preposition"
#     },
#     "H7225": { ... },
#     ...
#   }
#
# tagged_kjv.json:
#   {
#     "name": "King James Version (Strong's tagged)",
#     "abbreviation": "KJV",
#     "license": "Public Domain",
#     "source": "<wherever you got it>",
#     "books": [
#       {
#         "code": "JHN",
#         "name": "John",
#         "chapters": [
#           {
#             "number": 3,
#             "verses": [
#               {
#                 "n": 16,
#                 "words": [
#                   {"text": "For", "strongs": "G1063"},
#                   {"text": "God", "strongs": "G2316"},
#                   {"text": "so", "strongs": "G3779"},
#                   ...
#                 ]
#               }
#             ]
#           }
#         ]
#       }
#     ]
#   }
#
# IMPORTANT: book codes MUST be the 3-letter USFM codes (GEN, EXO, …,
# JHN, ROM, …, REV) and match what's in web.json so cross-lookups
# work. Verse numbers MUST be 1-indexed.

set -euo pipefail

# --- where this script writes (relative to repo root) -----------------
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/internal/scripture/bible"
STRONGS_OUT="${OUT_DIR}/strongs.json"
TAGGED_OUT="${OUT_DIR}/tagged_kjv.json"

echo "Repo root:   ${REPO_ROOT}"
echo "Output dir:  ${OUT_DIR}"
echo

if [ ! -d "${OUT_DIR}" ]; then
    echo "error: ${OUT_DIR} does not exist — wrong checkout?" >&2
    exit 1
fi

# Sanity-check the tools we'll need for any reasonable implementation
# of the TODOs below. Hard-fail early rather than halfway through.
for tool in curl jq; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "error: required tool '${tool}' is not installed" >&2
        exit 1
    fi
done

# --- TODO #1: fetch + assemble the Strong's lexicon -------------------
#
# Pick one of these strategies (or roll your own) and replace the
# `echo "TODO"` placeholders with real commands:
#
# (a) Grab the openscriptures Greek + Hebrew JSON dumps separately and
#     merge them into one file:
#       GREEK_URL=https://raw.githubusercontent.com/openscriptures/strongs/master/greek/strongs-greek-dictionary.json
#       HEBREW_URL=https://raw.githubusercontent.com/openscriptures/strongs/master/hebrew/strongs-hebrew-dictionary.json
#     NOTE: Verify those paths against the upstream repo — they have
#     historically moved between commits. Inspect the directory tree
#     on the repo's default branch before relying on them.
#
# (b) Download a pre-merged community fork (search GitHub for
#     "strongs-dictionary.json" — several mirrors exist). Pin to a
#     specific commit SHA rather than `master` so the bytes embedded
#     in your binary are reproducible.
#
# After download, the schema may need re-shaping with jq to match
# what strongs.go expects (`lemma`, `translit`, `strongs_def`,
# `kjv_def`, `derivation` keyed by "G####"/"H####"). Example pattern:
#
#   jq -s '
#     ( .[0] | to_entries | map({key: .key, value: {
#         lemma:       .value.lemma,
#         translit:    .value.translit,
#         strongs_def: .value.strongs_def,
#         kjv_def:     .value.kjv_def,
#         derivation:  .value.derivation
#       }}) | from_entries )
#     + ( .[1] | ... same ... )
#   ' greek-raw.json hebrew-raw.json > "${STRONGS_OUT}"
#
echo "TODO: fetch + assemble Strong's lexicon → ${STRONGS_OUT}"
echo "      ~50MB once populated"

# --- TODO #2: fetch + assemble the tagged KJV -------------------------
#
# Hardest part of the job: most public-domain KJV dumps are NOT
# pre-tagged with Strong's codes at word granularity. Options:
#
# (a) scrollmapper/bible_databases ships a SQLite DB with a
#     `kjv_strongs` table or similar — query it, group by
#     book/chapter/verse, and emit the JSON shape above. URL is
#     unstable; check the repo and pin a commit.
#
# (b) Use the STEPBible/Tyndale "translators-amalgamated" tagged text
#     (CC-BY) — richer tags but a different licence than PD.
#
# (c) Convert the OSIS XML KJV (with embedded <w lemma="strong:G1722">
#     elements) — slowest but produces the cleanest output.
#
# Whatever the source, the output MUST use 3-letter USFM book codes
# matching internal/scripture/bible/web.json. A simple verification
# step: every book.code in tagged_kjv.json should also appear in
# web.json's book list.
#
echo "TODO: fetch + assemble tagged KJV → ${TAGGED_OUT}"
echo "      ~30MB once populated"

# --- Reminder: rebuild after populating -------------------------------
#
# go:embed reads the files at compile time, so after this script
# writes the real JSONs you need a fresh `make build` (or
# `go build ./...`) for the bytes to land in the binary.
#
echo
echo "Next steps:"
echo "  1. Fill in the TODOs above with verified URLs / commit pins."
echo "  2. Re-run this script."
echo "  3. make build   # so go:embed picks up the new JSON"
echo "  4. Restart granit and check /api/v1/bible/strongs/status"
echo "     — both fields should now be true."
echo
echo "IMPORTANT — stop git from tracking the populated data (~80MB)."
echo "The placeholder files are committed so the build always succeeds;"
echo "after populating, mark them skip-worktree so a git add won't drag"
echo "the lexicon into a commit:"
echo "  git update-index --skip-worktree \\"
echo "    internal/scripture/bible/strongs.json \\"
echo "    internal/scripture/bible/tagged_kjv.json"
echo "  # to restore tracking later: --no-skip-worktree"
