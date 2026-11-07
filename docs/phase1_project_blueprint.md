# Distributed Object Store — Phase 1 Blueprint

## Phase 1 Goal

Build and test **one standalone Worker** that stores binary chunks on the local filesystem and exposes HTTP APIs to put, retrieve, delete, and list chunks. Controller integration, chunking, replication, and failure recovery come in later phases.

## Work Division

| Person | Responsibility | Deliverables |
|---|---|---|
| **Person 1 — Storage** | Implement filesystem-backed storage; write chunks safely using temporary files and atomic rename; calculate and retain SHA-256 checksums; implement `Put`, `Open`, `Delete`, and `List`; write storage unit tests. | `worker/internal/storage`, `worker/internal/integrity`, storage tests |
| **Person 2 — HTTP API** | Set up the Go HTTP server and routes; implement chunk PUT/GET/DELETE/LIST handlers; validate chunk IDs and requests; define HTTP status codes and error responses; write handler tests using a mock `Storage`. | `worker/internal/api`, server wiring, handler tests |

### Shared agreement before coding

Both people should agree on the interface below first. Person 1 implements it; Person 2 depends on it. This keeps the two tasks independent.

## Worker Interface Blueprint

```go
package storage

import (
    "context"
    "io"
)

type ChunkInfo struct {
    ID       string `json:"chunk_id"`
    Size     int64  `json:"size"`
    Checksum string `json:"checksum"` // SHA-256 hex digest
}

type Storage interface {
    Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error)
    Open(ctx context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error)
    Delete(ctx context.Context, chunkID string) error
    List(ctx context.Context) ([]ChunkInfo, error)
}
```

`Open` returns the stored chunk stream and its recorded information. The HTTP layer should stream the body rather than loading the entire chunk into memory. Person 1 should ensure chunk IDs cannot escape the configured storage directory.

## Phase 1 HTTP Contract

| Method | Endpoint | Behaviour |
|---|---|---|
| `PUT` | `/internal/chunks/{chunk_id}` | Request body is raw chunk bytes; persist the chunk; return `ChunkInfo`. |
| `GET` | `/internal/chunks/{chunk_id}` | Return raw chunk bytes with checksum/size headers. |
| `DELETE` | `/internal/chunks/{chunk_id}` | Delete the chunk; return `204`. |
| `GET` | `/internal/workers/{worker_id}/chunks` | Return the worker's chunk inventory. |

Use meaningful errors (`400` invalid ID/request, `404` missing chunk, `500` storage failure). Agree on the exact JSON error shape during integration. Worker registration and heartbeat communication with the Controller can be added in a later phase.

## Repository Structure

A single Go module at the repository root keeps the project straightforward. The `worker`, `controller`, and `client` remain separate applications.

```text
distributed-object-store/
├── go.mod
├── go.sum
├── README.md
├── docker-compose.yml
│
├── pkg/
│   └── protocol/
│       ├── chunk.go             # Shared request/response types
│       └── worker.go            # Worker registration/heartbeat types
│
├── worker/
│   ├── cmd/
│   │   └── worker/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/
│   │   │   ├── routes.go
│   │   │   ├── chunk_handlers.go
│   │   │   └── response.go
│   │   ├── storage/
│   │   │   ├── storage.go       # Storage interface and ChunkInfo
│   │   │   ├── filesystem.go    # Filesystem implementation
│   │   │   └── filesystem_test.go
│   │   ├── integrity/
│   │   │   └── checksum.go
│   │   └── config/
│   │       └── config.go
│   ├── Dockerfile
│   └── .dockerignore
│
├── controller/
│   ├── cmd/
│   │   └── controller/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/                 # Client-facing REST endpoints
│   │   ├── object/              # Object upload/download orchestration
│   │   ├── chunk/               # 1 MB chunking and ordering
│   │   ├── metadata/             # Object/chunk metadata repository
│   │   ├── workers/              # Worker registry and HTTP client
│   │   ├── placement/            # Worker selection
│   │   ├── replication/          # Replica creation and repair
│   │   ├── health/               # Heartbeats and failure detection
│   │   └── config/
│   │       └── config.go
│   └── Dockerfile
│
├── client/
│   ├── cmd/
│   │   └── client/
│   │       └── main.go
│   └── internal/
│       └── api/
│           └── client.go         # Calls Controller REST APIs
│
└── tests/
    └── integration/              # End-to-end tests added in later phases
```

The Controller and Client directories are structural placeholders for now; do not implement them in Phase 1. Add tests alongside each package as needed.

## Phase 1 Definition of Done

- A chunk can be PUT, GET, DELETE, and listed through the Worker HTTP API.
- Retrieved bytes match the uploaded bytes.
- SHA-256 checksum and size are calculated and returned consistently.
- Missing chunks and invalid chunk IDs produce appropriate errors.
- Worker data survives container restart through a persistent Docker volume.
- Person 1's storage tests and Person 2's handler tests pass.
