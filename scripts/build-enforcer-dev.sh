#!/usr/bin/env bash
# Builds a development image of the enforcer from the working tree of a local
# checkout, uncommitted changes included, for trying a change to the enforcer
# against the lab before it is released. It is never the pinned enforcer: the
# image is named apart from the pinned one, labelled as a development build
# with the checkout's path, its HEAD, whether the tree was dirty and the tree
# the build hashed, and the binaries report `dev-<tree>`.
#
# The checkout is only read: its tracked and untracked files, without ignored
# ones, are copied into a scratch directory, and git is asked nothing that
# writes, the index included.
#
#   scripts/build-enforcer-dev.sh <checkout>
set -euo pipefail

export GIT_NO_REPLACE_OBJECTS=1
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES \
	GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT

root=$(cd "$(dirname "$0")/.." && pwd)

refuse() {
	echo "build-enforcer-dev: $*" >&2
	exit 1
}

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	[ -n "$value" ] || refuse "versions.env sets no $1"
	printf '%s' "$value"
}

# shellcheck source=scripts/context-tree.sh
. "$root/scripts/context-tree.sh"

if [ $# -ne 1 ] || [ -z "$1" ]; then
	echo "usage: scripts/build-enforcer-dev.sh <checkout>" >&2
	exit 2
fi
source=$(cd "$1" && pwd) || refuse "$1 is not a directory"
git=(git -C "$source" --no-optional-locks)
[ "$("${git[@]}" rev-parse --show-toplevel 2>/dev/null)" = "$(cd "$source" && pwd -P)" ] ||
	refuse "$source is not the top of a git checkout"
base=$("${git[@]}" rev-parse --verify HEAD) || refuse "$source has no commit checked out"
porcelain=$("${git[@]}" status --porcelain --untracked-files=normal) || refuse "git status failed in $source"
dirty=clean
[ -z "$porcelain" ] || dirty=dirty

work=$(mktemp -d "${TMPDIR:-/tmp}/enforcer-dev.XXXXXX")
trap 'rm -rf "$work"' EXIT
context="$work/context"
mkdir "$context"
"${git[@]}" ls-files -z --cached --others --exclude-standard >"$work/listed" ||
	refuse "git ls-files failed in $source"
# A nested repository or a submodule is listed as one entry whose files the
# tree hash would not cover, so two builds of different content would share a
# name; it is refused. A tracked file deleted in the working tree is listed and
# absent, and left out, as the working tree has it.
: >"$work/kept"
while IFS= read -r -d '' path; do
	case "$path" in
	*/) refuse "$source/$path is a nested repository; the build cannot name its content" ;;
	esac
	if [ -d "$source/$path" ] && [ ! -L "$source/$path" ]; then
		refuse "$source/$path is a submodule or a nested repository; the build cannot name its content"
	fi
	if [ -e "$source/$path" ] || [ -L "$source/$path" ]; then printf '%s\0' "$path" >>"$work/kept"; fi
done <"$work/listed"
(cd "$source" && tar --null -T "$work/kept" -cf -) | tar -x -C "$context"
tree=$(context_tree "$context") || refuse "the snapshot of $source could not be hashed"
rm -rf "$context/.git"

short=${tree:0:12}
version="dev-$short"
image="$(pin ENFORCER_IMAGE)-dev:$short"
docker build \
	--file "$root/compose/Dockerfile.enforcer" \
	--build-context "lab=$root/compose/healthprobe" \
	--build-arg "GO_BUILD_IMAGE=$(pin GO_BUILD_IMAGE)" \
	--build-arg "SERVICE_BASE_IMAGE=$(pin SERVICE_BASE_IMAGE)" \
	--build-arg "ENFORCER_COMMIT=$version" \
	--build-arg "ENFORCER_GATEWAY_BIN=$(pin ENFORCER_GATEWAY_BIN)" \
	--build-arg "ENFORCER_CONTROL_BIN=$(pin ENFORCER_CONTROL_BIN)" \
	--label "io.guardana.playground.enforcer.tree=$tree" \
	--label "io.guardana.playground.enforcer.source=development" \
	--label "io.guardana.playground.enforcer.source.path=$source" \
	--label "io.guardana.playground.enforcer.source.head=$base" \
	--label "io.guardana.playground.enforcer.source.state=$dirty" \
	--tag "$image" \
	"$context"

echo "enforcer-dev: $image $(docker image inspect --format '{{.Id}}' "$image") from $source at $base ($dirty), tree $tree"
