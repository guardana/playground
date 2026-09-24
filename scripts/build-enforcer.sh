#!/usr/bin/env bash
# Builds the enforcer image from the commit versions.env pins, taken with
# `git archive` from the clone ENFORCER_SOURCE names. The working tree of that
# clone is never read, so an edit or a checkout beside the lab cannot reach a
# run. The extracted archive must hash to the commit's own tree, or the build is
# refused, and so is a commit whose tree is not ENFORCER_TREE. The image is
# tagged with the commit, labelled with it and with the tree it verified; the
# runner reads them back and refuses an enforcer image without that tree.
set -euo pipefail

# Replacement objects would let every git call below read another object than
# the one the commit id names.
export GIT_NO_REPLACE_OBJECTS=1
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES \
	GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT

root=$(cd "$(dirname "$0")/.." && pwd)

refuse() {
	echo "build-enforcer: $*" >&2
	exit 1
}

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	[ -n "$value" ] || refuse "versions.env sets no $1"
	printf '%s' "$value"
}

# context_tree hashes the extracted archive the way git hashes a commit's tree,
# with no conversion: neither the building machine's configuration nor the
# archive's own .gitattributes may normalise a file back to what the commit
# holds.
context_tree() {
	local run=(env GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
		git -C "$1" -c core.autocrlf=false -c core.attributesFile=/dev/null)
	"${run[@]}" init -q
	printf '* -text -eol -filter -ident -working-tree-encoding\n' >"$1/.git/info/attributes"
	"${run[@]}" add -A -f
	"${run[@]}" write-tree
}

if [ -z "${ENFORCER_SOURCE:-}" ]; then
	echo "build-enforcer: set ENFORCER_SOURCE to a clone of the enforcer's repository" >&2
	exit 2
fi

commit=$(pin ENFORCER_COMMIT)
case "$commit" in
*[!0-9a-f]*) refuse "ENFORCER_COMMIT is not a full lowercase commit id: $commit" ;;
esac
[ "${#commit}" -eq 40 ] || refuse "ENFORCER_COMMIT is not a full commit id: $commit"
git -C "$ENFORCER_SOURCE" cat-file -e "${commit}^{commit}" 2>/dev/null ||
	refuse "$ENFORCER_SOURCE holds no commit $commit; fetch it first"
replaced=$(git -C "$ENFORCER_SOURCE" for-each-ref --format='%(refname)' refs/replace/)
[ -z "$replaced" ] || refuse "$ENFORCER_SOURCE carries replacement refs, remove them: $replaced"
tree=$(git -C "$ENFORCER_SOURCE" rev-parse --verify "${commit}^{tree}")
pinned_tree=$(pin ENFORCER_TREE)
[ "$tree" = "$pinned_tree" ] ||
	refuse "commit $commit has tree $tree, not the ENFORCER_TREE versions.env pins: $pinned_tree"

image="$(pin ENFORCER_IMAGE):$commit"
go_image=$(pin GO_BUILD_IMAGE)
base_image=$(pin SERVICE_BASE_IMAGE)
gateway_bin=$(pin ENFORCER_GATEWAY_BIN)
control_bin=$(pin ENFORCER_CONTROL_BIN)
context=$(mktemp -d "${TMPDIR:-/tmp}/enforcer-source.XXXXXX")
trap 'rm -rf "$context"' EXIT
git -C "$ENFORCER_SOURCE" archive --format=tar "$commit" | tar -x -C "$context"
extracted=$(context_tree "$context")
[ "$extracted" = "$tree" ] ||
	refuse "the archive of $commit hashes to tree $extracted, not its own tree $tree; an attribute or a filter changed it"
rm -rf "$context/.git"

docker build \
	--file "$root/compose/Dockerfile.enforcer" \
	--build-context "lab=$root/compose/healthprobe" \
	--build-arg "GO_BUILD_IMAGE=$go_image" \
	--build-arg "SERVICE_BASE_IMAGE=$base_image" \
	--build-arg "ENFORCER_COMMIT=$commit" \
	--build-arg "ENFORCER_GATEWAY_BIN=$gateway_bin" \
	--build-arg "ENFORCER_CONTROL_BIN=$control_bin" \
	--label "io.guardana.playground.enforcer.tree=$extracted" \
	--tag "$image" \
	"$context"

echo "enforcer: $image $(docker image inspect --format '{{.Id}}' "$image")"
