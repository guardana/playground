#!/usr/bin/env bash
# Makes the lab's policy signing key on this machine, once, with the pinned
# enforcer's own `policy keygen`. The key lives outside the clone and outside
# every image: the runner mounts it read-only into the one-shot container that
# signs a scenario's bundle, never into the gateway. keygen's two printed lines,
# the key id and the public key, are public and kept beside the key for the
# runner to hand the gateway. An existing key is reported, never replaced.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

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
"$root" | "$root"/*) refuse "LAB_KEYS_DIR is inside the clone: $dir" ;;
esac

if [ -e "$dir" ]; then
	[ -f "$dir/signing.key" ] && [ -f "$dir/public.txt" ] ||
		refuse "$dir exists but holds no complete lab key; move it away and run again"
	echo "lab-key: $dir already holds the lab key"
	cat "$dir/public.txt"
	exit 0
fi

image="$(pin ENFORCER_IMAGE):$(pin ENFORCER_COMMIT)"
docker image inspect "$image" >/dev/null 2>&1 || refuse "no image $image; run make enforcer-image first"

parent=$(dirname "$dir")
mkdir -p "$parent"
chmod 700 "$parent"
lines=$(docker run --rm --pull never --network none --read-only --cap-drop ALL \
	--security-opt no-new-privileges --user "$(id -u):$(id -g)" \
	-v "$parent:/keys" --entrypoint /enforcer/control "$image" \
	policy keygen --out "/keys/$(basename "$dir")")
printf '%s\n' "$lines" | grep -Eq '^key_id: ' || refuse "keygen printed no key_id"
printf '%s\n' "$lines" | grep -Eq '^public_key: ' || refuse "keygen printed no public_key"
printf '%s\n' "$lines" >"$dir/public.txt"
echo "lab-key: made $dir"
cat "$dir/public.txt"
