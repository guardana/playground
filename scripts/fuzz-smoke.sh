#!/usr/bin/env bash
# Runs every fuzz target briefly, so a corpus that stopped compiling is caught
# in the gate rather than in the next long fuzzing run.
set -euo pipefail

cd "$(dirname "$0")/.."
duration="${1:-10s}"

set +e # a file without a fuzz target makes grep exit non-zero, which is fine
found=0
while IFS=' ' read -r package target; do
	[ -n "$package" ] || continue
	found=$((found + 1))
	echo "fuzz $package $target"
	go test "$package" -run '^$' -fuzz "^${target}\$" -fuzztime "$duration"
done < <(
	scripts/repo-files.sh | grep '_test\.go$' | while IFS= read -r file; do
		# A file with no fuzz target is the common case, not a failure.
		grep -oE '^func (Fuzz[A-Za-z0-9_]*)\(' "$file" | sed -E 's/^func //; s/\($//' |
			while IFS= read -r target; do echo "./$(dirname "$file") $target"; done || true
	done
)

echo "fuzz-smoke: $found targets"
