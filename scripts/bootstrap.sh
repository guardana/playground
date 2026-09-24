#!/usr/bin/env bash
# Installs the tools the quality gate needs and verifies their versions against
# scripts/tool-versions.env.
#
# Homebrew where it exists, release binaries into ./bin otherwise, so a CI
# runner installs the same versions a laptop does and `make quality` means the
# same thing in both places. goimports, govulncheck and protoc-gen-go come from
# the go.mod tool directives instead, already pinned by the module.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=tool-versions.env
source scripts/tool-versions.env

command -v go >/dev/null || {
	echo "install Go first: https://go.dev/dl/" >&2
	exit 1
}
go mod download

readonly BIN="$PWD/bin"

fetch() { # tool url
	local tool=$1 url=$2 scratch binary
	scratch=$(mktemp -d)
	curl -fsSL "$url" -o "$scratch/download"
	case "$url" in
	*.tar.gz)
		tar -xzf "$scratch/download" -C "$scratch"
		binary=$(find "$scratch" -type f -name "$tool" | head -1)
		;;
	*) binary="$scratch/download" ;;
	esac
	[ -n "$binary" ] || {
		echo "no $tool binary inside $url" >&2
		exit 1
	}
	mkdir -p "$BIN"
	install -m 0755 "$binary" "$BIN/$tool"
	rm -rf "$scratch"
}

install_missing() {
	command -v buf >/dev/null || fetch buf "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64"
	command -v golangci-lint >/dev/null || fetch golangci-lint "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
	command -v actionlint >/dev/null || fetch actionlint "https://github.com/rhysd/actionlint/releases/download/v${ACTIONLINT_VERSION}/actionlint_${ACTIONLINT_VERSION}_linux_amd64.tar.gz"
	command -v gitleaks >/dev/null || fetch gitleaks "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
	command -v osv-scanner >/dev/null || fetch osv-scanner "https://github.com/google/osv-scanner/releases/download/v${OSV_SCANNER_VERSION}/osv-scanner_linux_amd64"
	command -v syft >/dev/null || fetch syft "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_amd64.tar.gz"
	command -v zizmor >/dev/null || fetch zizmor "https://github.com/zizmorcore/zizmor/releases/download/v${ZIZMOR_VERSION}/zizmor-x86_64-unknown-linux-gnu.tar.gz"
}

if command -v brew >/dev/null; then
	for tool in buf golangci-lint actionlint gitleaks osv-scanner syft zizmor; do
		command -v "$tool" >/dev/null || brew install "$tool"
	done
else
	install_missing
	export PATH="$BIN:$PATH"
	[ -n "${GITHUB_PATH:-}" ] && echo "$BIN" >>"$GITHUB_PATH"
fi

status=0
verify() { # name wanted actual
	case "$3" in
	*"$2"*) printf 'ok    %-16s %s\n' "$1" "$2" ;;
	*)
		printf 'FAIL  %-16s want %s, have %s\n' "$1" "$2" "$3" >&2
		status=1
		;;
	esac
}

verify buf "$BUF_VERSION" "$(buf --version 2>&1)"
verify golangci-lint "$GOLANGCI_LINT_VERSION" "$(golangci-lint --version 2>&1)"
verify actionlint "$ACTIONLINT_VERSION" "$(actionlint -version 2>&1)"
verify gitleaks "$GITLEAKS_VERSION" "$(gitleaks version 2>&1)"
verify osv-scanner "$OSV_SCANNER_VERSION" "$(osv-scanner --version 2>&1)"
verify syft "$SYFT_VERSION" "$(syft version 2>&1)"
verify zizmor "$ZIZMOR_VERSION" "$(zizmor --version 2>&1)"

[ "$status" -eq 0 ] && echo "bootstrap ok"
exit $status
