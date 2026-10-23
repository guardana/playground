#!/usr/bin/env bash
# Fails when a system under test is named outside the code that drives it.
#
# Shared formats, the runner's core, the neutral packages and their tests name
# roles, never a product, so a second tool can be graded through them the same
# way. The names come from versions.env, so an upstream rename moves the rule
# with it. scripts/product-names-allow.txt holds the paths where a name is the
# subject: a driver, the pins, a tool's own configuration and scenarios, and
# the pages that introduce it.
set -euo pipefail

cd "$(dirname "$0")/.."
export LC_ALL=C

# The scans below read the list through a pipe that hides its exit status, so a
# list that could not be made fails here rather than scanning nothing.
scripts/repo-files.sh >/dev/null

readonly ALLOWLIST=scripts/product-names-allow.txt
readonly VARIABLES='VERIFIER_PACKAGE VERIFIER_CLI ENFORCER_GATEWAY_BIN ENFORCER_CONTROL_BIN ENFORCER_NAMESPACE ENFORCER_REPOSITORY'
# Matched in any case, and as a whole capitalised word, on top of the names
# derived from versions.env.
readonly ANY_CASE=guardana
readonly WORD=Control
# The repository's own identity carries the organisation's name and names no
# system under test, so these are removed from a line before it is matched.
readonly STRIPPED='github.com/guardana/playground io.guardana.playground guardana-playground @guardana/maintainers'
# The Go files of the runner's and the scripted agent's main packages construct
# and call the drivers: there, and only there, a driver's import path and its
# exported identifiers are wiring. Which package may import a driver at all is
# held by the import-graph test in internal/repoguard.
readonly WIRING='runner agents/scripted'

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# value VAR prints VAR's value in versions.env, failing when it is not set.
value() {
	local line
	line=$(grep -a -E "^$1=" versions.env | tail -n 1) || {
		echo "product-names: versions.env sets no $1" >&2
		return 1
	}
	printf '%s\n' "${line#*=}"
}

derive_names() {
	local var v
	for var in $VARIABLES; do
		v=$(value "$var")
		if [ "$var" = ENFORCER_REPOSITORY ]; then
			v=${v#*://}
			v=${v#*/}
			v=${v%%/*}
		fi
		if [ -z "$v" ]; then
			echo "product-names: versions.env gives $var no name" >&2
			return 1
		fi
		printf '%s\n' "$v"
	done | sort -u
}

derive_names >"$work/names"
if [ ! -s "$work/names" ]; then
	echo "product-names: no name derived from versions.env; nothing would be checked" >&2
	exit 1
fi

scripts/repo-files.sh >"$work/files"
status=0
while IFS= read -r f; do
	[ -r "$f" ] || { echo "product-names: unreadable: $f" >&2; status=1; }
done <"$work/files"

# A line that holds any name at all, in any case, before tokens are stripped.
candidates() {
	local args=(-e "$ANY_CASE" -e "$WORD") name
	while IFS= read -r name; do args+=(-e "$name"); done <"$work/names"
	tr '\n' '\0' <"$work/files" | xargs -0 grep -naiF "${args[@]}" -- 2>/dev/null || true
}

# Prints each path:line:text whose text, stripped, still names a system under
# test: the any-case name anywhere, the word or a derived name as a whole word.
# A driver is a directory under drivers/; in a wiring file its import path and
# its package's exported identifiers are removed first.
# shellcheck disable=SC2016 # an awk program, expanded by awk
readonly MATCH='
function word(c) { return c ~ /[A-Za-z0-9_]/ }
function replace(text, from, to,   i, out) {
	out = ""
	while ((i = index(text, from)) > 0) { out = out substr(text, 1, i - 1) to; text = substr(text, i + length(from)) }
	return out text
}
function whole(text, name,   i, at) {
	at = 0
	while ((i = index(substr(text, at + 1), name)) > 0) {
		at += i
		if (!word(substr(text, at - 1, 1)) && !word(substr(text, at + length(name), 1))) return 1
	}
	return 0
}
function unwire(text,   k, d, i, at, before) {
	for (k = 1; k <= nd; k++) {
		d = driver[k]
		text = replace(replace(text, "/drivers/" d "\"", "/drivers/\""), "/drivers/" d "/", "/drivers//")
		at = 0
		while ((i = index(substr(text, at + 1), d ".")) > 0) {
			at += i
			before = substr(text, at - 1, 1)
			if (!word(before) && before != "." && substr(text, at + length(d) + 1, 1) ~ /[A-Z]/) {
				text = substr(text, 1, at - 1) substr(text, at + length(d)); at = 0
			}
		}
	}
	return text
}
BEGIN { nn = split(names, name, " "); ns = split(stripped, token, " "); nd = split(drivers, driver, " ") }
{
	p = index($0, ":"); path = substr($0, 1, p - 1); rest = substr($0, p + 1)
	text = substr(rest, index(rest, ":") + 1)
	for (k = 1; k <= ns; k++) text = replace(text, token[k], "")
	dir = path; sub(/\/[^\/]*$/, "", dir)
	if (path ~ /\.go$/ && index(" " wiring " ", " " dir " ")) text = unwire(text)
	hit = index(tolower(text), tolower(anycase)) || whole(text, wordname)
	for (k = 1; k <= nn && !hit; k++) hit = whole(text, name[k])
	if (hit) print
}'

candidates | awk -v names="$(tr '\n' ' ' <"$work/names")" -v stripped="$STRIPPED" -v wiring="$WIRING" \
	-v drivers="$(sed -n 's|^drivers/\([^/]*\)/.*|\1|p' "$work/files" | sort -u | tr '\n' ' ')" \
	-v anycase="$ANY_CASE" -v wordname="$WORD" "$MATCH" >"$work/offending"

# Reads the allowlist, the file list and the offending lines, in that order. An
# entry is a pattern in which * matches any run of characters, slashes
# included, then its reason. Prints every offending line no entry allows, and
# every entry without a reason, matching no file, or allowing no offending line.
# shellcheck disable=SC2016 # an awk program, expanded by awk
readonly ALLOW='
function regex(glob,   out, c, i) {
	out = "^"
	for (i = 1; i <= length(glob); i++) {
		c = substr(glob, i, 1)
		if (c == "*") out = out ".*"
		else if (c == "?") out = out "."
		else if (c ~ /[A-Za-z0-9_\/-]/) out = out c
		else if (c == "^" || c == "\\") out = out "\\" c
		else out = out "[" c "]"
	}
	return out "$"
}
FILENAME == allowlist {
	if ($0 ~ /^[ \t]*$/ || $0 ~ /^#/) next
	if (NF < 2) { print "allowlist entry without a reason: " $1; next }
	n++; glob[n] = $1; re[n] = regex($1); next
}
FILENAME == files { for (k = 1; k <= n; k++) if ($0 ~ re[k]) matched[k] = 1; next }
{
	path = substr($0, 1, index($0, ":") - 1); ok = 0
	for (k = 1; k <= n; k++) if (path ~ re[k]) { used[k] = 1; ok = 1 }
	if (!ok) print
}
END {
	for (k = 1; k <= n; k++) {
		if (!matched[k]) print "allowlist entry matches no file: " glob[k]
		else if (!used[k]) print "allowlist entry covers no line naming a system under test: " glob[k]
	}
}'

awk -v allowlist="$ALLOWLIST" -v files="$work/files" "$ALLOW" "$ALLOWLIST" "$work/files" "$work/offending" >"$work/problems"
while IFS= read -r problem; do
	echo "product-names: $problem" >&2
	status=1
done <"$work/problems"

if [ "$status" -ne 0 ]; then
	echo >&2
	echo "Name the role (enforcer, verifier, gate, prober, grader) instead, move the" >&2
	echo "code under drivers/<tool>/, or add its path to $ALLOWLIST with a reason." >&2
	exit 1
fi

echo "product-names: clean ($(wc -l <"$work/names" | tr -d ' ') names, $(wc -l <"$work/offending" | tr -d ' ') allowed lines)"
