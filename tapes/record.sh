#!/usr/bin/env bash
# record.sh — Record all VHS tape files into GIF assets.
#
# Prerequisites:
#   - VHS installed:     go install github.com/charmbracelet/vhs@latest
#   - ffmpeg installed:  sudo apt install ffmpeg  (or brew install ffmpeg)
#   - ttyd installed:    VHS handles this, but check: https://github.com/tsl0922/ttyd
#   - Granit built:      ~/go/bin/granit (go install ./cmd/granit/)
#   - demo-vault/ exists in the project root
#
# Usage:
#   ./tapes/record.sh            # record all tapes
#   ./tapes/record.sh hero       # record a single tape by name (without .tape extension)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_DIR"

# Verify prerequisites
if ! command -v vhs &>/dev/null && ! ~/go/bin/vhs --version &>/dev/null 2>&1; then
    echo "ERROR: VHS is not installed."
    echo "  Install with: go install github.com/charmbracelet/vhs@latest"
    exit 1
fi

VHS_BIN="vhs"
command -v vhs &>/dev/null || VHS_BIN="$HOME/go/bin/vhs"

if ! command -v ffmpeg &>/dev/null; then
    echo "ERROR: ffmpeg is not installed. VHS requires ffmpeg for GIF encoding."
    echo "  Install with: sudo apt install ffmpeg  (or: brew install ffmpeg)"
    exit 1
fi

if [[ ! -d demo-vault ]]; then
    echo "ERROR: demo-vault/ directory not found at $PROJECT_DIR"
    exit 1
fi

if [[ ! -x ~/go/bin/granit ]] && ! command -v granit &>/dev/null; then
    echo "ERROR: granit binary not found. Build with: go install ./cmd/granit/"
    exit 1
fi

mkdir -p assets

# Record a single tape or all tapes
if [[ $# -gt 0 ]]; then
    tape="tapes/$1.tape"
    if [[ ! -f "$tape" ]]; then
        echo "ERROR: Tape file not found: $tape"
        exit 1
    fi
    echo "Recording $tape ..."
    "$VHS_BIN" "$tape"
    echo "Done: $tape"
else
    failed=0
    for tape in tapes/*.tape; do
        echo "Recording $tape ..."
        if "$VHS_BIN" "$tape"; then
            echo "  OK: $tape"
        else
            echo "  FAILED: $tape"
            failed=$((failed + 1))
        fi
    done
    echo ""
    if [[ $failed -gt 0 ]]; then
        echo "$failed tape(s) failed to record."
        exit 1
    else
        echo "All tapes recorded successfully."
    fi
fi
