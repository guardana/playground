SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
SOURCES = scripts/repo-files.sh | grep '\.go$$'

.PHONY: bootstrap fmt fmt-check vet lint test test-race security docs-check \
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

docs-check:
	$(GO) test -count=1 ./internal/docscheck/

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
