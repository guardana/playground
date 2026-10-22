# Scan artifacts

Plain-text files the verifier's `guardana scan` grades. Each case holds a
`control` and a `defect` directory with the same file names; exactly one file
differs, and that file carries the one defect the scan must report.

## The catalogue

| Case | File | The defect | Scanned by |
|---|---|---|---|
| `pickle-import` | `model.pkl` | a protocol-0 GLOBAL into a module that does not exist | `scan-01-a-pickle-that-imports-on-load-is-reported` |
| `remote-code-config` | `config.json` | `_attn_implementation` names a kernel from a repository instead of `eager` | `scan-02-a-config-that-loads-remote-code-is-reported` |
| `unknown-import` | `train.py` | imports `torchutilz`, a package nobody publishes, instead of `json` | `scan-03-a-script-that-imports-an-unknown-package-is-reported` |
| `planted-secret` | `settings.yaml` | an AWS-shaped access key id instead of one read from the environment | `scan-04-a-secret-in-an-artifact-is-reported` |

## Nothing loads them

The lab copies a variant into a container with no network and hands it to the
scan. No step unpickles, imports or runs a file, and each file stays inert if
somebody did:

- A pickle holds no opcode that calls or builds anything, and its only import
  names `clab_never_a_module`, which nothing provides, so a load fails on it.
- Python keeps every statement inside a function nothing calls, under a name
  no tool runs on its own.
- A host, if any, is under `.invalid`, which cannot resolve. A model repository
  a config names holds `--`, which the hub client refuses before any request.
- The access key id has AWS's documented example shape and opens nothing.

`internal/labcheck` walks every pickle's opcodes without loading it, refuses a
file or package by the absent module's name anywhere in the tree, a host
outside `.invalid`, a fetchable model repository, Python run on import or under
a name tooling runs, and a byte outside printable ASCII, and checks that control
and defect differ in one file.
`.gitattributes` keeps the bytes exact in every checkout.
