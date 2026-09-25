#!/usr/bin/env bash
# Fetches the enforcer commit versions.env pins from ENFORCER_REPOSITORY into a
# new bare repository at <dir>, the clone ENFORCER_SOURCE then names for
# scripts/build-enforcer.sh. It fetches that one commit by its id, shallow and
# anonymously: no credential helper, no prompt and no user or system git
# configuration take part, so what it fetches is what anyone would. It fails
# with the reason and what to do when the commit cannot be fetched; a run
# without the enforcer at its pin measures nothing, so it never skips.
#
#   scripts/fetch-enforcer.sh <dir>
set -euo pipefail

export GIT_TERMINAL_PROMPT=0 GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_NO_REPLACE_OBJECTS=1
unset GIT_ASKPASS SSH_ASKPASS GIT_DIR GIT_WORK_TREE GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT

root=$(cd "$(dirname "$0")/.." && pwd)

refuse() {
	echo "fetch-enforcer: $*" >&2
	exit 1
}

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	[ -n "$value" ] || refuse "versions.env sets no $1"
	printf '%s' "$value"
}

if [ $# -ne 1 ] || [ -z "$1" ]; then
	echo "usage: scripts/fetch-enforcer.sh <dir>" >&2
	exit 2
fi
dir=$1
if [ -e "$dir" ] && [ -n "$(ls -A "$dir")" ]; then
	refuse "$dir exists and is not empty; name a new directory"
fi

repository=$(pin ENFORCER_REPOSITORY)
case "$repository" in
https://*@* | http://*@*) refuse "ENFORCER_REPOSITORY carries credentials; name the repository alone: $repository" ;;
https://*) ;;
*) refuse "ENFORCER_REPOSITORY is not an https URL: $repository" ;;
esac
commit=$(pin ENFORCER_COMMIT)
case "$commit" in
*[!0-9a-f]*) refuse "ENFORCER_COMMIT is not a full lowercase commit id: $commit" ;;
esac
[ "${#commit}" -eq 40 ] || refuse "ENFORCER_COMMIT is not a full commit id: $commit"
tree=$(pin ENFORCER_TREE)

git=(git -c credential.helper= -c core.askPass= -c protocol.version=2 -c http.followRedirects=false)
"${git[@]}" init -q --bare "$dir"
if ! "${git[@]}" -C "$dir" fetch --quiet --depth 1 --no-tags "$repository" "$commit"; then
	refuse "$repository does not serve commit $commit anonymously (git's reason is above).
The enforcer's commit has to be published where ENFORCER_REPOSITORY points before
CI can build it: publish it there, or point ENFORCER_REPOSITORY at a public
repository that holds it. By hand, set ENFORCER_SOURCE to any clone holding the
commit instead of running this script."
fi
"${git[@]}" -C "$dir" cat-file -e "${commit}^{commit}" ||
	refuse "the fetch from $repository succeeded and $dir holds no commit $commit"
fetched=$("${git[@]}" -C "$dir" rev-parse --verify "${commit}^{tree}")
[ "$fetched" = "$tree" ] ||
	refuse "commit $commit from $repository has tree $fetched, not the ENFORCER_TREE versions.env pins: $tree"
echo "fetch-enforcer: $commit (tree $tree) from $repository in $dir"
