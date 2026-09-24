# Security policy

This repository contains attack payloads and services that behave badly on
purpose. That is its job, and it is not what to report.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository: **Security** →
**Report a vulnerability**. It is private to the maintainers.

Do not open a public issue. If you cannot use GitHub's form, open a public issue
asking a maintainer to contact you, with no detail in it.

## What counts here

The lab is supposed to be sealed and honest. Reports that matter break one of
those two:

- A payload, a victim service or an agent reaches something outside the compose
  network, or the host.
- A fixture, a canary or a cassette holds something real: a credential, a
  customer record, a captured prompt.
- A scenario passes while the behaviour it claims to test is broken — an
  assertion that reads the agent's narrative, an `INDETERMINATE` counted as a
  pass, or a check that silently inspects nothing.
- The runner reports green when it did not run, or when a service failed to
  start.

The last two are the ones that quietly destroy the value of everything here, and
they are treated as security bugs rather than test bugs.

A vulnerability in one of the systems under test belongs in that system's own
repository, not this one.

## Scope

This repository. Pre-alpha, so report against the default branch.

## What we ask of you

Run the lab against your own deployment. Do not point it at systems you do not
operate.
