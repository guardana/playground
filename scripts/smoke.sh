#!/usr/bin/env bash
# Runs representative green scenarios through the ordinary runner. A failed
# scenario does not stop the others, so one invocation shows every broken path.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
unset LAB_WORKSPACE

scratch=$(mktemp -d "${TMPDIR:-/tmp}/lab-smoke.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
go build -o "$scratch/runner" ./runner

failed=0
for scenario in \
	tool-02-permitted-read-is-recorded-by-the-enforcer \
	flow-01-a-private-read-is-not-mailed-to-an-untrusted-sink \
	approval-01-held-send-runs-once-after-approval \
	trace-01-an-approved-payout-change-meets-the-contract \
	verify-02-stable-manifest-against-its-own-pin; do
	if ! "$scratch/runner" -scenario "$scenario" -reports "${REPORTS:-reports}"; then
		failed=1
	fi
done

if [ "$failed" -ne 0 ]; then
	echo "smoke: red" >&2
	exit 1
fi
echo "smoke: green"
