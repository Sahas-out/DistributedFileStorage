# Distributed Object Store

A mini distributed cloud object store: one controller, four storage workers, and clients. Objects are split into 1 MiB chunks and each chunk is stored on two workers. See [docs/Mini_Distributed_Cloud_Object_Store.pdf](docs/Mini_Distributed_Cloud_Object_Store.pdf) for the project statement.

**Current status: Phase 1.** One standalone worker that stores chunks and serves them over HTTP. See [docs/phase1_project_blueprint.md](docs/phase1_project_blueprint.md) and [docs/phase1_worker_http_api_plan.md](docs/phase1_worker_http_api_plan.md).

> Until Person 1's filesystem store lands, the worker uses an in-memory store (`storage.NewMemory`), so chunks do not survive a restart yet. The Docker volume is already wired up for it.

## Layout

```text
pkg/protocol/          shared wire types (ChunkInfo, ChunkList, ErrorResponse, header names, error codes)
worker/cmd/worker/     worker entry point
worker/internal/api/   HTTP routes, handlers, middleware, handler tests
worker/internal/storage/  Storage interface (shared contract), in-memory store
worker/internal/config/   environment-based configuration
worker/Dockerfile
docker-compose.yml
```

## Requirements

- Go 1.22 or newer
- Docker with Docker Compose v2 (for the container setup)

## Build, test, run

```sh
go vet ./...
go test ./...

# run locally (PowerShell: $env:PORT=8081; go run ./worker/cmd/worker)
PORT=8081 WORKER_ID=worker1 go run ./worker/cmd/worker
```

### Docker

```sh
docker compose up --build -d     # worker1 on http://localhost:8081
docker compose ps                # should show (healthy)
docker compose logs -f worker1
docker compose down              # add -v to also delete the data volume
```

### Configuration (environment variables)

| Variable | Default | Meaning |
|---|---|---|
| `WORKER_ID` | hostname | ID reported in listings and health |
| `PORT` | `8080` | HTTP listen port |
| `DATA_DIR` | `/data` | Directory for chunk storage |
| `MAX_CHUNK_BYTES` | `1048576` | Largest accepted chunk (1 MiB) |

## Worker HTTP API

| Method | Endpoint | Success | Notes |
|---|---|---|---|
| `PUT` | `/internal/chunks/{chunk_id}` | `201` + `ChunkInfo` JSON | Body is raw bytes, at most 1 MiB. Optional `X-Chunk-Checksum` request header; a mismatch returns `400`. Re-PUT overwrites. |
| `GET` | `/internal/chunks/{chunk_id}` | `200` + raw bytes | Headers: `Content-Type: application/octet-stream`, `Content-Length`, `X-Chunk-Checksum`, `X-Chunk-Size`. Bytes are checked against the recorded checksum before sending. |
| `DELETE` | `/internal/chunks/{chunk_id}` | `204` | Also `204` if the chunk doesn't exist (idempotent). |
| `GET` | `/internal/workers/{worker_id}/chunks` | `200` + `ChunkList` JSON | `{worker_id}` is currently ignored. |
| `GET` | `/healthz` | `200` | `{"status":"ok","worker_id":"..."}` |

Chunk IDs must match `^[A-Za-z0-9_-]{1,128}$`. Checksums are lowercase SHA-256 hex.

### Errors

All errors are JSON:

```json
{"error": {"code": "chunk_not_found", "message": "chunk abc not found"}}
```

| Status | Code | When |
|---|---|---|
| 400 | `invalid_chunk_id` | Chunk ID fails validation |
| 400 | `invalid_request` | Malformed `X-Chunk-Checksum`, or the body couldn't be read |
| 400 | `checksum_mismatch` | PUT body doesn't match `X-Chunk-Checksum`; nothing is stored |
| 404 | `chunk_not_found` | GET of a missing chunk |
| 413 | `chunk_too_large` | PUT body over `MAX_CHUNK_BYTES` |
| 500 | `chunk_corrupted` | Stored bytes don't match the recorded checksum; the controller should use another replica |
| 500 | `storage_error` | Any other storage failure; details are logged, not returned |
| 500 | `internal_error` | A panic was recovered |

### Examples (bash)

```sh
W=http://localhost:8081
head -c 1048576 /dev/urandom > chunk.bin
SUM=$(sha256sum chunk.bin | cut -d' ' -f1)

curl -s -X PUT --data-binary @chunk.bin -H "X-Chunk-Checksum: $SUM" $W/internal/chunks/obj1_0
curl -s -D - -o out.bin $W/internal/chunks/obj1_0 && cmp chunk.bin out.bin && echo "bytes match"
curl -s $W/internal/workers/worker1/chunks
curl -s -o /dev/null -w "%{http_code}\n" -X DELETE $W/internal/chunks/obj1_0   # 204
curl -s $W/internal/chunks/obj1_0                                              # 404 JSON
curl -s -X PUT --data-binary @chunk.bin $W/internal/chunks/bad.id              # 400 JSON
```

On Windows, use `curl.exe` in PowerShell (plain `curl` is an alias for `Invoke-WebRequest`).
