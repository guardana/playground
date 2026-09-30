SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
SOURCES = scripts/repo-files.sh | grep '\.go$$'

.PHONY: bootstrap fmt fmt-check vet lint test test-race security docs-check \
	docs-frontmatter docs-gen docs-impact \
	check-sizes check-attribution check-hygiene check-actions-pinned \
	enforcer-image verifier-image images lab-key classify-victims up down scenario scenarios smoke ci-scenarios \
	quality-quick quality

# The compose profiles a target brings up, space-separated, and where a run
# writes its records.
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
# docs/docs.json.
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

# The lab's policy signing key, made once per machine outside the clone by the
# pinned enforcer's own keygen. Scenarios the enforcer decides sign with it.
lab-key:
	scripts/lab-key.sh

# The fingerprints the enforcer's doctor prints for every victim tool, which the
# classification in config/gateway pins. Run after a victim's tools change.
classify-victims:
	scripts/classify-victims.sh

# The lab itself. `up` and `down` are for working on a scenario by hand; a
# scenario run brings up what it needs and takes it down again. `core` is the
# victims alone. The enforcer's profiles need a run the runner prepares (the
# signed bundle, the assembled configuration): `go run ./runner -scenario <id>
# -keep` leaves one up. By hand, the services write under reports/manual.
up:
	mkdir -p reports/manual/journals reports/manual/agent
	chmod 777 reports/manual reports/manual/journals reports/manual/agent
	$(COMPOSE) $(foreach profile,$(PROFILE),--profile $(profile)) up -d --build

down:
	$(COMPOSE) $(foreach profile,$(PROFILE),--profile $(profile)) down --volumes --remove-orphans

# One scenario, named by its identifier, which is its file name. With
# LAB_WORKSPACE=<dir> set, scenarios and the files they name are read from that
# directory outside the clone, laid out like the lab, instead of from the clone.
scenario:
	@test -n "$(ID)" || { echo "usage: make scenario ID=<scenario id>" >&2; exit 2; }
	$(GO) run ./runner -scenario $(ID) -reports $(REPORTS)

scenarios:
	$(GO) run ./runner -all -reports $(REPORTS)

# A smaller representative loop over both pinned systems. It still grades each
# path from the same records as a full catalogue run.
smoke:
	REPORTS=$(REPORTS) scripts/smoke.sh

# What CI runs after the gate: images, a throwaway lab key, the catalogue judged
# against scenarios/red-by-design.txt, and every example from a copy outside
# the clone. Needs ENFORCER_SOURCE; scripts/fetch-enforcer.sh <dir> makes one.
ci-scenarios:
	REPORTS=$(REPORTS) scripts/ci-scenarios.sh

quality-quick: fmt-check vet test check-attribution check-hygiene

quality: fmt-check vet lint test test-race security docs-check docs-frontmatter check-sizes \
	check-attribution check-hygiene check-actions-pinned
	@echo "quality: green"
