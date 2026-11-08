# Phase 1 — Worker HTTP API (Person 2)

## Context
Phase 1 is to build one standalone Worker that stores chunks and exposes them over HTTP (`docs/phase1_project_blueprint.md`). The user is Person 2 and owns the HTTP layer: the routes, the chunk handlers, validation, the error format, the server wiring and the handler tests. Person 1 owns the filesystem storage. The repo is empty apart from docs. This plan creates the Go module, the agreed `Storage` interface, an in-memory stub so the worker runs now, the API package and its tests, `main.go`, and the Docker files.

## Decisions locked in
- **Go and routing:** `go 1.22` in go.mod. Module path is `github.com/Sahas-out/DistributedFileStorage` (the origin remote). Routing uses only the standard library 1.22 `ServeMux` (`"PUT /internal/chunks/{chunk_id}"` with `r.PathValue`). No third-party dependencies.
- **Chunk ID:** must match `^[A-Za-z0-9_-]{1,128}$`, otherwise `400 invalid_chunk_id`.
- **Errors:** every error is JSON: `{"error":{"code":"...","message":"..."}}`, sent with `Content-Type: application/json`.
- **PUT** `/internal/chunks/{id}`:
  - The body is capped at 1 MiB (1,048,576 bytes, configurable) via `http.MaxBytesReader`. Over the cap returns `413 chunk_too_large`.
  - The body is buffered (at most 1 MiB) and hashed. If an optional `X-Chunk-Checksum` request header doesn't match, the handler returns `400 checksum_mismatch` and nothing is written.
  - Then `storage.Put`. Overwrite is idempotent.
  - Returns `201` with the `ChunkInfo` JSON.
- **GET** `/internal/chunks/{id}`:
  - `Open`, then read the whole chunk (at most 1 MiB) while hashing.
  - If the hash differs from the stored checksum, return `500 chunk_corrupted`. The controller can use this code to fall back to another copy and repair.
  - Otherwise return `200` with `Content-Type: application/octet-stream`, `Content-Length`, `X-Chunk-Checksum: <bare hex>` and `X-Chunk-Size`.
  - Verifying before sending is the only way to report corruption with a proper status code, and it costs little at 1 MiB.
- **DELETE** `/internal/chunks/{id}`: returns `204`, including when the chunk doesn't exist (idempotent).
- **List** `GET /internal/workers/{worker_id}/chunks`: `{worker_id}` is ignored for now. Returns `200 {"worker_id":<configured>,"count":n,"chunks":[...]}`. `chunks` is always an array, never `null`.
- **Health:** `GET /healthz` returns `200 {"status":"ok","worker_id":...}`.
- **Error mapping:** `storage.ErrNotFound` → 404 `chunk_not_found`. `storage.ErrInvalidChunkID` → 400. `ErrMissingChecksum` and anything else → 500 `storage_error`; the details are logged and not leaked to the caller.
- **Shared types:** the wire type `ChunkInfo` and `ErrorResponse` live in `pkg/protocol/chunk.go`. `storage` declares `type ChunkInfo = protocol.ChunkInfo` (a type alias), so the agreed interface signature is unchanged, there is exactly one definition, and the controller can import it later. Header-name constants also go in `protocol`.

## Files to create
| File | Contents |
|---|---|
| `go.mod` | `module github.com/Sahas-out/DistributedFileStorage`, `go 1.22` |
| `pkg/protocol/chunk.go` | `ChunkInfo{ID,Size,Checksum}` with the agreed JSON tags; `ChunkList`; `ErrorResponse`/`ErrorBody`; constants `HeaderChunkChecksum = "X-Chunk-Checksum"` and `HeaderChunkSize = "X-Chunk-Size"` |
| `worker/internal/storage/storage.go` | The `Storage` interface exactly as in the blueprint; the `ChunkInfo` alias; sentinel errors `ErrNotFound`, `ErrInvalidChunkID`, `ErrMissingChecksum`; a comment marking it as the shared contract with Person 1 |
| `worker/internal/storage/memory.go` | `NewMemory()`: an in-memory implementation backed by a map and a `sync.RWMutex` that computes SHA-256 on `Put`, returns sorted results from `List`, and returns `ErrNotFound` for missing chunks. It's a stand-in until Person 1's `filesystem.go` lands. |
| `worker/internal/config/config.go` | `Load()` reads environment variables: `WORKER_ID` (defaults to the hostname), `PORT` (default `8080`), `DATA_DIR` (default `/data`), `MAX_CHUNK_BYTES` (default `1048576`). Returns an error on bad values. |
| `worker/internal/api/response.go` | `writeJSON`, `writeError(w, status, code, msg)`, `mapStorageError(err)` |
| `worker/internal/api/routes.go` | `type Server struct{store storage.Storage; workerID string; maxChunk int64; log *slog.Logger}`; `NewServer(...)`; `Handler() http.Handler` that registers the routes and wraps them in logging and panic-recovery middleware (a panic returns a JSON 500) |
| `worker/internal/api/chunk_handlers.go` | `handlePutChunk`, `handleGetChunk`, `handleDeleteChunk`, `handleListChunks`, `handleHealth`, and `validChunkID` (a precompiled regex) |
| `worker/internal/api/chunk_handlers_test.go` | Table-driven tests with `httptest` against a configurable mock `Storage` (see below) |
| `worker/cmd/worker/main.go` | Loads the config and sets up `slog` with a JSON handler. Creates the store with `storage.NewMemory()` behind a single `// TODO(person1): storage.NewFilesystem(cfg.DataDir)` line. Runs an `http.Server` with `ReadHeaderTimeout`, and on SIGINT/SIGTERM shuts down gracefully (needed for `docker stop`). |
| `worker/Dockerfile` | Multi-stage build: `golang:1.22-alpine` builds a static `CGO_ENABLED=0` binary; the runtime image is `alpine` with a non-root user, `VOLUME /data`, `EXPOSE 8080`, and a `HEALTHCHECK` using `wget` against `/healthz` |
| `worker/.dockerignore` | Ignores `.git`, test binaries and other clutter |
| `docker-compose.yml` | One `worker1` service: `container_name: worker1`, `WORKER_ID=worker1`, a named volume `worker1-data:/data`, port `8081:8080`. The build context is the repo root and the Dockerfile is `worker/Dockerfile`, because the module root is the repo root. |
| `README.md` | A short guide to building, running and testing, with curl examples for each endpoint |

## Handler tests (mock Storage)
The mock is a struct with func fields (`putFn`, `openFn`, …), so each test can inject results or errors. It records calls so tests can check, for example, that `Put` was never called when the checksum didn't match. The cases:
- PUT:
  - happy path returns 201 with JSON matching the `ChunkInfo`
  - an invalid ID (`../x`, `a/b`, an empty ID, an ID of 129 characters, `a.b`) returns 400 without calling storage
  - an oversized body returns 413
  - a mismatched `X-Chunk-Checksum` returns 400 and `Put` is never called
  - a matching checksum returns 201
  - a storage error returns 500 with the JSON error shape
- GET:
  - happy path returns the exact bytes, with headers set correctly
  - a missing chunk returns 404 `chunk_not_found`
  - corrupted bytes (the stored checksum doesn't match the content) return 500 `chunk_corrupted`
  - an invalid ID returns 400
- DELETE: an existing chunk returns 204, and a missing chunk also returns 204. A storage error returns 500.
- List: an empty store returns `"chunks":[]` (not `null`). With entries, `count` is correct, and the response is the same whatever the worker ID in the path.
- Healthz returns 200.
- Wrong method (e.g. `POST /internal/chunks/x`): the stdlib mux returns 405.
- A round trip through `storage.NewMemory()`: PUT, GET (the bytes match), List, DELETE, then GET returns 404.

## Verification
1. `go vet ./...` and `go test ./... -race` (if the race detector isn't available on Windows without cgo, run without `-race`).
2. `go run ./worker/cmd/worker` with `PORT=8081`, then curl each endpoint:
   - PUT a random 1 MiB file and compare the returned checksum with `sha256sum`.
   - GET it into a file and `cmp` it against the original.
   - Check the list, the oversized-body response, and that a bad ID returns 400.
   - DELETE returns 204 twice in a row.
3. `docker compose up --build -d`, repeat the curl smoke test against `:8081`, check that `docker ps` shows the container as healthy, then `docker compose down`.
   - Limitation: the persistence-across-restart item in the definition of done can only pass once Person 1's filesystem store replaces `NewMemory`. The volume is already wired up for it.

## Notes for Person 1 (to tell the teammate)
- `storage.go` (the interface, the alias, and the error values) and `memory.go` exist. `filesystem.go` should return `ErrNotFound`/`ErrInvalidChunkID` (wrapped is fine; the API uses `errors.Is`) and `ErrMissingChecksum` when a chunk exists without its checksum metadata.
- Proposed constructor: `NewFilesystem(dir string) (*Filesystem, error)`.

## Implementation notes (changes from the plan above)
- **Chunk ID validation** is `storage.ValidChunkID` in `worker/internal/storage/storage.go`, not a private helper in `api`. Person 1's filesystem store should call it too, so both layers enforce the same rule.
- **PUT reads the body before validating.** It reads the whole body (capped at 1 MiB) before checking the chunk ID or the checksum header. If the server rejects a request while ~1 MiB of body is still unread, it closes the connection. The resulting TCP reset discarded the 400 response when going through Docker's port proxy, and curl hung until it timed out. The regression test `rejected PUTs still consume the whole body` covers this. A side effect: an oversized body with an invalid ID returns 413, not 400.
- **GET also answers HEAD.** The Go 1.22 mux routes HEAD to GET handlers. HEAD returns the headers without a body.
- **`.dockerignore`** lives at the repo root, because the build context is the repo root. A `worker/.dockerignore` would be ignored.
