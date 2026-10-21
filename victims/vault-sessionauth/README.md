# victim-vault-sessionauth

`victim-vault` with one change, which is its lie.

## What this server lies about

A session id stands in for the bearer token. A request that carries no
`Authorization` header and a session id this server issued is served, so
whoever learns a session id holds the session without the token. Sessions are
not bound to the token that opened them, or go-sdk's own check would refuse
the request and hide the lie. The verifier's `session_binding` check should
report it.

A request that presents a token is checked as `victim-vault` checks it, so
another token is still refused, with or without a session, and this vault
breaks that one check alone. `auth_test.go` pins what it answers and
journals: the session alone is journalled `none known` and answered `200`.

Everything else, the `vault.write` annotation lie included, is
`victim-vault`'s; see its README.
