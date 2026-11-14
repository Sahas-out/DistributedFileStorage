# Step 7: Packages

**Goal:** split step 6's single file into **packages** laid out exactly like the real project, so each part has a clear owner and a clear boundary. The behaviour is unchanged; only the debug route is gone, because the real worker doesn't have it.

## Run it

```powershell
go run ./learn/step07-packages/worker/cmd/worker
```

It listens on **8097**, and the curl commands from step 5 work against it. Note that the path now points at `worker/cmd/worker`, the folder that contains `package main`.

## Where step 6's code went
```
step07-packages/
├── pkg/
│   └── protocol/
│       └── chunk.go            ChunkInfo, ChunkList, ErrorResponse, header names, error codes
└── worker/
    ├── cmd/worker/
    │   └── main.go             package main: wiring only (~10 lines)
    └── internal/
        ├── storage/
        │   ├── storage.go      Storage interface, sentinel errors, ValidChunkID, ChunkInfo alias
        │   └── memory.go       Memory implementation
        └── api/
            ├── routes.go       Server, NewServer, Handler (the routes)
            ├── chunk_handlers.go   the five handlers, storageError, hash helpers
            └── response.go     writeJSON, writeError
```
Who imports whom (arrows mean "imports"):

```
main ──► api ──► storage ──► protocol
  └────────────►  storage      api ──► protocol
```
`protocol` imports nothing from the project, and `storage` doesn't know `api` exists. Dependencies only point **downward**, and Go *forbids* import cycles (A imports B imports A), so this layering is enforced by the compiler.

## New Go concepts

### One folder = one package
Every `.go` file in a folder must declare the same `package` name, and they're compiled together. Files in the same package **see each other's names directly**, with no imports needed. For example, `chunk_handlers.go` calls `writeError`, which lives in `response.go`. Splitting a package into files is purely for readability.

### Import paths come from `go.mod`
The repo's `go.mod` says `module github.com/Sahas-out/DistributedFileStorage`. A package's import path is that module path plus its folder:

```go
import "github.com/Sahas-out/DistributedFileStorage/learn/step07-packages/worker/internal/storage"
```

You then refer to it by the **package name** declared in its files, usually the last folder name: `storage.NewMemory()`, `api.NewServer(...)`. The module path looks like a URL, but nothing is downloaded. Go finds these folders locally.

### Exported vs unexported: the capital-letter rule
Go has no `public`/`private` keywords. Instead:
- A name that starts with a **capital letter** is **exported** (public): `NewServer`, `Storage`, `ErrNotFound`, `ChunkInfo.ID`.
- A **lowercase** name is visible only **inside its own package** (package-private): `handlePutChunk`, `writeError`, `Server.store`.

That's why the handlers are lowercase. Nobody outside `api` should call them directly; they reach the outside world only through `Handler()`. It's also why `Server`'s fields are lowercase: other packages *must* use `NewServer`.

### `internal/` folders
A package under `internal/` can only be imported by code rooted at the parent of `internal`. `worker/internal/storage` can be used by anything under `worker/`, but the future `controller/` can't import it. This lets the worker change its internals freely. Java has no direct equivalent; it's roughly what the Java 9 module system's non-exported packages give you.

### Why `protocol` lives in `pkg/` and not under `worker/internal/`
The controller will need the same `ChunkInfo` and error codes to talk to workers. Because `internal/` would block that, shared wire types go in `pkg/protocol`.

### Type aliases
In `storage.go`:

```go
type ChunkInfo = protocol.ChunkInfo
```

The `=` makes `storage.ChunkInfo` **another name for the very same type**. It isn't a copy and needs no conversion. The storage interface can keep saying `ChunkInfo` (as the team agreed in the blueprint) while there's still only one definition. Without the `=`, `type X Y` would create a *new, separate* type.

### Package doc comments
A comment directly above `package api`, such as `// Package api implements…`, is the package's documentation. Tools like `go doc` and your editor show it.

## What changed since step 6
- **Moved:** step 6's code into the files shown above, with names prefixed by their package where needed (`storage.ErrNotFound`, `protocol.CodeChunkNotFound`, …).
- `main` now only wires things together, and it uses `srv.Handler()` directly.
- **Removed:** the debug corruption route (Steps 8 and 9 test corruption with a mock instead).
- `storageError` now also receives the request, so it can log the HTTP method.

## How this compares to the real worker
The layout is now the same as the real repo. Comparing file by file:

| File | Compared with the real one |
|---|---|
| `pkg/protocol/chunk.go` | Identical |
| `storage/storage.go`, `storage/memory.go` | Identical |
| `api/response.go` | Identical |
| `api/chunk_handlers.go` | Identical except `log.Printf` instead of `s.log` (structured logging comes in step 9) |
| `api/routes.go` | No logger, no middleware (step 9) |
| `cmd/worker/main.go` | Hard-coded settings, no graceful shutdown (step 9) |
| `config/` | Not yet (step 9) |

## Try it yourself
1. Rename `writeError` to `WriteError` in `response.go` and every place it's used. It now compiles as exported, but should it be? Change it back.
2. In `main.go`, try `srv.store`. What error do you get, and why?
3. Make `storage.go` import the `api` package (just add the import and use something from it). Read the "import cycle not allowed" error.

**Next, step 8:** automated tests for the handlers.
