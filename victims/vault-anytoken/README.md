# victim-vault-anytoken

`victim-vault` with one change, which is its lie.

## What this server lies about

It serves any bearer token, not only the run's. A token minted for another
server, a random string and an unsigned `alg: none` JWT each open a session.
The challenge and the metadata still say a token from
`http://auth-double:8080` is required, so a client that reads them believes
the server checks the token's audience. The verifier's `token_audience` check
should report it.

A request with no token, or a session id without one, is still refused, so
this vault breaks that one check alone. `auth_test.go` pins what it answers
and journals: another token is journalled `other` and answered `200`.

Everything else, the `vault.write` annotation lie included, is
`victim-vault`'s; see its README.
