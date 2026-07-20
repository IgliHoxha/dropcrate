# dropcrate

**dropcrate** is a small, self-hostable object-storage and file-sharing API
written in Go. Upload a file, get back a short download link, and let it expire
automatically after a configurable time-to-live.

- Bytes are stored in any **S3-compatible** bucket (MinIO locally, AWS S3 in prod).
- Metadata lives in **MySQL**.
- Hot lookups are cached in **Redis**.
- Uploaded links can **expire** — set a per-file TTL or fall back to the server default.

## Architecture

```
                 ┌──────────────┐
   HTTP  ─────▶  │   api (chi)  │
                 └──────┬───────┘
                        │
                        ▼
                 ┌──────────────┐
   gRPC  ─────▶  │     rpc      │
                 └──────┬───────┘
                        │
                        ▼
                 ┌──────────────┐
                 │   service    │  upload / download / delete
                 └──────┬───────┘
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
  ┌───────────┐   ┌───────────┐   ┌────────────┐
  │   MySQL   │   │   Redis   │   │ S3 / MinIO │
  │  (files)  │   │  (cache)  │   │ (objects)  │
  └───────────┘   └───────────┘   └────────────┘
```

Both the HTTP and gRPC transports call the same `service` layer, so they share
one set of upload/download/delete use cases.

## Project layout

```
.
├── main.go                 # entrypoint; embeds migrations
├── cmd/                    # cobra commands (serve, migrate, sweep)
├── internal/
│   ├── api/                # HTTP handlers, router, middleware
│   ├── rpc/                # gRPC transport (+ generated stubs)
│   ├── cache/              # Redis metadata cache
│   ├── events/             # domain events + Kafka publisher (optional)
│   ├── config/            # env-based configuration
│   ├── database/          # MySQL connection + migrator
│   ├── files/             # domain model + repository
│   ├── service/           # use cases wiring the layers together
│   ├── storage/           # S3-compatible object store
│   └── sweeper/           # background reaper for expired files
├── proto/                 # protobuf/gRPC definitions
├── migrations/            # SQL schema, applied by `dropcrate migrate`
├── docs/openapi.yaml      # API specification
├── buf.yaml, buf.gen.yaml # buf config for `make proto`
├── Dockerfile             # distroless container image
└── .github/workflows/     # CI (build, vet, test, lint)
```

## Getting started

```shell
# 1. Start MySQL, Redis and MinIO
make up

# 2. Resolve dependencies (first run only)
make tidy

# 3. Create the schema
make migrate

# 4. Run the API
make run
```

Configuration is read from the environment; copy `.env.example` to `.env` and
adjust as needed. Defaults target the bundled `docker-compose` stack.

## API

dropcrate speaks two protocols over the same use cases: **HTTP** (default
`:8080`) and **gRPC** (default `:9090`, set with `GRPC_ADDR`).

### Authentication

Authentication is **off by default**. Set `API_KEYS` (comma-separated) to
require a bearer key on the mutating operations — **upload** and **delete** —
on both transports. Reads stay open so shareable download links keep working.

```shell
export API_KEYS=secret-key
# HTTP
curl -H "Authorization: Bearer secret-key" -F "file=@photo.png" http://localhost:8080/v1/files
# gRPC (metadata)
grpcurl -H "authorization: Bearer secret-key" -plaintext -d '{"id":"…"}' \
  localhost:9090 dropcrate.v1.FileService/Delete
```

Without a valid key, HTTP returns `401` and gRPC returns `Unauthenticated`.

### Signed download links

Set `DOWNLOAD_SIGNING_KEY` to make download URLs **expiring and unforgeable**:
the `download_url` returned on upload carries an `exp` and an HMAC `sig` over
the file id, valid for `DOWNLOAD_URL_TTL` (default `1h`). Requests to
`/v1/files/{id}` without a valid, unexpired signature get `403`. Disabled by
default (downloads open by id). This applies to the HTTP transport; gRPC
`Download` remains id-based.

### HTTP

| Method   | Path                   | Description                          | Auth |
| -------- | ---------------------- | ------------------------------------ | ---- |
| `POST`   | `/v1/files`            | Upload a file (multipart `file`)     | ✓    |
| `GET`    | `/v1/files/{id}`       | Download the file bytes              |      |
| `GET`    | `/v1/files/{id}/meta`  | Fetch metadata only                  |      |
| `DELETE` | `/v1/files/{id}`       | Delete a file                        | ✓    |
| `GET`    | `/healthz`             | Liveness probe                       |      |
| `GET`    | `/readyz`              | Readiness probe (pings MySQL/Redis/S3) |    |
| `GET`    | `/metrics`             | Prometheus metrics                   |      |

### Example

```shell
# Upload, keeping the file for 24 hours
curl -F "file=@photo.png" -F "ttl=24h" http://localhost:8080/v1/files

# Response
{
  "id": "7c2f...",
  "filename": "photo.png",
  "content_type": "image/png",
  "size": 20481,
  "created_at": "2026-07-20T10:00:00Z",
  "expires_at": "2026-07-21T10:00:00Z",
  "download_url": "http://localhost:8080/v1/files/7c2f..."
}

# Download
curl -OJ http://localhost:8080/v1/files/7c2f...
```

Pass `ttl=0` (or `ttl=never`) to keep a file indefinitely, or omit `ttl` to use
the server's `DEFAULT_TTL`.

### gRPC

`dropcrate.v1.FileService` exposes `Upload` (client-streaming), `Download`
(server-streaming), `GetMetadata`, and `Delete`. Server reflection is enabled,
so tools like [`grpcurl`](https://github.com/fullstorydev/grpcurl) work without a
compiled `.proto`:

```shell
# Discover the API
grpcurl -plaintext localhost:9090 list dropcrate.v1.FileService

# Upload (first stream message is 'info', the rest are base64 'chunk's)
CHUNK=$(base64 < photo.png | tr -d '\n')
printf '{"info":{"filename":"photo.png","content_type":"image/png","ttl_seconds":86400}}\n{"chunk":"%s"}\n' "$CHUNK" \
  | grpcurl -plaintext -d @ localhost:9090 dropcrate.v1.FileService/Upload

# Metadata / delete
grpcurl -plaintext -d '{"id":"7c2f..."}' localhost:9090 dropcrate.v1.FileService/GetMetadata
grpcurl -plaintext -d '{"id":"7c2f..."}' localhost:9090 dropcrate.v1.FileService/Delete
```

`ttl_seconds` follows the same convention as the HTTP `ttl`: `0` uses the server
default, a negative value never expires, and a positive value sets the lifetime
in seconds.

Regenerate the stubs after editing `proto/` with `make proto` (needs
[`buf`](https://buf.build)).

## Domain events (Kafka)

dropcrate can publish a best-effort domain event whenever a file's lifecycle
changes. Publishing is **off by default** and only activates when `KAFKA_BROKERS`
is set — otherwise a no-op publisher is used and nothing changes. A broker
problem never blocks or fails a request; events are emitted asynchronously.

| Event                | Emitted when                          |
| -------------------- | ------------------------------------- |
| `file.uploaded`      | an upload completes                   |
| `file.deleted`       | a file is deleted                     |
| `file.expired`       | the reaper purges an expired file     |

Each event is JSON, keyed by file id, on topic `KAFKA_TOPIC_PREFIX` + event type
(e.g. `dropcrate.file.uploaded`). Enable it against the bundled broker:

```shell
make up                                    # now also starts a local Kafka broker
KAFKA_BROKERS=127.0.0.1:9092 make run
```

## Observability

- **Metrics** — Prometheus counters and latency histograms for both transports,
  plus Go runtime metrics, at `GET /metrics`.
- **Tracing** — optional OpenTelemetry spans on both transports, off unless
  `OTEL_EXPORTER_OTLP_ENDPOINT` is set. `make up` starts a local Jaeger
  (UI at http://localhost:16686):

  ```shell
  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4317 make run
  ```

## Expiry & reaping

Each file may carry an `expires_at`. Expired files are treated as gone the
moment they are requested (returning `404`), and their bytes are reclaimed by a
background reaper:

- **In-process:** `serve` runs a sweeper every `SWEEP_INTERVAL` that deletes
  expired files in batches of `SWEEP_BATCH`. Set `SWEEP_INTERVAL=0` to disable.
- **One-off / cron:** `dropcrate sweep` reaps everything expired and exits —
  handy as a Kubernetes CronJob when you prefer not to reap in-process.

```shell
make sweep   # or: go run . sweep
```

## Docker

```shell
make docker                              # build dropcrate:latest (distroless)
docker run --rm -p 8080:8080 -p 9090:9090 --env-file .env dropcrate:latest
```

The image is a static `distroless` binary running as a non-root user. It bundles
the migrations, so `docker run ... dropcrate:latest migrate` works too.

## CI

`.github/workflows/ci.yml` runs on every push and pull request: `go vet`,
`go build`, `go test -race`, and `golangci-lint`.

## Development

```shell
make test    # run unit tests
make lint    # golangci-lint
make build   # compile ./dropcrate
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow and what CI expects.

## License

[MIT](LICENSE)
