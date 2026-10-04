#!/usr/bin/env bash
# Sourced by the scripts that run the enforcer's image on a bind mount.
#
# container_user prints the --user under which what that container writes on
# a bind mount belongs to whoever runs the script: their own ids where the
# daemon keeps them, root under rootless Docker, where the container's root is
# that user. Root is never tried on a daemon that does not say it is rootless,
# and a mapping under which neither holds is refused rather than guessed, since
# the enforcer refuses a key another account owns.

not_the_callers="the file it wrote belongs to another uid"

file_owner() { # path, not followed through a link
	[ -f "$1" ] && [ ! -L "$1" ] || return 1
	if stat -c %u / >/dev/null 2>&1; then
		stat -c %u "$1"
	else
		stat -f %u "$1"
	fi
}

# probe_as succeeds, printing nothing, when the enforcer's keygen run as user
# writes a key this user owns; otherwise it prints why. The scratch directory
# goes with the subshell, whatever stops it.
probe_as() { # image user
	(
		scratch=$(mktemp -d "${TMPDIR:-/tmp}/lab-user.XXXXXX")
		trap 'rm -rf "$scratch"' EXIT
		if ! said=$(docker run --rm --pull never --network none --read-only --cap-drop ALL \
			--security-opt no-new-privileges --user "$2" -v "$scratch:/probe" \
			--entrypoint /enforcer/control "$1" policy keygen --out /probe/key 2>&1 >/dev/null); then
			printf '%s' "${said:-docker run failed}"
			exit 1
		fi
		if [ "$(file_owner "$scratch/key/signing.key" || true)" != "$(id -u)" ]; then
			printf '%s' "$not_the_callers"
			exit 1
		fi
	)
}

container_user() { # image
	local own own_said root_said
	own="$(id -u):$(id -g)"
	if own_said=$(probe_as "$1" "$own"); then
		printf '%s' "$own"
		return 0
	fi
	if ! docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -q 'name=rootless'; then
		echo "the user probe as $own: $own_said" >&2
		return 1
	fi
	if root_said=$(probe_as "$1" 0:0); then
		printf '%s' 0:0
		return 0
	fi
	if [ "$own_said" = "$not_the_callers" ] && [ "$root_said" = "$not_the_callers" ]; then
		echo "under rootless Docker neither $own nor 0:0 writes files this user owns; the daemon maps users in a way the lab does not know" >&2
	else
		echo "the user probe as $own: $own_said; as 0:0: $root_said" >&2
	fi
	return 1
}
