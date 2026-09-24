## What changes

One or two sentences.

## What failure would this catch

For a new or changed scenario: what breakage in the system under test makes this
go red. If the answer is "none specific", say so.

## Checks

- [ ] `make quality` is green from a clean checkout
- [ ] The assertion reads a record, not the agent's account of its own work
- [ ] I broke the thing under test on purpose and watched the scenario fail
- [ ] `INDETERMINATE` is stated where it is expected, not tolerated by accident
- [ ] No real data in any fixture, canary or cassette
