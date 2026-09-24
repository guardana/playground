#!/usr/bin/env bash
# Prints every file this repository owns, one path per line.
#
# In a work tree: tracked files and untracked ones git does not ignore, so a
# file is checked before it is staged and a violation a guard's own test plants
# is one the guard can see. Local tooling stays out through .git/info/exclude.
# Without git, in an export, find mirrors .gitignore plus the rule that a hidden
# directory is local tooling and never ours, with .github as the one exception.
set -euo pipefail

cd "$(dirname "$0")/.."

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	git ls-files --cached --others --exclude-standard |
		while IFS= read -r f; do
			if [ -e "$f" ]; then printf '%s\n' "$f"; fi
		done | sort -u
	exit 0
fi

find . \
	-type d \( -name '.?*' ! -name '.github' \) -prune -o \
	-type d \( -path './dist' -o -path './bin' -o -path './bench/results' -o -path './reports' \) -prune -o \
	-type f ! -name '.DS_Store' ! -name '*.bak' ! -name '*.orig' ! -name '*.local.md' \
	-print |
	sed 's|^\./||' | sort
