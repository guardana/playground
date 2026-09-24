#!/usr/bin/env bash
# Builds the verifier image from the release versions.env pins, installed from
# compose/verifier/requirements.lock with every hash checked. Refuses to build
# when the lock pins another release than versions.env does.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

pin() {
	local value
	value=$(sed -n "s/^$1=//p" "$root/versions.env")
	if [ -z "$value" ]; then
		echo "build-verifier: versions.env sets no $1" >&2
		exit 1
	fi
	printf '%s' "$value"
}

package=$(pin VERIFIER_PACKAGE)
version=$(pin VERIFIER_VERSION)
if ! grep -q "^${package}==${version} " "$root/compose/verifier/requirements.lock"; then
	echo "build-verifier: requirements.lock does not pin ${package}==${version}" >&2
	exit 1
fi

image="$(pin VERIFIER_IMAGE):$version"
docker build \
	--file "$root/compose/Dockerfile.verifier" \
	--build-arg "PYTHON_IMAGE=$(pin PYTHON_IMAGE)" \
	--build-arg "VERIFIER_CLI=$(pin VERIFIER_CLI)" \
	--build-arg "VERIFIER_VERSION=$version" \
	--tag "$image" \
	"$root/compose/verifier"

echo "verifier: $image $(docker image inspect --format '{{.Id}}' "$image")"
