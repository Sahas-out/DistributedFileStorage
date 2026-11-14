# Step 3: JSON and locks

**Goal:** fix step 2's data race, reply with JSON instead of empty or plain responses, and add the endpoint that lists every stored chunk.

## Run it

```powershell
go run ./learn/step03-json-and-locks
```

```powershell
curl.exe -i -X PUT --data-binary "hello" http://localhost:8093/internal/chunks/a   # 201 {"chunk_id":"a","size":5}
curl.exe -X PUT --data-binary "hi" http://localhost:8093/internal/chunks/b
curl.exe http://localhost:8093/internal/workers/worker1/chunks
# {"worker_id":"worker1","count":2,"chunks":[{"chunk_id":"a","size":5},{"chunk_id":"b","size":2}]}
curl.exe http://localhost:8093/healthz                                             # {"status":"ok","worker_id":"worker1"}
```

## New Go concepts

### Structs
```go
type ChunkInfo struct {
    ID   string `json:"chunk_id"`
    Size int64  `json:"size"`
}
```
A struct is a group of named fields. It's like a Java `record` or a C# class with only properties, but with no constructor, inheritance or `new`. You create one with a **composite literal**:

```go
info := ChunkInfo{ID: id, Size: int64(len(data))}
```

Any field you leave out gets its zero value.

`int64(len(data))` is a **type conversion**. `len` returns an `int`, and Go never converts between number types automatically, not even `int` to `int64`. You have to write it out.

### Struct tags and JSON
The text in backticks after a field is a **tag**. `encoding/json` reads `json:"chunk_id"` to decide the JSON key, much like `@JsonProperty("chunk_id")` in Jackson or `[JsonPropertyName]` in C#.

**Capitalisation matters:** only fields that start with a capital letter (`ID`, `Size`) are visible outside the package, and `encoding/json` is another package. A field called `size` would silently disappear from the JSON. This is Go's public/private rule; step 7 covers it properly.

`json.NewEncoder(w).Encode(v)` writes `v` as JSON straight into the response.

### `any`
`writeJSON(w, status, v any)` accepts a value of any type, like `Object` in Java. One helper can then send a `ChunkInfo`, a `ChunkList` or a `map`.

### Slices: `make`, `append`, `range`
```go
list := make([]ChunkInfo, 0, len(chunks))   // length 0, room for len(chunks)
list = append(list, item)                   // like list.add(item); reassign the result!
for id, data := range chunks { ... }        // like for (var e : map.entrySet())
```
A **slice** (`[]T`) is Go's growable list, roughly `ArrayList<T>`. `append` may allocate a new backing array, so always write `list = append(list, …)`.

We use `make(..., 0, n)` instead of `var list []ChunkInfo` on purpose. A `nil` slice is encoded as JSON `null`, but an empty one becomes `[]`. Clients prefer `[]`.

### Map iteration order is random
Go deliberately randomises map iteration order, so we sort:

```go
sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
```

The function is a comparator that says whether item `i` should come before item `j`, like a Java `Comparator`.

### Goroutines and `sync.Mutex`
Each HTTP request runs in its own **goroutine**, a lightweight thread managed by Go. Go maps aren't safe for concurrent use, so every access is wrapped in a lock:

```go
mu.Lock()
chunks[id] = data
mu.Unlock()
```

`sync.Mutex` is like a Java `ReentrantLock`, or a `synchronized` block / C# `lock (obj) { }`. One difference: **a Go mutex is not reentrant**. Locking it twice from the same goroutine deadlocks.

### `defer`
```go
mu.Lock()
defer mu.Unlock()
```
`defer` schedules a call to run when the surrounding function returns, even if it returns early or panics. It's Go's version of `try { ... } finally { mu.unlock(); }`. A good habit is to put the `defer` right after acquiring the resource.

## What changed since step 2
- **New:** the `ChunkInfo` and `ChunkList` structs, the `writeJSON` helper, and the list route `GET /internal/workers/{worker_id}/chunks`.
- PUT now returns `201` with `ChunkInfo` JSON, and `/healthz` includes the worker ID.
- **Fixed:** a mutex now guards every map access.

## How this compares to the real worker
| Here | Real code |
|---|---|
| `ChunkInfo`, `ChunkList` in `main.go` | Shared wire types in [pkg/protocol/chunk.go](../../pkg/protocol/chunk.go) (step 7) |
| `writeJSON` | Same function in [api/response.go](../../worker/internal/api/response.go) |
| Global `sync.Mutex` | `sync.RWMutex` inside the `Memory` struct, with many readers or one writer (step 6) |
| List built from the map | `store.List()`, then the handler wraps it in `ChunkList` |

## Try it yourself
1. Rename the `Size` field to `size` (lowercase) and PUT a chunk. What happens to the JSON?
2. Remove `defer mu.Unlock()` from `collectChunks`, then list twice. The second request hangs forever. Why?
3. Change `make([]ChunkInfo, 0, len(chunks))` to `var list []ChunkInfo` and list an empty store. Compare the JSON.

**Next, step 4:** SHA-256 checksums so the worker can detect corrupted data.
