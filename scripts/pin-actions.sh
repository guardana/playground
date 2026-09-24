#!/usr/bin/env bash
# Replaces the placeholder in each `uses:` line with the commit the named tag
# points at.
#
# A tag is a mutable pointer. CI runs with access worth protecting, so what runs
# is named by commit and the tag survives only as a comment saying which release
# that commit was.
set -euo pipefail

cd "$(dirname "$0")/.."

command -v gh >/dev/null || {
	echo "needs the GitHub CLI, authenticated" >&2
	exit 1
}

commit_for() { # owner/repo tag
	local reference type sha
	reference=$(gh api "repos/$1/git/ref/tags/$2" --jq '{type: .object.type, sha: .object.sha}')
	type=$(echo "$reference" | sed -n 's/.*"type":"\([^"]*\)".*/\1/p')
	sha=$(echo "$reference" | sed -n 's/.*"sha":"\([^"]*\)".*/\1/p')
	# An annotated tag points at a tag object; the commit is one hop further.
	if [ "$type" = "tag" ]; then
		sha=$(gh api "repos/$1/git/tags/$sha" --jq '.object.sha')
	fi
	echo "$sha"
}

for workflow in .github/workflows/*.yml; do
	while IFS= read -r line; do
		repo=$(echo "$line" | sed -nE 's|.*uses: ([^@ ]+)@[0-9a-f]{40} # pin: (v[0-9A-Za-z.+-]+).*|\1|p')
		tag=$(echo "$line" | sed -nE 's|.*uses: ([^@ ]+)@[0-9a-f]{40} # pin: (v[0-9A-Za-z.+-]+).*|\2|p')
		[ -n "$repo" ] && [ -n "$tag" ] || continue

		# Subdirectory actions live in the repository named by the first two segments.
		owner_repo=$(echo "$repo" | cut -d/ -f1,2)
		sha=$(commit_for "$owner_repo" "$tag")
		[ "${#sha}" -eq 40 ] || {
			echo "could not resolve $owner_repo@$tag" >&2
			exit 1
		}

		escaped=$(printf '%s' "$repo" | sed 's|/|\\/|g')
		sed -i.bak -E "s|(uses: ${escaped}@)[0-9a-f]{40}( # pin: ${tag//./\\.})|\1${sha}\2|" "$workflow"
		rm -f "$workflow.bak"
		echo "pinned $repo@$tag -> $sha"
	done <"$workflow"
done
