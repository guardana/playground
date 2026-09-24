#!/usr/bin/env bash
# Fails when something that is not the product has entered the repository.
#
# Three objective rules: no file whose name marks it as working material, no
# text outside English, no placeholder left behind. Style is not checked here;
# it is a review question.
set -euo pipefail

cd "$(dirname "$0")/.."

# The scans below read the list through a pipe that hides its exit status, so a
# list that could not be made fails here rather than scanning nothing.
scripts/repo-files.sh >/dev/null

status=0
note() {
	echo "$1" >&2
	status=1
}

# The check scripts name the strings they forbid.
files() { scripts/repo-files.sh | grep -vE '^scripts/(check-.*\.sh|.*-allowlist\.txt)$'; }

# Working material belongs outside the repository.
while IFS= read -r f; do
	case "$f" in
	*.local.md | *.bak | *.orig | *.rej | *.DS_Store) note "junk file: $f" ;;
	PROMPT-* | SPEC-* | *-report.md) note "working material: $f" ;;
	reports/* | */reports/* | scratch/* | */scratch/* | notes/* | */notes/*) note "working material: $f" ;;
	esac
done < <(files)

# A hidden path is local tooling unless it is one of the project's own dotfiles.
while IFS= read -r f; do
	case "$f" in
	.git* | .editorconfig | .golangci.yml | .dockerignore) ;;
	.*) note "local tooling must not be tracked: $f" ;;
	esac
done < <(files)

# English only.
while IFS= read -r hit; do
	note "not English: $hit"
done < <(files | tr '\n' '\0' | xargs -0 grep -nIE '[ąćęłńóśźżĄĆĘŁŃÓŚŹŻ]' 2>/dev/null || true)

# Placeholders that were meant to be replaced before the change landed.
while IFS= read -r hit; do
	note "placeholder: $hit"
done < <(
	files | grep -E '\.(md|go|ya?ml|proto|json|sh)$' |
		tr '\n' '\0' | xargs -0 grep -nIE '\b(TBD|FIXME|XXX|lorem ipsum)\b|<placeholder>' 2>/dev/null || true
)

if [ "$status" -ne 0 ]; then
	exit 1
fi

echo "hygiene: clean"
