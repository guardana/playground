#!/usr/bin/env bash
# Fails when a workflow references an action by anything other than a commit.
set -euo pipefail

cd "$(dirname "$0")/.."

shopt -s nullglob
workflows=(.github/workflows/*.yml)
if [ ${#workflows[@]} -eq 0 ]; then
	echo "actions: no workflows"
	exit 0
fi

status=0
for workflow in "${workflows[@]}"; do
	while IFS= read -r line; do
		reference=$(echo "$line" | sed -nE 's|^[[:space:]]*-?[[:space:]]*uses:[[:space:]]*(.+)$|\1|p')
		[ -n "$reference" ] || continue
		# An action kept in this repository is already pinned by the checkout.
		case "$reference" in ./*) continue ;; esac

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
