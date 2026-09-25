#!/usr/bin/env bash
# Runs what CI runs after the quality gate, the same way on a person's machine:
# builds both systems under test from their pins, makes a lab key of its own
# that is deleted afterwards, runs the whole catalogue judged against
# scenarios/red-by-design.txt, then copies every example out of the clone and
# runs it as a workspace. Every part runs even when an earlier one is red, and
# the script exits non-zero when any part is. Needs Docker and ENFORCER_SOURCE,
# a clone holding ENFORCER_COMMIT (scripts/fetch-enforcer.sh makes one).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
reports=${REPORTS:-reports}

if [ -z "${ENFORCER_SOURCE:-}" ]; then
	echo "ci-scenarios: set ENFORCER_SOURCE to a clone holding ENFORCER_COMMIT (scripts/fetch-enforcer.sh <dir> makes one)" >&2
	exit 2
fi

scratch=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/lab-ci.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
chmod 700 "$scratch"
export LAB_KEYS_DIR="$scratch/lab-key"

cd "$root"
make images
make lab-key
go build -o "$scratch/runner" ./runner

red=0
if "$scratch/runner" -all -reports "$reports" -red-by-design scenarios/red-by-design.txt; then
	echo "ci-scenarios: catalogue green, red exactly where scenarios/red-by-design.txt says"
else
	echo "ci-scenarios: catalogue red" >&2
	red=1
fi

examples=0
for example in examples/*/; do
	[ -d "$example" ] || continue
	example=${example%/}
	examples=$((examples + 1))
	name=$(basename "$example")
	copy="$scratch/examples/$name"
	mkdir -p "$(dirname "$copy")"
	cp -R "$example" "$copy"
	if LAB_WORKSPACE="$copy" "$scratch/runner" -all -reports "$reports"; then
		echo "ci-scenarios: example $name green from a copy outside the clone"
	else
		echo "ci-scenarios: example $name red" >&2
		red=1
	fi
done
if [ "$examples" -eq 0 ]; then
	echo "ci-scenarios: no example under examples/; nothing was run from a workspace" >&2
	red=1
fi

if [ "$red" -ne 0 ]; then
	echo "ci-scenarios: red" >&2
	exit 1
fi
echo "ci-scenarios: green"
