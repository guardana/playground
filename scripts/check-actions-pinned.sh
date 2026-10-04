#!/usr/bin/env bash
# Fails when a workflow or an action kept in this repository references an
# action by anything other than a commit, and when it finds no file to read.
set -euo pipefail

cd "$(dirname "$0")/.."

workflows=()
while IFS= read -r f; do
	workflows+=("$f")
done < <(scripts/repo-files.sh | grep -a -E '^\.github/(workflows/[^/]+|actions/.+/action)\.ya?ml$' || true)
if [ ${#workflows[@]} -eq 0 ]; then
	echo "actions: no workflow or action file found; nothing was checked" >&2
	exit 1
fi

listed() {
	local f
	for f in "${workflows[@]}"; do
		[ "$f" = "$1" ] && return 0
	done
	return 1
}

status=0
for workflow in "${workflows[@]}"; do
	while IFS= read -r line; do
		trimmed=${line#"${line%%[![:space:]]*}"}
		case "$trimmed" in '#'*) continue ;; esac
		reference=$(echo "$line" | sed -nE 's|^[[:space:]]*-?[[:space:]]*uses:[[:space:]]*(.+)$|\1|p')
		if [ -z "$reference" ]; then
			# A uses in any other form, a flow mapping, a quoted or a complex key,
			# would pass unread, so any other line naming it fails.
			if echo "$line" | grep -qE '(^|[^[:alnum:]_-])uses([^[:alnum:]_-]|$)'; then
				echo "$workflow: a uses: this check cannot read: $trimmed" >&2
				status=1
			fi
			continue
		fi
		# An action kept in this repository is pinned by the checkout; its own
		# file has to be one this check reads.
		case "$reference" in
		./*)
			action=${reference#./}
			action=${action%%[[:space:]]*}
			action=${action%/}
			if ! listed "$action/action.yml" && ! listed "$action/action.yaml"; then
				echo "$workflow: $reference names no action file under .github/actions/" >&2
				status=1
			fi
			continue
			;;
		esac

		case "$reference" in
		*@0000000000000000000000000000000000000000*)
			echo "$workflow: placeholder left in place: $reference" >&2
			status=1
			;;
		esac
		if ! echo "$reference" | grep -qE '^[^@ ]+@[0-9a-f]{40} # pin: v[0-9A-Za-z.+-]+$'; then
			echo "$workflow: not pinned to a commit with its tag noted: $reference" >&2
			status=1
		fi
	done <"$workflow"
done

[ "$status" -eq 0 ] && echo "actions: pinned"
exit $status
