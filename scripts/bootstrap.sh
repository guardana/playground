#!/usr/bin/env bash
# Installs the tools the quality gate needs into ./bin and verifies their
# versions against scripts/tool-versions.env.
#
# One path on Linux and macOS, amd64 and arm64: the project's release binary,
# downloaded on every run and installed only when it matches the sha256 pinned
# for this platform, so a CI runner and a laptop run the same versions and
# `make quality` means the same thing on both. A binary already in ./bin stays
# only when its bytes are the verified one's, and the sha256 of each binary
# installed is recorded in ./bin/.verified. `--verify`, for the gate, checks
# without the network that each binary is the one recorded, before running it
# for its version; a tool elsewhere on PATH never counts. goimports and
# govulncheck come from the go.mod tool directives instead, already pinned by
# the module.
set -euo pipefail

readonly TOOLS="golangci-lint actionlint gitleaks osv-scanner zizmor"

refuse() {
	echo "bootstrap: $*" >&2
	exit 1
}

# pinned prints the tool-versions.env value for a tool and suffix: zizmor and
# SHA256_LINUX_ARM64 name ZIZMOR_SHA256_LINUX_ARM64. Empty when unset.
pinned() { # tool suffix
	local name
	name="$(printf '%s' "$1" | tr '[:lower:]-' '[:upper:]_')_$2"
	printf '%s' "${!name:-}"
}

# platform prints this machine as the pins name it: the system, then the
# architecture.
platform() {
	local os
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) refuse "no pinned release asset for $(uname -s); put the versions in scripts/tool-versions.env into ./bin by hand" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) echo "$os amd64" ;;
	aarch64 | arm64) echo "$os arm64" ;;
	*) refuse "no pinned release asset for $os $(uname -m)" ;;
	esac
}

upper() { printf '%s' "$1" | tr '[:lower:]' '[:upper:]'; }

asset_url() { # tool os arch
	local releases=https://github.com
	case "$1/$2/$3" in
	golangci-lint/*/*) echo "$releases/golangci/golangci-lint/releases/download/v$GOLANGCI_LINT_VERSION/golangci-lint-$GOLANGCI_LINT_VERSION-$2-$3.tar.gz" ;;
	actionlint/*/*) echo "$releases/rhysd/actionlint/releases/download/v$ACTIONLINT_VERSION/actionlint_${ACTIONLINT_VERSION}_$2_$3.tar.gz" ;;
	gitleaks/*/amd64) echo "$releases/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION/gitleaks_${GITLEAKS_VERSION}_$2_x64.tar.gz" ;;
	gitleaks/*/arm64) echo "$releases/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION/gitleaks_${GITLEAKS_VERSION}_$2_arm64.tar.gz" ;;
	osv-scanner/*/*) echo "$releases/google/osv-scanner/releases/download/v$OSV_SCANNER_VERSION/osv-scanner_$2_$3" ;;
	zizmor/linux/amd64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-x86_64-unknown-linux-gnu.tar.gz" ;;
	zizmor/linux/arm64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-aarch64-unknown-linux-gnu.tar.gz" ;;
	zizmor/darwin/amd64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-x86_64-apple-darwin.tar.gz" ;;
	zizmor/darwin/arm64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-aarch64-apple-darwin.tar.gz" ;;
	*) refuse "no release asset known for $1 on $2 $3" ;;
	esac
}

sha256_of() { # file
	if command -v sha256sum >/dev/null; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

verify_sha256() { # file expected
	local actual
	[ -n "$2" ] || refuse "no sha256 pinned for $1"
	actual=$(sha256_of "$1")
	[ "$actual" = "$2" ] || refuse "sha256 mismatch for $1: pinned $2, downloaded $actual"
}

fetch() { # tool os arch bindir
	local tool=$1 url expected scratch binary
	url=$(asset_url "$tool" "$2" "$3")
	expected=$(pinned "$tool" "SHA256_$(upper "$2")_$(upper "$3")")
	[ -n "$expected" ] || refuse "scripts/tool-versions.env pins no sha256 for $tool on $2 $3"
	scratch=$(mktemp -d "${TMPDIR:-/tmp}/bootstrap.XXXXXX")
	# A refusal exits from inside this function, when its locals are gone, so
	# the trap carries the path itself.
	trap "rm -rf -- $(printf '%q' "$scratch")" EXIT
	curl --proto '=https' --tlsv1.2 -fsSL --retry 3 --connect-timeout 30 --max-time 300 "$url" -o "$scratch/download"
	verify_sha256 "$scratch/download" "$expected"
	case "$url" in
	*.tar.gz)
		mkdir "$scratch/unpacked"
		tar -xzf "$scratch/download" -C "$scratch/unpacked"
		binary=$(find "$scratch/unpacked" -type f -name "$tool" -print -quit)
		;;
	*) binary="$scratch/download" ;;
	esac
	[ -n "$binary" ] || refuse "no $tool binary inside $url"
	mkdir -p "$4"
	if [ -L "$4/$tool" ] || [ ! -x "$4/$tool" ] || ! cmp -s "$binary" "$4/$tool"; then
		rm -f "$4/$tool"
		install -m 0755 "$binary" "$4/$tool"
	fi
	printf '%s %s\n' "$tool" "$(sha256_of "$4/$tool")" >>"$4/.verified.new"
	rm -rf "$scratch"
	trap - EXIT
}

# recorded succeeds when bin/tool is a regular file whose sha256 is the one
# bootstrap recorded for it.
recorded() { # bin tool
	local line
	[ -f "$1/$2" ] && [ ! -L "$1/$2" ] || return 1
	line=$(grep -E "^$2 [0-9a-f]{64}$" "$1/.verified" 2>/dev/null) || return 1
	[ "${line#* }" = "$(sha256_of "$1/$2")" ]
}

# installed prints the version token the binary at path reports, without a
# leading v, so 1.7.12 is compared whole and never as a prefix of 1.7.123.
installed() { # path
	local out tool
	tool=$(basename "$1")
	[ -x "$1" ] || return 0
	out=$("$1" "$(version_flag "$tool")" 2>&1) || return 0
	case "$tool" in
	golangci-lint) out=$(awk '{ for (i = 1; i < NF; i++) if ($i == "version") { print $(i + 1); exit } }' <<<"$out") ;;
	osv-scanner) out=$(awk '$1 == "osv-scanner" && $2 == "version:" { print $3; exit }' <<<"$out") ;;
	zizmor) out=$(awk 'NR == 1 { print $2 }' <<<"$out") ;;
	*) out=$(awk 'NR == 1 { print $1 }' <<<"$out") ;;
	esac
	printf '%s' "${out#v}"
}

version_flag() { # tool
	case "$1" in
	actionlint) echo -version ;;
	gitleaks) echo version ;;
	*) echo --version ;;
	esac
}

main() {
	cd "$(dirname "$0")/.."
	# shellcheck source=tool-versions.env
	source scripts/tool-versions.env

	local bin="${BIN:-$PWD/bin}" tool here os arch status=0 want have
	if [ "${1:-}" != --verify ]; then
		scripts/check-go-version.sh
		go mod download
		here=$(platform)
		read -r os arch <<<"$here"
		rm -f "$bin/.verified.new"
		for tool in $TOOLS; do
			fetch "$tool" "$os" "$arch" "$bin"
		done
		mv "$bin/.verified.new" "$bin/.verified"
		if [ -n "${GITHUB_PATH:-}" ]; then echo "$bin" >>"$GITHUB_PATH"; fi
	fi

	for tool in $TOOLS; do
		want=$(pinned "$tool" VERSION)
		if ! recorded "$bin" "$tool"; then
			printf 'FAIL  %-16s not the binary bootstrap verified\n' "$tool" >&2
			status=1
			continue
		fi
		have=$(installed "$bin/$tool")
		if [ -n "$want" ] && [ "$have" = "$want" ]; then
			printf 'ok    %-16s %s\n' "$tool" "$want"
		else
			printf 'FAIL  %-16s want %s, have %s\n' "$tool" "${want:-<unpinned>}" "${have:-<none>}" >&2
			status=1
		fi
	done
	[ "$status" -eq 0 ] || refuse "./bin does not hold the pinned tools; run make bootstrap"
	echo "bootstrap ok"
}

# Sourcing defines the functions without installing anything.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
