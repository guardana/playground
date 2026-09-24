SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
SOURCES = scripts/repo-files.sh | grep '\.go$$'

.PHONY: bootstrap fmt fmt-check vet lint test test-race security docs-check \
	docs-frontmatter docs-gen docs-impact \
	check-sizes check-attribution check-hygiene check-actions-pinned \
	enforcer-image verifier-image images up down scenario scenarios quality-quick quality

# The compose profile a target brings up, and where a run writes its records.
PROFILE ?= core
REPORTS ?= reports
COMPOSE = docker compose --env-file versions.env -f compose/compose.yaml

bootstrap:
	scripts/bootstrap.sh

fmt:
	$(GO) tool goimports -w $$($(SOURCES))

fmt-check:
	@unformatted=$$(gofmt -l $$($(SOURCES))); \
	if [ -n "$$unformatted" ]; then echo "unformatted:"; echo "$$unformatted"; exit 1; fi; \
	echo "format: clean"

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

test:
	$(GO) test -count=1 -shuffle=on ./...

test-race:
	$(GO) test -count=1 -race ./...

security:
	$(GO) tool govulncheck ./...
	gitleaks dir --no-banner --redact .
	osv-scanner scan source -r .
	actionlint .github/workflows/*.yml
	zizmor --min-severity medium .github/workflows

check-sizes:
	scripts/check-file-sizes.sh

check-attribution:
	scripts/check-attribution.sh

check-hygiene:
	scripts/check-hygiene.sh

check-actions-pinned:
	scripts/check-actions-pinned.sh

# `go test ./...` never compiles the tagged whole-tree test or the two
# `//go:build ignore` scripts, so a rename they miss would stay green.
docs-check:
	$(GO) test -count=1 ./internal/docscheck/
	$(GO) vet -tags docsfrontmatter ./internal/docscheck/
	$(GO) build -o /dev/null scripts/gen-docs-index.go
	$(GO) build -o /dev/null scripts/docs-impact.go

# Frontmatter, types, covers, diagrams and word budgets of every page against
# docs/docs.json. Outside `quality` until the existing pages carry frontmatter.
docs-frontmatter:
	$(GO) test -count=1 -tags docsfrontmatter -run TestEveryPageCarriesItsFrontmatter ./internal/docscheck/

docs-gen:
	$(GO) run scripts/gen-docs-index.go -o docs/README.md

# The pages a change makes suspect. FOR=<path> or STALE=1 instead of RANGE.
# Built and run rather than `go run`, which reports every failure as exit 1
# and would hide exit 2, NOT MEASURED, behind exit 1, a page that did not parse.
docs-impact:
	@if [ -n "$(FOR)" ]; then set -- --for "$(FOR)"; \
	elif [ -n "$(STALE)" ]; then set -- --stale; \
	elif [ -n "$(RANGE)" ]; then set -- --range "$(RANGE)"; \
	else echo "usage: make docs-impact RANGE=<a..b> | FOR=<path> | STALE=1" >&2; exit 2; fi; \
	bin=$$(mktemp -d); trap 'rm -rf "$$bin"' EXIT; \
	$(GO) build -o "$$bin/docs-impact" scripts/docs-impact.go; \
	"$$bin/docs-impact" "$$@"

# The systems under test, built from their pins in versions.env. The enforcer
# needs ENFORCER_SOURCE, a clone of its repository holding ENFORCER_COMMIT.
enforcer-image:
	scripts/build-enforcer.sh

verifier-image:
	scripts/build-verifier.sh

images: enforcer-image verifier-image

# The lab itself. `up` and `down` are for working on a scenario by hand; a
# scenario run brings up what it needs and takes it down again.
up:
	$(COMPOSE) --profile $(PROFILE) up -d --build

down:
	$(COMPOSE) --profile $(PROFILE) down --volumes --remove-orphans

# One scenario, named by its identifier, which is its file name.
scenario:
	@test -n "$(ID)" || { echo "usage: make scenario ID=<scenario id>" >&2; exit 2; }
	$(GO) run ./runner -scenario $(ID) -reports $(REPORTS)

scenarios:
	$(GO) run ./runner -all -reports $(REPORTS)

quality-quick: fmt-check vet test check-attribution check-hygiene

quality: fmt-check vet lint test test-race security docs-check check-sizes \
	check-attribution check-hygiene check-actions-pinned
	@echo "quality: green"
