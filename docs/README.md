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

## Contracts

- [Trajectory and scenario files](lab-files.md): The two files a run is written in, the checks each expectation becomes, and how a workspace outside the clone holds your own.

## Project

- [Dependencies](dependencies.md): Every direct dependency of the lab, the pinned systems under test and the images, with the reason each one is here.
- [Status](status.md): What exists in the lab today, component by component, with what each green result does and does not show.
