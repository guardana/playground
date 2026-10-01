---
title: Dependencies
summary: Every direct dependency of the lab, the pinned systems under test and the images, with the reason each one is here.
type: project
audience: [engineering]
covers: [go.mod, go.sum, versions.env, compose/Dockerfile.*, compose/verifier/requirements.lock, scripts/bootstrap.sh]
---

# Dependencies

Every direct dependency is listed here with the reason it exists. A lab that
drags in a large tree is a lab nobody can audit, and this one is pointed at
security tooling.

## Runtime

### github.com/modelcontextprotocol/go-sdk

The official Go implementation of the Model Context Protocol. The victim tool
servers and the scripted agent speak MCP, as the enforcer does, and the lab's
whole claim is that it exercises the protocol a real deployment uses. Writing
the JSON-RPC framing, the streamable HTTP transport and the session lifecycle by
hand would make every finding a question about our transport rather than about
the system under test. Apache-2.0, with parts still under the MIT licence the
project is moving from; maintained by the protocol's own authors.

The annotations this SDK carries on a tool — `readOnlyHint` and the rest — are
the field the victims lie in. It models them as hints, which is what they are.

The scripted agent tells an error the gateway sent from one the SDK made itself
by the SDK's own codes for a call it could not deliver (-32003, -32004, -32005
at the version `go.mod` pins); an upgrade that adds another such code has to be
added there, or `on_error: continue` would go on past a call that never
reached the gateway.

### sigs.k8s.io/yaml

Reads the trajectory and scenario files. The standard library has no YAML
parser, and this one converts a document to JSON and then decodes it with
`encoding/json`, so one set of struct tags governs both formats and
`UnmarshalStrict` refuses a key the type has no field for. That refusal is the
reason for the choice: a misspelled expectation that loads is an assertion
nobody makes. MIT, with the part taken from `encoding/json` under
BSD-3-Clause. It parses with `go.yaml.in/yaml/v2`, which `go.mod` lists as
indirect: Apache-2.0, with the parts ported from libyaml under MIT.

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

Pinned in `scripts/tool-versions.env`: `golangci-lint` for Go linting,
`actionlint` and `zizmor` for workflow linting, `gitleaks` for secret scanning
and `osv-scanner` for advisories, which `make quality` runs, and `buf` and
`syft`, which no target runs yet. `make bootstrap` installs each one missing,
from Homebrew where it exists and otherwise from the pinned linux-amd64
release, and fails when an installed version is not the pin.

The container images the lab runs are pinned separately in `versions.env`,
because they are the subject of the experiment rather than part of the build.
Each image the lab pulls is pinned by tag and by the digest of its multi-arch
index; the digest is what Docker resolves.

### OpenTelemetry Collector

`OTEL_COLLECTOR_IMAGE`. The one place the enforcer's OTLP/HTTP log export
lands, so `internal/evidence.DecodeOTLP` reads back what the plane actually
sent rather than a file the lab wrote itself. It receives over TLS with a
certificate the runner signs for each run with a CA of its own, since the
enforcer sends plaintext only to a loopback address. The
core distribution carries the file exporter (`docker run --rm
otel/opentelemetry-collector:<tag> components` lists it), so writing an
OTLP/HTTP receiver by hand to avoid one dependency would make every finding a
question about the lab's receiver instead of the enforcer's export.
Apache-2.0, maintained by the OpenTelemetry project.

### Mailpit

`MAILPIT_IMAGE`. The SMTP server victim-mail delivers to, so a send goes over
real SMTP to something that accepts it. No scenario reads it: what a send did
is graded from victim-mail's journal. MIT, maintained by its author.

### nginx

`ATTACKER_WEB_IMAGE`, the `attacker-web` service: serves the inert pages in
`attacks/` to victim-web on `tool-net`. A static file server needs nothing the
lab should write itself. nginx is BSD-2-Clause; the image is the nginx
project's Alpine build.

### Base images

`GO_BUILD_IMAGE` compiles every lab service and the enforcer;
`SERVICE_BASE_IMAGE`, distroless `static` as a non-root user, is what they run
on, with no shell and no package manager. Go is BSD-3-Clause; distroless is
Apache-2.0, maintained by Google.

### Toxiproxy

`TOXIPROXY_IMAGE`. The TCP proxy a chaos scenario puts between the enforcer and
one victim, to delay or hold that victim's answers with a toxic the runner adds
and removes through the proxy's own command line. A proxy written for the lab
would make every chaos result a question about the lab's proxy. MIT, maintained
by Shopify.

## The systems under test

### The enforcement plane

Built by `make enforcer-image` from `git archive` of `ENFORCER_COMMIT`, the
commit of the release `ENFORCER_RELEASE`, taken from the clone
`ENFORCER_SOURCE` names, rather than pulled as the release's image, so the
build can check the commit's tree. The build refuses a clone with replacement
refs, a commit whose tree is not `ENFORCER_TREE`, and an archive that does not
hash back to that tree, which an export attribute or a filter would cause.
It uses `GO_BUILD_IMAGE` and `SERVICE_BASE_IMAGE`, stamps the commit into the
binaries and the image's `org.opencontainers.image.revision` label, and every
report prints the image ID and that label beside the pin. The label is a build
argument, so it names the pin the image was built for, not what went into it;
the tree the build verified goes into `io.guardana.playground.enforcer.tree`,
and the runner fails `plane/image` when the running image lacks it.
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
