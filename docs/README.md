---
title: Documentation
summary: Every page under docs, grouped by type, rendered from each page's own frontmatter.
type: project
audience: [engineering, product]
covers: [docs/**]
generated: scripts/gen-docs-index.go
---

# Documentation

Rendered from every page's own frontmatter by `scripts/gen-docs-index.go`. Rebuild it
with `make docs-gen`; an edit made here does not survive the next run.

## Runbooks

- [Bring your own verifier contract](runbooks/bring-your-own-contract.md): Have the pinned verifier grade the agent's own trace of a run against a security contract of yours, and see the contract fail when it should.
- [Bring your own gateway configuration](runbooks/bring-your-own-gateway-configuration.md): Which settings of the enforcer's gateway a scenario sets, which the lab owns and refuses, and how a run assembles the rest.
- [Bring your own policy](runbooks/bring-your-own-policy.md): Run a policy of your own through the pinned enforcer against the lab's victims, and see what it allows, denies and holds.
- [Quickstart](runbooks/quickstart.md): From a checkout to one scenario graded green against the pinned enforcer, what a red run means, and how to clean up.

## How it works

- [How a scenario runs](how-it-works/scenario-run.md): Which service sits on which network, how one call is decided and held, which file each check reads, and what a run leaves on disk.
- [How a verifier scenario runs](how-it-works/verifier-run.md): How the runner runs the pinned verifier against the victims and over the agent's own trace, and which file each verifier check reads.

## Reference

- [Failure modes](reference/failure-modes.md): How an agent deployment goes wrong, which kind of tooling must catch each failure, and the scenarios that simulate it today.
- [Glossary](reference/glossary.md): The lab's own terms, one name per concept, and the name each page uses for the two systems under test.
- [Agent use cases](reference/use-cases.md): The agent deployments the lab simulates or plans to, what each acts through, the failure modes it meets, and what the lab covers of it today.

## Contracts

- [Trajectory and scenario files](lab-files.md): The two files a run is written in, the checks each expectation becomes, and how a workspace outside the clone holds your own.

## Project

- [Dependencies](dependencies.md): Every direct dependency of the lab, the pinned systems under test and the images, with the reason each one is here.
- [Status](status.md): What exists in the lab today, component by component, with what each green result does and does not show.
