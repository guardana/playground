#!/usr/bin/env bash
# Runs the benchmarks and records them with the machine they ran on.
set -euo pipefail

cd "$(dirname "$0")/.."

revision=$(git rev-parse --short HEAD 2>/dev/null || echo "no-revision")
output="bench/results/$(date -u +%Y-%m-%d)-${revision}.txt"

{
	echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "revision: $revision"
	echo "go: $(go version)"
	echo "cpu: $(uname -m) $(getconf _NPROCESSORS_ONLN) cores"
	echo "gomaxprocs: ${GOMAXPROCS:-$(go env GOMAXPROCS)}"
	echo
} >"$output"

go test ./bench/ -bench . -count=10 -benchmem | tee -a "$output"
echo "written to $output"
