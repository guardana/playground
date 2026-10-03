#!/usr/bin/env bash
# Fails when something that is not the product has entered the repository.
#
# Four objective rules: no file whose name marks it as working material, no
# text outside English, no placeholder left behind, and no path that exists only
# on a maintainer's machine, since a stranger's clone has no sibling checkout and
# no such home directory. Style is not checked here; it is a review question.
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
files() { scripts/repo-files.sh | grep -a -vE '^scripts/(check-.*\.sh|.*-allowlist\.txt)$'; }

# The scans below hide grep's errors, and grep cannot read a file without the
# permission to, so such a file is refused here rather than passed unread. They
# read every file as text (-a): grep would skip one it takes for binary.
while IFS= read -r f; do
	[ -r "$f" ] || note "unreadable: $f"
done < <(files)

# Working material belongs outside the repository.
while IFS= read -r f; do
	case "$f" in
	*.local.md | *.bak | *.orig | *.rej | *.DS_Store) note "junk file: $f" ;;
	PROMPT-* | SPEC-* | *-report.md) note "working material: $f" ;;
	reports/* | */reports/* | scratch/* | */scratch/* | notes/* | */notes/*) note "working material: $f" ;;
	esac
done < <(files)

# A hidden path is local tooling unless it is one of the project's own dotfiles
# at the root, named one by one: a hidden name below the root is tooling too.
while IFS= read -r f; do
	case "$f" in
	.github/.* | .github/*/.*) note "local tooling must not be tracked: $f" ;;
	.github/* | .gitignore | .gitattributes | .editorconfig | .golangci.yml | .dockerignore) ;;
	.* | */.*) note "local tooling must not be tracked: $f" ;;
	esac
done < <(files)

# English only.
while IFS= read -r hit; do
	note "not English: $hit"
done < <(files | tr '\n' '\0' | xargs -0 grep -naE -e '[ąćęłńóśźżĄĆĘŁŃÓŚŹŻ]' -- 2>/dev/null || true)

# Placeholders that were meant to be replaced before the change landed.
while IFS= read -r hit; do
	note "placeholder: $hit"
done < <(
	files | grep -a -E '\.(md|go|ya?ml|proto|json|sh)$' |
		tr '\n' '\0' | xargs -0 grep -naE -e '\b(TBD|FIXME|XXX|lorem ipsum)\b|<placeholder>' -- 2>/dev/null || true
)

# Paths only a maintainer's machine has: a home directory, the macOS /tmp and
# per-user temporary directory, a sibling checkout of a system under test. A
# path counts only where one starts, so a URL segment does not; the distroless
# images' own home, /home/nonroot, is every stranger's too.
start='(^|[^A-Za-z0-9._~-])'
name_end='(/|$|[^A-Za-z0-9._-])'
# The home alternative takes the whole name and no character after it, so a
# second path right after /home/nonroot still finds its start.
machine_path="${start}/Users/|${start}/home/[A-Za-z0-9._-]+|${start}/private/(tmp|var/folders)${name_end}"
sibling_end='(/|$|[^A-Za-z0-9_-])'
machine_path="$machine_path|${start}/var/folders/|\.\./(control|guardana)${sibling_end}"
while IFS= read -r hit; do
	note "maintainer path: $hit"
done < <(
	files | tr '\n' '\0' | xargs -0 grep -noaE -e "$machine_path" -- 2>/dev/null |
		grep -a -vE '/home/nonroot$' || true
)

if [ "$status" -ne 0 ]; then
	exit 1
fi

echo "hygiene: clean"
