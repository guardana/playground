# Governance

## Who decides

The project has maintainers. A maintainer can approve and merge changes.

Changes to the runner's assertion library, to how a scenario declares what it
expects, or to CI need two maintainer approvals, one from someone who did not
write the change. Those are the places where a mistake makes everything else
look green.

## Becoming a maintainer

Sustained, reviewed contributions, and judgement the existing maintainers trust
on a scenario they cannot fully verify themselves. Existing maintainers decide
by consensus. Stepping back is normal and carries no stigma.

## What does not change

Everything here is Apache-2.0 and stays that way. A result the lab publishes is
about a published version of its subjects, so it is reproducible by someone who
does not know the maintainers. A development build of the enforcer
(`make dev-scenarios`) is labelled as one in every line and report it produces,
and is never reported as a release's result.

## Conduct

`CODE_OF_CONDUCT.md`, enforced by the maintainers.
