# Step 2: Chunks in a map

**Goal:** make the server store data. A client can `PUT` bytes under a chunk ID, `GET` them back, and `DELETE` them. Everything is kept in a map in memory.

## Run it

```powershell
go run ./learn/step02-chunks-in-a-map
```

```powershell
curl.exe -i -X PUT --data-binary "hello chunk" http://localhost:8092/internal/chunks/c1   # 201
curl.exe -i http://localhost:8092/internal/chunks/c1                                     # 200 hello chunk
curl.exe -i http://localhost:8092/internal/chunks/missing                                # 404
curl.exe -i -X DELETE http://localhost:8092/internal/chunks/c1                           # 204
curl.exe -i http://localhost:8092/internal/chunks/c1                                     # 404 now
```

To send a real file, use `--data-binary "@path\to\file.jpg"`. The `@` tells curl to read the body from that file.

## New Go concepts

### Package-level variables
```go
var chunks = map[string][]byte{}
```
A `var` outside any function is visible to every function in the package. Java has no exact equivalent; the closest is a `static` field on a class. `map[string][]byte` is a map from string to byte slice, and `{}` makes it empty and ready to use.

`var x = ...` declares a variable. Inside functions you'll mostly see the short form `x := ...`, which declares the variable and infers its type. `:=` declares a new variable, while `=` assigns to one that already exists.

### Named functions as handlers
`http.HandleFunc("GET /healthz", handleHealth)` passes the function itself, without calling it, much like a Java method reference `this::handleHealth`. Any function with the signature `func(http.ResponseWriter, *http.Request)` can be a handler.

### Path wildcards
In the pattern `"PUT /internal/chunks/{chunk_id}"`, `{chunk_id}` matches one path segment, and `r.PathValue("chunk_id")` reads it. This is the equivalent of Spring's `@PathVariable`. Patterns like this are built into Go since 1.22, which is why the project needs Go 1.22.

### Multiple return values and `if err != nil`
This is **the** most important Go idea:

```go
data, err := io.ReadAll(r.Body)
if err != nil {
    http.Error(w, "could not read request body", http.StatusBadRequest)
    return
}
```

Go has **no exceptions** for normal errors. A function that can fail returns its result *and* an `error`. You check the error right away, and if it isn't `nil`, you handle it and usually `return`.

| Java | Go |
|---|---|
| `try { data = readAll(body); } catch (IOException e) { ... }` | `data, err := io.ReadAll(body)` then `if err != nil { ... }` |

You'll write `if err != nil` hundreds of times. It's verbose, but you can always see exactly where errors are handled.

### The "comma ok" idiom
```go
data, ok := chunks[id]
```
A map lookup can return a second boolean that says whether the key was there. Without it, a missing key just gives you the **zero value**, which is `nil` for a slice. It's like `map.containsKey(id)` and `map.get(id)` in a single step.

### Zero values
Every Go type has a zero value: `0` for numbers, `""` for strings, `false` for bools, `nil` for slices, maps and pointers. A variable is never "uninitialised".

### Built-in functions
`delete(map, key)` removes a key. Go has a few such built-ins (`len`, `append`, `make`, `delete`, …), which you call without any package name.

### Status codes
`http.StatusCreated` (201), `http.StatusNotFound` (404) and `http.StatusNoContent` (204) are named constants, so the code reads clearly.

## What changed since step 1
- **New:** the map `chunks` and the three chunk handlers.
- The handlers are now named functions instead of inline lambdas.
- **New:** `http.Error` for error replies. Errors are plain text for now; step 5 makes them JSON.

## ⚠️ The bug in this step
Go runs every request in its **own goroutine**, so two requests can run at the same moment. Two of them touching the same map at once is a **data race**: a Go map is not safe for concurrent use, like `HashMap` in Java. Go may even crash with `fatal error: concurrent map writes`.

The Go toolchain can detect this: `go run -race ./learn/step02-chunks-in-a-map`. The race detector needs cgo and a C compiler on Windows, so it's easiest to try in the Docker container. **Step 3 fixes this with a lock.**

## How this compares to the real worker
| Here | Real code |
|---|---|
| Global map | A `Memory` struct with a map and a lock, behind a `Storage` interface: [storage/memory.go](../../worker/internal/storage/memory.go) |
| Any chunk ID accepted | IDs validated with a regex |
| Plain-text errors | JSON errors with codes |
| PUT returns an empty 201 | PUT returns 201 with size and checksum as JSON |

## Try it yourself
1. PUT a chunk, then PUT different bytes to the same ID. Which version does GET return? (Overwriting is what the real worker does too.)
2. Make `handleDeleteChunk` return 404 when the chunk doesn't exist. Hint: use the comma-ok idiom first. Then undo it, because the worker uses an idempotent 204.
3. Add a `GET /internal/count` route that writes `len(chunks)`. You'll need `strconv.Itoa` to turn the int into a string.

**Next, step 3:** fix the race with a lock, and return JSON using structs.
