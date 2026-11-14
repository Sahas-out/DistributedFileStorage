# Step 5: Validation and errors

**Goal:** never trust the client. This step checks chunk IDs, caps the body size at 1 MiB, lets the client send the checksum it expects, and makes every error a JSON object with a machine-readable code. After this step the HTTP behaviour **matches the real worker**, apart from the debug route and the `storage_error` responses, which need a storage layer that can fail (step 6). Steps 6–9 mostly restructure the code.

## Run it

```powershell
go run ./learn/step05-validation-and-errors
```

```powershell
$W = "http://localhost:8095"
curl.exe -i -X PUT --data-binary "hi" "$W/internal/chunks/bad.id"          # 400 invalid_chunk_id
curl.exe -i -X PUT --data-binary "hi" "$W/internal/chunks/c1" -H "X-Chunk-Checksum: 1234"   # 400 invalid_request
curl.exe -i -X PUT --data-binary "hi" "$W/internal/chunks/c1" -H "X-Chunk-Checksum: $('0'*64)" # 400 checksum_mismatch
curl.exe -i "$W/internal/chunks/nothing"                                    # 404 chunk_not_found (JSON)

# 413: make a file of 1 MiB + 1 byte and upload it
[IO.File]::WriteAllBytes("$PWD\big.bin", (New-Object byte[] 1048577))
curl.exe -i -X PUT --data-binary "@big.bin" "$W/internal/chunks/big"        # 413 chunk_too_large
```

Every error now looks like this:

```json
{"error":{"code":"chunk_not_found","message":"chunk nothing not found"}}
```

## New Go concepts

### `regexp`
```go
var chunkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
```
The pattern is compiled once, when the program starts. `Must…` functions **panic** (crash) on failure instead of returning an error. That's acceptable only for constants you know are valid, because a typo crashes the program at startup instead of failing silently later.

**Why this exact pattern:** an ID like `../../etc/passwd` could escape the storage folder once chunks are files on disk. Allowing only `[A-Za-z0-9_-]` makes `/`, `\` and `.` impossible, so that attack can't happen. This is validation by **allow-list** rather than by trying to block bad characters.

### Functions returning `(value, bool)`
```go
id, ok := chunkID(w, r)
if !ok { return }
```
A common Go pattern: the helper has already written the 400 reply, and `ok` tells the caller to stop.

### `errors.As`
```go
var tooLarge *http.MaxBytesError
if errors.As(err, &tooLarge) { ... }
```
Errors are values, so to ask "is this a size-limit error?" you use `errors.As`, which checks whether `err` is of the given type and fills in `tooLarge` if so. It's the Go equivalent of `catch (MaxBytesException e)`. `&tooLarge` passes the **address** of the variable so `errors.As` can write to it; more on pointers in step 6.

### Nested structs
`ErrorResponse{Error: ErrorBody{Code: …, Message: …}}` builds the nested JSON `{"error":{"code","message"}}`.

### `1 << 20`
A bit shift: 1 × 2²⁰ = 1,048,576 = 1 MiB.

## Design decisions in this step (and why)
| Decision | Why |
|---|---|
| **Error codes** separate from messages | The controller will branch on `chunk_corrupted` to fetch the other replica. Messages are for humans and may change; codes must not |
| **`writeError` is the only way to send an error** | Same idea as `@ControllerAdvice` in Spring: one consistent format everywhere |
| **Optional `X-Chunk-Checksum` on PUT** | The controller knows the checksum before sending, so damage in transit is caught and the bad data is never stored |
| **Read the body *before* validating** | If the server replies while ~1 MiB of request body is still unread, it closes the connection, and the TCP reset can **destroy the reply before the client reads it**. We saw this in testing: curl hung through Docker. Reading first (at most 1 MiB) avoids it |
| **Size check uses `MaxBytesReader`** | It stops reading at the limit, so a huge upload can't use up the worker's memory |
| **GET also checks size and empty checksum** | A truncated file or missing checksum metadata is also corruption |
| **Constant-time digest comparison** | A security habit. It doesn't matter much here, but it's how hashes should be compared |

## What changed since step 4
- **New:** the ID regex and `chunkID` helper, `MaxBytesReader` with the 413 response, checksum header checking, the `ErrorResponse` types with error codes, and `writeError`.
- All `http.Error` calls became `writeError`.

## How this compares to the real worker
The HTTP behaviour now matches the real worker. Some names have different homes:

| Here | Real code |
|---|---|
| `chunkIDPattern` | `storage.ValidChunkID()` in [storage.go](../../worker/internal/storage/storage.go), shared with Person 1 |
| Error code constants, `ErrorResponse` | [pkg/protocol/chunk.go](../../pkg/protocol/chunk.go) |
| `writeError` | [api/response.go](../../worker/internal/api/response.go) |
| `handlePutChunk` | Same steps, in [api/chunk_handlers.go](../../worker/internal/api/chunk_handlers.go) |

## Try it yourself
1. Move the `chunkID` check above the body read, then PUT 1 MiB to `bad.id` through the Docker worker at `:8081`. Can you reproduce the hang?
2. Add a new code `CodeEmptyChunk` and reject empty PUT bodies with 400.
3. Why do the handlers `return` right after `writeError`? What would happen without that `return`?

**Next, step 6:** an interface so the storage can be swapped out, and a `Server` struct instead of global variables.
