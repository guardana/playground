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

# Names arrive NUL-separated, so git quotes none of them and no byte in a name
# hides it from a guard. A name holding a line break cannot be printed one per
# line, and git lists a nested repository or a submodule as a directory without
# its files, so either fails the list.
one_per_line() {
	local f
	while IFS= read -r -d '' f; do
		f=${f#./}
		case "$f" in
		*$'\n'*)
			echo "repo-files: a file name holds a line break: $(printf '%q' "$f")" >&2
			return 1
			;;
		esac
		if [ -d "$f" ] && [ ! -L "$f" ]; then
			echo "repo-files: ${f%/} is a nested repository; no guard can read its files" >&2
			return 1
		fi
		if [ -e "$f" ] || [ -L "$f" ]; then printf '%s\n' "$f"; fi
	done
}

list() {
	if [ "$(git rev-parse --show-toplevel 2>/dev/null)" = "$(pwd -P)" ]; then
		git ls-files -z --cached --others --exclude-standard | one_per_line | sort -u
		return
	fi
	find . \
		-type d \( -name '.?*' ! -name '.github' \) -prune -o \
		-type d \( -path './dist' -o -path './bin' -o -path './reports' \) -prune -o \
		\( -type f -o -type l \) ! -name '.DS_Store' ! -name '*.bak' ! -name '*.orig' ! -name '*.local.md' \
		-print0 | one_per_line | sort
}

listed=$(list)
if [ -z "$listed" ]; then
	echo "repo-files: no file listed in $(pwd -P)" >&2
	exit 1
fi
printf '%s\n' "$listed"
