#!/usr/bin/env bash
# Prints every file this repository owns, one path per line.
#
# In its own work tree: tracked files and untracked ones git does not ignore,
# so a file is checked before it is staged and a violation a guard's own test
# plants is one the guard can see. Local tooling stays out through
# .git/info/exclude. Anywhere else, an export or a copy inside another
# repository, find mirrors .gitignore plus the rule that a hidden directory is
# local tooling and never ours, with .github as the one exception. An empty
# list is an error: a guard reading it would inspect nothing and report clean.
set -euo pipefail

cd "$(dirname "$0")/.."

list() {
	if [ "$(git rev-parse --show-toplevel 2>/dev/null)" = "$(pwd -P)" ]; then
		git ls-files --cached --others --exclude-standard |
			while IFS= read -r f; do
				if [ -e "$f" ]; then printf '%s\n' "$f"; fi
			done | sort -u
		return
	fi
	find . \
		-type d \( -name '.?*' ! -name '.github' \) -prune -o \
		-type d \( -path './dist' -o -path './bin' -o -path './bench/results' -o -path './reports' \) -prune -o \
		-type f ! -name '.DS_Store' ! -name '*.bak' ! -name '*.orig' ! -name '*.local.md' \
		-print |
		sed 's|^\./||' | sort
}

listed=$(list)
if [ -z "$listed" ]; then
	echo "repo-files: no file listed in $(pwd -P)" >&2
	exit 1
fi
printf '%s\n' "$listed"
