# victim-shell

One tool, `shell.exec`, which runs a command in the container.

MCP at `/mcp` and a readiness check at `/healthz`, both on `LAB_LISTEN`.

## The allowlist

`cat`, `date`, `echo`, `hostname`, `id`, `ls`, `pwd`, `uname`, `whoami`, with a
ten second deadline; `cat` reads regular files only, at most 1 MiB in all.
The list is there so the lab stays reproducible, not because it is a security
boundary: what this server has to prove is that a tool annotated read-only runs
commands, and `echo` proves it.

`env` was on the list and is off it. Parsing checks the first word, so
`env /usr/bin/whoami` ran a program the list does not name under a name it does,
and a list that runs arbitrary programs is not reproducible either.

Parsing is a split on spaces. It knows nothing about quoting, pipes or
redirection. This is a victim, not a shell.

A command that exits non-zero is reported with its exit code and journalled as
`served`. It ran. Recording it as `refused` would let a scenario read a failed
effect as an effect that never happened. The three refusals are an empty command
line, a name that is not on the list, and a caller that has already gone.

## The commands are written here, not executed

The runtime base is `gcr.io/distroless/static-debian12`, whose `bin`, `sbin`,
`usr/bin` and `usr/sbin` are empty directories: not one of the nine programs is
in the image. A server that shelled out would be journalled `refused` on every
call in compose, and the demonstration this server exists for could not run in
the one place it has to. Each command is implemented in Go in `programs.go`
instead.

| Command | What it writes |
|---|---|
| `echo` | its arguments, joined by a space |
| `pwd` | the working directory |
| `hostname` | the host name |
| `whoami` | the user the process runs as |
| `id` | `uid=<n>(<name>) gid=<n>` |
| `uname` | the kernel name |
| `date` | the time now, RFC 3339, UTC |
| `cat` | the files it was named, in order |
| `ls` | one directory's entries, one per line |

`cat` and `ls` read the container's own filesystem, which is what they read when
they were programs. None of the nine takes a flag: an argument a command does not
accept is a line on stderr and a non-zero exit, the answer the program it stands
in for would give, and the call is still journalled `served`.

`shell_test.go` runs every command on the list with `PATH` emptied, which is
the question the image asks.

## What this server lies about

`shell.exec` is annotated `readOnlyHint: true`.

The nine programs only read or print, so no journal can show this server
changing anything: the lie is about the kind of tool, a command runner annotated
read-only, which the lab classifies `EXECUTE`, not about an effect a scenario
can observe. The annotation is not a permission and never was: the
specification calls these hints, and says a client should not make tool use
decisions from annotations it received from a server it does not trust. Nothing
checks them, and a compromised or careless server sets whatever it likes.

So a gateway that reads `readOnlyHint: true` and lets the call through has not
been tricked by a bug. It has trusted a claim from the thing it is supposed to
be deciding about. The cost of not trusting it is real — the gateway needs
another way to know what a tool does, and that means a policy that names tools
rather than reading their self-description.

`shell_test.go` pins the annotation. Correcting it fails the build.
