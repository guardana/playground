#!/usr/bin/env bash
# Installs the tools the quality gate needs and verifies their versions against
# scripts/tool-versions.env.
#
# Homebrew where it exists, release binaries into ./bin otherwise, so a CI
# runner installs the same versions a laptop does and `make quality` means the
# same thing in both places. A release binary is installed only when its
# download matches the sha256 pinned for this platform. goimports and
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

linux_arch() {
	[ "$(uname -s)" = Linux ] ||
		refuse "no pinned release asset for $(uname -s); install Homebrew or the versions in scripts/tool-versions.env"
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) refuse "no pinned release asset for Linux $(uname -m)" ;;
	esac
}

asset_url() { # tool arch
	local releases=https://github.com
	case "$1/$2" in
	golangci-lint/*) echo "$releases/golangci/golangci-lint/releases/download/v$GOLANGCI_LINT_VERSION/golangci-lint-$GOLANGCI_LINT_VERSION-linux-$2.tar.gz" ;;
	actionlint/*) echo "$releases/rhysd/actionlint/releases/download/v$ACTIONLINT_VERSION/actionlint_${ACTIONLINT_VERSION}_linux_$2.tar.gz" ;;
	gitleaks/amd64) echo "$releases/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz" ;;
	gitleaks/arm64) echo "$releases/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION/gitleaks_${GITLEAKS_VERSION}_linux_arm64.tar.gz" ;;
	osv-scanner/*) echo "$releases/google/osv-scanner/releases/download/v$OSV_SCANNER_VERSION/osv-scanner_linux_$2" ;;
	zizmor/amd64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-x86_64-unknown-linux-gnu.tar.gz" ;;
	zizmor/arm64) echo "$releases/zizmorcore/zizmor/releases/download/v$ZIZMOR_VERSION/zizmor-aarch64-unknown-linux-gnu.tar.gz" ;;
	*) refuse "no release asset known for $1 on linux $2" ;;
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

fetch() { # tool arch bindir
	local tool=$1 url expected scratch binary
	url=$(asset_url "$tool" "$2")
	expected=$(pinned "$tool" "SHA256_LINUX_$(printf '%s' "$2" | tr '[:lower:]' '[:upper:]')")
	[ -n "$expected" ] || refuse "scripts/tool-versions.env pins no sha256 for $tool on linux $2"
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
	mkdir -p "$3"
	install -m 0755 "$binary" "$3/$tool"
	rm -rf "$scratch"
	trap - EXIT
}

# installed prints the version token the tool on PATH reports, without a
# leading v, so 1.7.12 is compared whole and never as a prefix of 1.7.123.
installed() { # tool
	local out
	out=$("$1" "$(version_flag "$1")" 2>&1) || return 0
	case "$1" in
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

	command -v go >/dev/null || refuse "install Go first: https://go.dev/dl/"
	go mod download

	local bin="$PWD/bin" tool arch status=0 want have
	export PATH="$bin:$PATH"
	if command -v brew >/dev/null; then
		for tool in $TOOLS; do
			command -v "$tool" >/dev/null || brew install "$tool"
		done
	else
		arch=$(linux_arch)
		for tool in $TOOLS; do
			[ "$(installed "$tool")" = "$(pinned "$tool" VERSION)" ] || fetch "$tool" "$arch" "$bin"
		done
		if [ -n "${GITHUB_PATH:-}" ]; then echo "$bin" >>"$GITHUB_PATH"; fi
	fi

	for tool in $TOOLS; do
		want=$(pinned "$tool" VERSION)
		have=$(installed "$tool")
		if [ -n "$want" ] && [ "$have" = "$want" ]; then
			printf 'ok    %-16s %s\n' "$tool" "$want"
		else
			printf 'FAIL  %-16s want %s, have %s\n' "$tool" "${want:-<unpinned>}" "${have:-<none>}" >&2
			status=1
		fi
	done
	[ "$status" -eq 0 ] && echo "bootstrap ok"
	return "$status"
}

# Sourcing defines the functions without installing anything.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
