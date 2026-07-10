# example

A small module showing pantograph end to end: an upload pipeline where `api/`
hands off to `worker/` through a queue, and a token-auth flow in `auth/`. It
has its own `go.mod` so it stays out of the parent's build and tests, and it
pins pantograph via a `replace` directive so it always runs against the source
in this repo.

Each domain lands on one page: [docs/pipeline.md](docs/pipeline.md) and
[docs/auth.md](docs/auth.md). Packages render as lanes, calls between tagged
functions as arrows. The api-to-worker hop in `pipeline` is a handoff: the
upload crosses a channel, so there is no call for the walk to follow, and the
`jobs` annotation names the edge instead. The kind-to-shape vocabulary and the
domain grouping live in [pantograph.yaml](pantograph.yaml).

Regenerate:

```bash
cd example
go generate ./docs # or: go tool pantograph generate -out ./docs
```
