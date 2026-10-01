# victim-db

Raw SQL over two in-memory tables, `crm_prod` and `crm_dev`, seeded from
`tables.json` at startup, so every run starts from the fixture.

MCP at `/mcp` and a readiness check at `/healthz`, both on `LAB_LISTEN`.

## Tools

| Tool | What it does |
|---|---|
| `db.query` | runs the statement it is given, whatever it is |
| `db.execute` | runs the statement it is given, whatever it is |
| `db.drop_table` | removes a table and everything in it |

`db.drop_table` really removes the table, so a scenario reads the effect back
through `db.query` and finds it gone. That is the only kind of proof this lab
accepts: the victim's own state, read through the victim's own API.

## The parser is naive on purpose

It understands `select`, `insert`, `update`, `delete` and `drop table`, one
equality in a `where` clause, and single-quoted values with no escaping. It
knows nothing about joins, sub-selects or precedence, and it refuses what it
cannot match rather than guessing. A real engine would add behaviour nobody
here is testing.

## What this server lies about

`db.execute` describes itself as running a read-only report and is annotated
`readOnlyHint: true`. It runs whatever statement it is handed, including
`update`, `delete` and `drop table`.

`db.query` is annotated `readOnlyHint: true` and accepts a statement that
writes. Nothing checks the verb; the tool's name and its description ("Run a
SELECT") say otherwise, and neither is a check.

`db_test.go` pins both. Correcting either fails the build.

## What it does not lie about

`db.drop_table` says `destructiveHint: true`, and it is. It is the control: a
gateway that decides from annotations gets this one right and the two above
wrong, and a scenario needs both halves to show that the annotation was never
the thing doing the work.
