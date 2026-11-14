# Step 6: A storage interface

**Goal:** the HTTP code shouldn't care *how* chunks are stored. In this project, Person 1 builds the real filesystem store while you build the HTTP layer, so the two parts need a contract between them: an **interface**. This step also replaces global variables with a `Server` struct and makes the handlers its **methods**.

The behaviour is the same as step 5. Only the structure changes.

## Run it

```powershell
go run ./learn/step06-storage-interface
```

The same curl commands as step 5 work, on port **8096**. The corruption aid still exists: `curl.exe -X POST http://localhost:8096/debug/corrupt/c1`.

## The big picture
```
             main()  ── knows the concrete type: *Memory
               │ builds
       ┌───────┴────────┐
   NewMemory()     NewServer(store, ...)
   *Memory  ──────►  Server{ store Storage }   ── only knows the INTERFACE
   (implements           │
    Storage)        handlers call s.store.Put / Open / Delete / List
```
To switch to Person 1's filesystem store later, change one line in `main` and leave every handler alone.

## New Go concepts

### Interfaces are implemented implicitly
```go
type Storage interface {
    Put(ctx context.Context, chunkID string, src io.Reader) (ChunkInfo, error)
    Open(ctx context.Context, chunkID string) (io.ReadCloser, ChunkInfo, error)
    Delete(ctx context.Context, chunkID string) error
    List(ctx context.Context) ([]ChunkInfo, error)
}
```
In Java you'd write `class Memory implements Storage`. **Go has no `implements`.** Any type with these four methods *is* a `Storage`, automatically. This is called **structural typing**.

That makes it easy to accidentally miss a method, so we add a compile-time check:

```go
var _ Storage = (*Memory)(nil)   // "a *Memory must be usable as a Storage"
```

If `Memory` lacks a method, the build fails on this line with a clear message.

### Methods and receivers
```go
func (m *Memory) Put(...) (ChunkInfo, error) { ... }
```
`(m *Memory)` is the **receiver**, which makes `Put` a method of `*Memory`. Inside the method, `m` plays the role of `this`, except you name it yourself (by convention a short letter). Methods are declared outside the struct, not inside a class body.

### Pointers: `*Memory` and `&Memory{...}`
- `&Memory{...}` creates a Memory and gives back its **address** (a pointer).
- `*Memory` is the type "pointer to Memory".

**Why pointers here?** Structs are *values* in Go: assigning or passing one **copies** it. A copied `Memory` would have a copied mutex, which breaks locking. A pointer lets everyone share one store, the way every Java object variable already does. The rule of thumb: **use pointer receivers (`*T`) for structs that have state or a mutex.** Go automatically dereferences, so you write `m.chunks`, never `(*m).chunks`.

### Constructors are just functions
Go has no constructors. By convention, `NewMemory()` and `NewServer(...)` build and return a ready-to-use value. This is exactly constructor injection: `NewServer` receives its dependencies as arguments.

### Sentinel errors, `errors.Is`, and wrapping
```go
var ErrNotFound = errors.New("chunk not found")
...
case errors.Is(err, ErrNotFound): // 404
```
A **sentinel error** is a fixed error value that callers compare against, playing the role of a specific exception class. `errors.Is` also sees through **wrapping**: if Person 1 returns `fmt.Errorf("open %s: %w", path, ErrNotFound)`, the `%w` keeps `ErrNotFound` inside, so `errors.Is` still matches. That's why the storage contract says "wrapping is fine".

`storageError` is the single place where storage errors become HTTP statuses, like a Spring `@ExceptionHandler`.

### `io.Reader` and `io.ReadCloser`
These are tiny standard interfaces: `Read(p []byte) (int, error)`, plus `Close() error` for `ReadCloser`. They're Go's `InputStream`.

`Put` takes an `io.Reader` and `Open` returns an `io.ReadCloser`, so a filesystem store can **stream** from disk instead of holding everything in memory. `bytes.NewReader(data)` turns a byte slice into a Reader, and `io.NopCloser` adds a do-nothing `Close`.

### `context.Context`
`r.Context()` is cancelled when the client disconnects or the server shuts down. Passing it to storage lets slow operations stop early. It's like a `CancellationToken` in C#. By convention it's always the first parameter, named `ctx`.

### `sync.RWMutex`
`RLock`/`RUnlock` let many readers in at once, while `Lock`/`Unlock` give one writer exclusive access. This is Java's `ReentrantReadWriteLock`.

### Composing routers
`main` wraps the Server's router inside another mux to add the debug route. Routers are just `http.Handler`s, and handlers can be nested.

## What changed since step 5
- **New:** the `Storage` interface, the sentinel errors, `ValidChunkID`, `Memory` with its methods, and the `Server` struct with `NewServer` and `Handler()`.
- The handlers became methods: `handleX(w, r)` → `(s *Server) handleX(w, r)`. They now call `s.store`.
- **New:** `storageError` mapping. PUT now double-checks what the store recorded, and GET reads through an `io.ReadCloser` with `defer rc.Close()`.
- Global variables are gone. Everything is created in `main` and passed in.

## How this compares to the real worker
| Here (one file) | Real code |
|---|---|
| `Storage`, sentinel errors, `ValidChunkID` | [storage/storage.go](../../worker/internal/storage/storage.go) |
| `Memory` | [storage/memory.go](../../worker/internal/storage/memory.go), identical apart from the debug method |
| `Server`, handlers, `storageError` | [api/routes.go](../../worker/internal/api/routes.go) and [api/chunk_handlers.go](../../worker/internal/api/chunk_handlers.go) |
| `log.Printf` | Structured `slog` logging (step 9) |

## Try it yourself
1. Delete the `List` method from `Memory` and build. Read the error the `var _ Storage = ...` line produces.
2. Change `func (m *Memory) Put` to `func (m Memory) Put` (no `*`) and build. Why does `var _ Storage = (*Memory)(nil)` still compile, and why is a value receiver a bug here anyway? Hint: the mutex gets copied.
3. Write a second store, `type Discard struct{}`, whose `Put` accepts anything and whose `Open` always returns `ErrNotFound`. Swap it into `main` and watch every GET return 404. That's the whole point of the interface.

**Next, step 7:** split this file into packages with the same layout as the real project.
