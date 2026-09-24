# Dependencies

Every direct dependency is listed here with the reason it exists. A lab that
drags in a large tree is a lab nobody can audit, and this one is pointed at
security tooling.

## Runtime

### github.com/modelcontextprotocol/go-sdk

The official Go implementation of the Model Context Protocol. The victim tool
servers, the scripted agent and the stub gateway all speak MCP, and the lab's
whole claim is that it exercises the protocol a real deployment uses. Writing
the JSON-RPC framing, the streamable HTTP transport and the session lifecycle by
hand would make every finding a question about our transport rather than about
the system under test. Apache-2.0, with parts still under the MIT licence the
project is moving from; maintained by the protocol's own authors.

The annotations this SDK carries on a tool — `readOnlyHint` and the rest — are
the field the victims lie in. It models them as hints, which is what they are.

### sigs.k8s.io/yaml

Reads the trajectory and scenario files. The standard library has no YAML
parser, and this one converts a document to JSON and then decodes it with
`encoding/json`, so one set of struct tags governs both formats and
`UnmarshalStrict` refuses a key the type has no field for. That refusal is the
reason for the choice: a misspelled expectation that loads is an assertion
nobody makes. MIT, with the vendored parser under BSD-3-Clause.

`gopkg.in/yaml.v3` was the alternative and is archived upstream;
`go.yaml.in/yaml/v4` is a release candidate.

## Build and check tooling

Pinned in `go.mod` under `tool`, so `go tool <name>` runs the same version
everywhere.

### golang.org/x/tools/cmd/goimports

Formats and orders imports so formatting never appears in a diff. BSD-3-Clause,
maintained by the Go team.

### golang.org/x/vuln/cmd/govulncheck

Reports known vulnerabilities in the dependency tree, filtered to reachable
code. BSD-3-Clause, maintained by the Go team.

## Outside the module

Pinned in `scripts/tool-versions.env` and installed by `make bootstrap`:
`golangci-lint`, `actionlint` and `zizmor` for workflow linting, `gitleaks` for
secret scanning, `osv-scanner` for advisories, `syft` for bills of materials.

The container images the lab runs are pinned separately in `versions.env`,
because they are the subject of the experiment rather than part of the build.
Each image the lab pulls is pinned by tag and by the digest of its multi-arch
index; the digest is what Docker resolves.

## The systems under test

### The enforcement plane

Built by `make enforcer-image` from `git archive` of `ENFORCER_COMMIT`, taken
from the clone `ENFORCER_SOURCE` names; it publishes no image to pull. The
build refuses a clone with replacement refs and an archive that does not hash
back to the commit's tree, which an export attribute or a filter would cause.
It uses `GO_BUILD_IMAGE` and `SERVICE_BASE_IMAGE`, stamps the commit into the
binaries and the image's `org.opencontainers.image.revision` label, and every
report prints the image ID and that label beside the pin. The label is a build
argument, so it names the pin the image was built for, not what went into it.
Apache-2.0.

### The verifier

`guardana-cli` from PyPI at `VERIFIER_VERSION`, built by `make verifier-image`
on `PYTHON_IMAGE`. `compose/verifier/requirements.lock` pins it and the twelve
packages it imports to exact versions and file hashes, and pip installs wheels
only with `--require-hashes`, so a file that changed on the index under a
version fails the build instead of entering the lab. pip is removed afterwards
and no file in the image keeps a setuid or setgid bit. Apache-2.0; its
dependencies are under MIT, BSD-2-Clause, ISC or the PSF licence.

## Adding one

Open the change with the paragraph, not after review asks for it: what it
solves, why the standard library cannot, its licence, and whether it is
maintained.
