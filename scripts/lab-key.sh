#!/usr/bin/env bash
# Makes the lab's policy signing key on this machine, once, with the pinned
# enforcer's own `policy keygen`. The key lives outside the clone and outside
# every image: the runner mounts it read-only into the one-shot container that
# signs a scenario's bundle, never into the gateway. keygen's two printed lines,
# the key id and the public key, are public and kept beside the key for the
# runner to hand the gateway. An existing key is reported, never replaced.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)

refuse() {
	echo "lab-key: $*" >&2
	exit 1
}

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	[ -n "$value" ] || refuse "versions.env sets no $1"
	printf '%s' "$value"
}

dir="${LAB_KEYS_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/guardana-playground/lab-key}"
case "$dir" in
/*) ;;
*) dir="$PWD/$dir" ;;
esac
# A . component makes mv put the key below the place named.
case "/$dir/" in
*/../* | */./*) refuse "LAB_KEYS_DIR holds a . or .. component; name the directory without one: $dir" ;;
esac
# $(...) drops trailing line breaks, so a path holding one would be checked as
# another directory than the one the key is written to.
case "$dir" in
*[[:cntrl:]]*) refuse "LAB_KEYS_DIR holds a control character: $(printf '%q' "$dir")" ;;
esac
# The deepest existing directory of the path is resolved to its real place,
# and each directory above that is compared with the clone by device and inode,
# so neither a link nor another spelling of a directory (a case-insensitive
# file system, a second name for a volume) can put the key inside it.
file_id() {
	if stat -c '%d:%i' / >/dev/null 2>&1; then
		stat -c '%d:%i' "$1"
	else
		stat -f '%d:%i' "$1"
	fi
}
clone=$(file_id "$root")
at=$dir
while [ ! -e "$at" ]; do at=$(dirname "$at"); done
[ -d "$at" ] || refuse "LAB_KEYS_DIR lies under $at, which is not a directory"
at=$(cd "$at" && pwd -P)
while :; do
	if [ "$(file_id "$at")" = "$clone" ]; then
		refuse "LAB_KEYS_DIR is inside the clone: $dir"
	fi
	[ "$at" = / ] && break
	at=$(dirname "$at")
done

if [ -e "$dir" ]; then
	[ -f "$dir/signing.key" ] && [ -f "$dir/public.txt" ] ||
		refuse "$dir exists but holds no complete lab key; move it away and run again"
	echo "lab-key: $dir already holds the lab key"
	cat "$dir/public.txt"
	exit 0
fi

image="$(pin ENFORCER_IMAGE):$(pin ENFORCER_COMMIT)"
docker image inspect "$image" >/dev/null 2>&1 || refuse "no image $image; run make enforcer-image first"
# shellcheck source=SCRIPTDIR/container-user.sh
source "$root/scripts/container-user.sh"
user=$(container_user "$image") || exit 1

parent=$(dirname "$dir")
if [ ! -d "$parent" ]; then
	mkdir -p "$parent"
	chmod 700 "$parent"
fi
# keygen writes into a directory of its own beside the key's place, so the
# container sees nothing else of the parent, and the key appears by one rename.
stage=$(mktemp -d "$parent/.lab-key.XXXXXX")
trap 'rm -rf "$stage"' EXIT
lines=$(docker run --rm --pull never --network none --read-only --cap-drop ALL \
	--security-opt no-new-privileges --user "$user" \
	-v "$stage:/keys" --entrypoint /enforcer/control "$image" \
	policy keygen --out /keys/key)
printf '%s\n' "$lines" | grep -Eq '^key_id: ' || refuse "keygen printed no key_id"
printf '%s\n' "$lines" | grep -Eq '^public_key: ' || refuse "keygen printed no public_key"
mv "$stage/key" "$dir"
printf '%s\n' "$lines" >"$dir/public.txt"
echo "lab-key: made $dir"
cat "$dir/public.txt"
