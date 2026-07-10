# pantograph

Flow diagrams for a Go codebase, generated from the code itself.

Tag functions with `//pantograph:` comments. pantograph reads the call graph,
draws the tagged functions as boxes, and connects them by actual calls.
Regenerate and the diagram matches the code again.

## Install

```bash
go get -tool github.com/ilyalavrenov/pantograph@latest
go tool pantograph # writes docs/flows/
```

`pantograph --help` lists the other commands.

## Tagging a flow

```go
//pantograph:ingest kind=external
func (c *Client) Fetch(...) (*Batch, error) { ... }

//pantograph:ingest kind=process
func (w *Worker) Process(...) error { ... }
```

Both functions land in the `ingest` diagram. If `Fetch` calls `Process`,
an arrow appears between them.

`kind=` picks the shape. Define the vocabulary in `pantograph.yaml`:

```yaml
kinds:
  external: hexagon # a call that leaves the process
  store: cylinder
  decision: diamond
  process: "" # plain rectangle
domains:
  pipeline:
    flows:
      - ingest
      - report
```

`domains:` groups flows onto one page. An omitted `kind=` renders as a plain
rectangle. An undeclared kind is an error.

## Example

[`example/`](example/) is a runnable module: an API handler validates an
upload and queues it, a worker stores it. See the generated
[diagrams](example/docs/pipeline.md) and its
[`pantograph.yaml`](example/pantograph.yaml).

## How it works

Rendering uses the [d2](https://oss.terrastruct.com/d2) Go library, no CLI or
browser. Each domain becomes a `<domain>.md` page with its diagram at
`svg/<domain>.svg`, written under the output directory (`-out`, default
`docs/flows`). Nodes group into lanes by package.
