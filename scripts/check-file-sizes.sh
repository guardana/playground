#!/usr/bin/env bash
# Warns at 350 non-blank lines and fails at 500.
#
# A long file is usually several ideas sharing a name. Generated code, tests and
# fixtures are exempt because their length says nothing about the design.
set -euo pipefail

cd "$(dirname "$0")/.."

# The scans below read the list through a pipe that hides its exit status, so a
# list that could not be made fails here rather than scanning nothing.
scripts/repo-files.sh >/dev/null

status=0
while IFS= read -r f; do
	lines=$(grep -cve '^[[:space:]]*$' -- "$f")
	if [ "$lines" -gt 500 ]; then
		echo "FAIL $f: $lines lines (limit 500)" >&2
		status=1
	elif [ "$lines" -gt 350 ]; then
		echo "warn $f: $lines lines (target 250)"
	fi
done < <(scripts/repo-files.sh | grep -a '\.go$' | grep -a -vE '^(api/gen/|testdata/)|_test\.go$|\.pb\.go$')

[ "$status" -eq 0 ] && echo "sizes: clean"
exit $status
