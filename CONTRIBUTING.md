# Contributing to dropcrate

Thanks for your interest in improving dropcrate! This guide covers the local
workflow and what CI expects.

## Prerequisites

- **Go 1.25+**
- **Docker** (for the local infra: MySQL, Redis, MinIO, Kafka)
- **buf** — only if you change the gRPC API (`brew install buf`)

## Getting started

```shell
make up        # start MySQL, Redis, MinIO, Kafka
make deps      # download and verify modules
make migrate   # create the schema
make run       # start the HTTP + gRPC servers
```

Configuration comes from the environment; copy `.env.example` to `.env` and
adjust as needed. The defaults target the bundled `docker-compose` stack.

## Development workflow

| Task                         | Command       |
| ---------------------------- | ------------- |
| Format code                  | `make fmt`    |
| Run tests                    | `make test`   |
| Lint (golangci-lint)         | `make lint`   |
| Regenerate gRPC stubs        | `make proto`  |
| Build the binary             | `make build`  |
| Build the container image    | `make docker` |

Run `make help` for the full list.

## Before opening a pull request

CI runs on every push and PR; please make sure these pass locally first:

1. **`make fmt`** — code is gofmt-clean.
2. **`go vet ./...`** and **`make lint`** — no vet or lint findings.
3. **`go test -race ./...`** — all tests pass under the race detector.
4. If you touched `proto/`, run **`make proto`** and commit the regenerated
   stubs — CI fails if they are stale.

## Guidelines

- Keep the two transports (HTTP and gRPC) behavior-consistent; both call the
  same `internal/service` layer, so put shared logic there.
- Optional integrations (Kafka events, API-key auth, signed URLs) are
  **off by default** and must stay that way — a fresh checkout should boot and
  behave exactly as before unless explicitly configured.
- Add tests for new behavior. The service layer depends on interfaces
  (`Repository`, `Cache`, `storage.Storage`, `events.Publisher`) so it can be
  tested with in-memory fakes — see `internal/service/service_test.go`.
- Match the surrounding style: small packages, clear doc comments, errors
  wrapped with `%w`.

## Commit messages

Write a concise imperative subject line ("Add …", "Fix …") and a body that
explains the *why*. Group related changes into focused commits.
