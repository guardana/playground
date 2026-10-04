#!/usr/bin/env bash
# Fails unless the go that runs inside this module is the version go.mod names,
# so the gate is compiled, vetted and linted by one Go on every machine. A
# newer Go would pass go.mod's minimum and still be another compiler.
set -euo pipefail

cd "$(dirname "$0")/.."

want=$(awk '$1 == "go" { print "go" $2; exit }' go.mod)
[ -n "$want" ] || { echo "go-version: go.mod names no go version" >&2; exit 1; }
command -v go >/dev/null || { echo "go-version: install $want first: https://go.dev/dl/" >&2; exit 1; }
if ! have=$(go env GOVERSION 2>&1); then
	printf 'go-version: %s\ngo-version: run with GOTOOLCHAIN=%s, which fetches it, or install it\n' "$have" "$want" >&2
	exit 1
fi
if [ "$have" != "$want" ]; then
	echo "go-version: go.mod names $want and this go is $have; run with GOTOOLCHAIN=$want, which fetches it, or install it" >&2
	exit 1
fi
echo "go-version: $have"
