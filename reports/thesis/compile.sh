#!/bin/bash
# =============================================================================
# Thesis compilation script
# Usage: bash compile.sh
# =============================================================================

set -e

MAIN="thesis"
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# TeX Live path
export PATH="/usr/local/texlive/2025/bin/universal-darwin:$PATH"

echo "=== Compiling thesis ==="

# First pass
pdflatex -interaction=nonstopmode "$MAIN.tex"

# Bibliography
bibtex "$MAIN"

# Second and third pass (resolve references)
pdflatex -interaction=nonstopmode "$MAIN.tex"
pdflatex -interaction=nonstopmode "$MAIN.tex"

echo ""
echo "=== Done: $MAIN.pdf ==="
