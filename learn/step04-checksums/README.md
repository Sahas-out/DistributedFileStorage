# Step 4: Checksums

**Goal:** detect corrupted data. When a chunk is stored, the worker records its **SHA-256 checksum**. On every read it hashes the bytes again and refuses to return them if the two checksums differ. This is how the project meets requirement §8 (data integrity) on the worker side.

## Run it

```powershell
go run ./learn/step04-checksums
```

```powershell
curl.exe -X PUT --data-binary "hello chunk" http://localhost:8094/internal/chunks/c1
# {"chunk_id":"c1","size":11,"checksum":"<64 hex chars>"}

curl.exe -i http://localhost:8094/internal/chunks/c1     # 200 + X-Chunk-Checksum / X-Chunk-Size headers
curl.exe -I http://localhost:8094/internal/chunks/c1     # HEAD: same headers, no body

# Simulate disk corruption (learning aid only, not in the real worker):
curl.exe -i -X POST http://localhost:8094/debug/corrupt/c1    # 204
curl.exe -i http://localhost:8094/internal/chunks/c1          # 500 "failed integrity check"
```

To check the checksum yourself, run this in PowerShell: `"hello chunk" | Out-File -NoNewline -Encoding ascii x.txt; (Get-FileHash x.txt -Algorithm SHA256).Hash.ToLower()`. It should match the checksum the worker returned.

## What a checksum is
SHA-256 turns any amount of data into a 32-byte fingerprint, usually written as 64 hex characters. If even one bit of the data changes, the fingerprint changes completely. Storing the fingerprint at write time and comparing it at read time tells you whether the data is still exactly what was written.

## New Go concepts

### Arrays vs slices
```go
sum := sha256.Sum256(b)          // type [32]byte, an ARRAY (fixed size, part of the type)
hex.EncodeToString(sum[:])       // sum[:] makes a SLICE that views the array
```
An array's length is part of its type, so `[32]byte` and `[16]byte` are different types. You'll rarely use arrays directly; slices (`[]byte`) are what nearly every function accepts, and `[:]` converts an array into a slice.

### Slices are references to shared data
In `handleCorrupt`, `c, ok := chunks[id]` gives a **copy** of the struct, because structs are values in Go, like C# `struct`s. But `c.data` is a slice, and copying a slice copies only a pointer to the same underlying bytes. So `c.data[0] ^= 0xFF` changes the stored chunk. Keep this distinction in mind: structs are copied, while the data behind slices and maps is shared.

### Constant blocks
```go
const (
    HeaderChunkChecksum = "X-Chunk-Checksum"
    HeaderChunkSize     = "X-Chunk-Size"
)
```
Grouped constants, like `static final String` fields.

### `strconv`
HTTP headers are strings, so numbers have to be converted: `strconv.Itoa(int)` and `strconv.FormatInt(int64, 10)`, where 10 is the base. Go has no automatic `"" + number` conversion.

### The blank identifier `_`
In `for _, c := range chunks`, the key isn't needed, so it's assigned to `_`, which throws it away. Go won't compile if a variable is declared but never used, so `_` is how you say "I'm ignoring this on purpose".

### HEAD requests
A `GET` route also answers `HEAD` (since Go 1.22). HEAD asks for the headers without the body, and `if r.Method != http.MethodHead` skips writing the body.

## What changed since step 3
- `ChunkInfo` gained a `Checksum` field. The map now stores `storedChunk{data, info}` so the recorded checksum stays with the bytes.
- PUT computes the checksum. GET checks it, sends four headers, and handles HEAD.
- **New:** the `POST /debug/corrupt/{id}` learning aid.
- The list endpoint now returns the recorded info.

## How this compares to the real worker
| Here | Real code |
|---|---|
| `sha256Hex` | Same function in [api/chunk_handlers.go](../../worker/internal/api/chunk_handlers.go) |
| Integrity check in GET | Same idea, plus checking the size and treating an empty checksum as corrupt. The error code `chunk_corrupted` (step 5) lets the controller recognise the failure and fetch the other copy |
| Header constants in `main.go` | `protocol.HeaderChunkChecksum` and `protocol.HeaderChunkSize` |
| `/debug/corrupt` | Doesn't exist. To demo corruption with the real worker, you'd edit the chunk file on the Docker volume once Person 1's filesystem store is in place |

## Try it yourself
1. PUT the same bytes under two different IDs. Are the checksums the same? Why is that useful?
2. Corrupt a chunk, then PUT it again with fresh data. Does GET work again? That's essentially what replica repair will do.
3. Change `!=` to `==` in the integrity check. What breaks?

**Next, step 5:** proper validation, JSON errors, and size limits.
