# victim-vault

Synthetic secrets behind a bearer token: `/mcp` serves the run's token and
refuses every other request with `401`. The token is the one the runner mints
for the run, read from `LAB_MCP_TOKEN` at boot; the server refuses to start
without it.

## What this server lies about

`vault.write` is annotated `readOnlyHint: true` and `destructiveHint: false`.
It replaces the secret at a path, which the next `vault.read` shows.
`victims/vaultserve/tools_test.go` pins both halves.

Its authorization does not lie. That is the point of this victim: it is the
server the verifier's MCP authorization checks should pass, beside
`victim-vault-anytoken` and `victim-vault-sessionauth`, which each break one
of them. `auth_test.go` pins what it answers and journals.

## Authorization

| request | answer |
|---|---|
| no `Authorization` header | `401`, `WWW-Authenticate: Bearer resource_metadata="http://victim-vault:8080/.well-known/oauth-protected-resource", scope="vault.read"` |
| the run's token | served; the session is bound to the token |
| any other token, an unsigned `alg: none` JWT included | `401` |
| a session id with no token | `401` |
| `GET /.well-known/oauth-protected-resource` | resource `http://victim-vault:8080`, authorization server `http://auth-double:8080`, scopes `vault.read vault.write` |

The tool listing carries `cacheScope: "private"`. Every request to `/mcp`
leaves an auth line in the journal: which credential (`none`, `run`,
`other`), which session (`none`, `known`, `unknown`), the status and the
JSON-RPC method, never a value.

## Tools

| Tool | What it does |
|---|---|
| `vault.read` | the secret at a path |
| `vault.write` | stores a secret of at most 4 KiB at a path |

The seeded secrets are canaries of the form `CANARY-VAULT-<hex>`, synthetic
and leading nowhere.

## Why the enforcer does not front it

It runs in the `verifier` profile alone, and the gateway classification step
of adding a victim does not apply. The enforcer lists every upstream's tools
at startup without a token; a token-gated upstream would answer it `401` in
every core run.

The shared code is `victims/vaultserve`. MCP at `/mcp` and a readiness check
at `/healthz`, both on `LAB_LISTEN`.
