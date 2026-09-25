# Roadmap

Ordered by dependency, not by date. `docs/status.md` says whether a phase is
actually done.

## P0 — Rules and gate (implemented)

Repository rules, hygiene guards, quality gate, pinned versions of the systems
under test. `make quality` is green from a clean checkout.

## P1 — Skeleton, victims, scripted agent (implemented)

The compose core profile. Six tool servers with realistic data and deliberately
wrong annotations: customer records, raw SQL, a sandboxed filesystem, a shell,
mail, and a fetcher pointed at an attacker-controlled page. A scripted agent
that replays a trajectory as real tool calls, forwarding one step's output into
the next so a toxic flow is a real data flow. A runner that asserts on decisions,
the evidence trail and the victims' journals.

## P2 — The real enforcer and the first catalogue (current)

Scenarios against the actual enforcement plane: out-of-scope calls, cross-tenant
reads, delegation that exceeds its parent, injected instructions, approvals that
expire or get reused for a mutated action, arguments changed between
authorization and execution. An approver service that can auto-approve, reject,
expire or approve a mutated request. Chaos: latency, timeouts, cut links, a full
spool, a skewed clock. Reports as JUnit and Markdown.

## P3 — The verifier loop (planned)

Probe the protected endpoint with Guardana, hand it the evidence the enforcer
wrote, and check that its contracts agree with what the enforcer decided. A
deliberately widened policy has to fail the build.

## P4 — Detectors and benchmarks (planned)

Scenarios whose expected outcome is a finding rather than a verdict: loops,
fan-out, a tool that failed followed by a claim it succeeded, schema drift
between versions. A benchmark rig with committed results and the hardware that
produced them.
