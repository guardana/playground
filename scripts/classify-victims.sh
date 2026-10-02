#!/usr/bin/env bash
# Writes config/gateway/fingerprints.yaml: the fingerprint the pinned enforcer's
# own `doctor` prints for every tool definition the victims list, so the
# classification in config/gateway/classification.yaml pins exactly those
# definitions; and config/gateway/tools.sha256, the digest of each listing
# snapshot those fingerprints were taken with, which internal/labcheck compares. The lab never computes a fingerprint itself; that would be a
# second implementation of the enforcer's canonical form. Run it after a
# victim's tools change; victims/*/listing_test.go fails until then.
#
# It brings the victims up in a compose project of its own, runs doctor in
# OBSERVE against them with a bundle signed by the lab key, and takes the
# project down again. The enforcer reads block YAML only: no flow collections.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
project=lab-classify

refuse() {
	echo "classify-victims: $*" >&2
	exit 1
}

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	[ -n "$value" ] || refuse "versions.env sets no $1"
	printf '%s' "$value"
}

keys="${LAB_KEYS_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/guardana-playground/lab-key}"
[ -f "$keys/signing.key" ] && [ -f "$keys/public.txt" ] || refuse "no lab key at $keys; run make lab-key first"
key_id=$(sed -n 's/^key_id: //p' "$keys/public.txt")
public_key=$(sed -n 's/^public_key: //p' "$keys/public.txt")
image="$(pin ENFORCER_IMAGE):$(pin ENFORCER_COMMIT)"
docker image inspect "$image" >/dev/null 2>&1 || refuse "no image $image; run make enforcer-image first"

work=$(mktemp -d "${TMPDIR:-/tmp}/classify.XXXXXX")
compose=(docker compose -p "$project" --env-file "$root/versions.env" -f "$root/compose/compose.yaml" --profile core)
cleanup() {
	LAB_RUN_HOST_DIR="$work/reports/classify" LAB_RUN_ID=classify "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT
mkdir -p "$work/reports/classify/journals" "$work/gateway/spool"
chmod 755 "$work/reports" "$work/reports/classify"
chmod 1777 "$work/reports/classify/journals"
chmod 700 "$work/gateway/spool"

victims=(victim-crm victim-db victim-fs victim-shell victim-mail victim-web victim-pay)
for victim in "${victims[@]}"; do
	(cd "$root" && go test "./victims/${victim#victim-}/" -run TestTheListingIsTheOneClassified -count=1 -args -update >/dev/null) ||
		refuse "could not snapshot $victim's listing"
done
LAB_RUN_HOST_DIR="$work/reports/classify" LAB_RUN_ID=classify "${compose[@]}" up -d --build --wait "${victims[@]}" >/dev/null

docker run --rm --pull never --network none --read-only --cap-drop ALL \
	--security-opt no-new-privileges --user "$(id -u):$(id -g)" \
	-v "$keys:/key:ro" -v "$root/config/policies:/policies:ro" -v "$work/gateway:/out" \
	--entrypoint /enforcer/control "$image" \
	policy sign --key /key/signing.key --out /out/policy.bundle /policies/classify.json >/dev/null

{
	printf 'mode: OBSERVE\nproject_id: lab\ntenant_id: lab\n'
	printf 'listener:\n  principal:\n    id: classify\n  agent:\n    id: classify\n'
	printf 'policy:\n  bundle_id: lab-classify\n  bundle_file: policy.bundle\n'
	printf '  key_id: %s\n  public_key: %s\n' "$key_id" "$public_key"
	printf 'evidence:\n  dir: spool\n'
	printf 'export:\n  endpoint: http://127.0.0.1:4318/v1/logs\n  allow_plaintext: true\n'
	printf 'upstreams:\n'
	for victim in "${victims[@]}"; do
		printf '  - name: %s\n    endpoint: http://%s:8080/mcp\n' "$victim" "$victim"
	done
} >"$work/gateway/gateway.yaml"

out=$(docker run --rm --pull never --network "${project}_tool-net" --read-only --cap-drop ALL \
	--security-opt no-new-privileges --user "$(id -u):$(id -g)" \
	-v "$work/gateway:/gateway" "$image" doctor --config /gateway/gateway.yaml) ||
	refuse "doctor did not pass: $(printf '%s\n' "$out" | tail -3)"

target="$root/config/gateway/fingerprints.yaml"
{
	echo "# Written by scripts/classify-victims.sh from the pinned enforcer's doctor; do not edit."
	printf '%s\n' "$out" |
		sed -n 's#^ *\([a-z-]*\)/\([a-z._]*\) is unclassified; its definition fingerprints as \(sha256:[0-9a-f]*\)$#- { upstream: \1, tool: \2, fingerprint: "\3" }#p' |
		LC_ALL=C sort
} >"$target.tmp"
count=$(grep -c '^- ' "$target.tmp" || true)
[ "$count" -gt 0 ] || refuse "doctor printed no fingerprint"
digests="$root/config/gateway/tools.sha256"
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$root/config/gateway" && sha256sum tools/*.json) >"$digests.tmp"
else
	(cd "$root/config/gateway" && shasum -a 256 tools/*.json) >"$digests.tmp"
fi
mv "$target.tmp" "$target"
mv "$digests.tmp" "$digests"
echo "classify-victims: $count fingerprints written to config/gateway/fingerprints.yaml"
echo "classify-victims: the snapshots they were taken from recorded in config/gateway/tools.sha256"
