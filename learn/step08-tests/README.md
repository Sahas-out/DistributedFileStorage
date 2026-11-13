# Step 8: Tests

**Goal:** prove the handlers behave as specified, automatically. In the blueprint, Person 2's deliverable is "handler tests using a mock `Storage`", and this is that test file. The code is the same as step 7; the only new file is [`worker/internal/api/chunk_handlers_test.go`](worker/internal/api/chunk_handlers_test.go).

## Run it

```powershell
go test ./learn/step08-tests/...            # summary
go test -v ./learn/step08-tests/...         # every test and subtest by name
go test -v -run TestPutChunk ./learn/step08-tests/...   # only tests whose name matches
```

If Windows blocks Go test binaries on your machine, run them in Docker instead:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.22 go test -v ./learn/step08-tests/...
```

The server also still runs: `go run ./learn/step08-tests/worker/cmd/worker` (port 8098).

## How Go tests work
| Java / C# (JUnit / xUnit) | Go |
|---|---|
| Test class in `src/test/...` | A `_test.go` file **in the same folder and package** as the code |
| `@Test void putReturns201()` | `func TestPutReturns201(t *testing.T)`: the name must start with `Test` |
| `assertEquals(201, status)` | `if rec.Code != 201 { t.Fatalf("status = %d, want 201", rec.Code) }` |
| A test framework dependency | Nothing to install: `testing` is in the standard library |
| `mvn test` / `dotnet test` | `go test ./...` (the `...` means "this folder and every folder below it") |

`go build` ignores `_test.go` files, so tests never end up in the binary.

Because the test is in **package `api`**, it can call unexported things such as `NewServer(...).Handler()` internals, `sha256Hex` and so on.

`t.Fatalf` fails the test and stops it. `t.Errorf` fails it but keeps going. Go has no assertion library by default, just `if` and a clear message in the form "got X, want Y".

## Key techniques in the test file

### 1. `httptest`: call handlers without a network
```go
req := httptest.NewRequest(http.MethodPut, "/internal/chunks/c1", body)
rec := httptest.NewRecorder()        // a fake ResponseWriter that records what's written
handler.ServeHTTP(rec, req)          // run the real router + handler
rec.Code, rec.Header(), rec.Body     // inspect the response
```
This is like Spring's `MockMvc`. No port and no server are needed, and it takes milliseconds. The helper `do(...)` in the file wraps these lines.

For one end-to-end test (`TestRoundTripWithMemoryStore`), `httptest.NewServer` starts a *real* server on a random port and the test uses a real HTTP client against it.

### 2. A hand-written mock
```go
type mockStorage struct {
    putFn  func(ctx context.Context, id string, src io.Reader) (storage.ChunkInfo, error)
    openFn func(...)
    ...
    calls []string
}
func (m *mockStorage) Put(ctx context.Context, id string, src io.Reader) (storage.ChunkInfo, error) {
    m.record("Put " + id)
    return m.putFn(ctx, id, src)
}
```
`mockStorage` has the four methods, so it **is** a `storage.Storage` (step 6's implicit interfaces at work). Each test fills in only the function it needs, which is exactly what `when(store.put(...)).thenReturn(...)` does in Mockito, without needing a library:

```go
m := &mockStorage{openFn: func(context.Context, string) (io.ReadCloser, storage.ChunkInfo, error) {
    return nil, storage.ChunkInfo{}, storage.ErrNotFound      // simulate a missing chunk
}}
```

This is how the tests simulate things that are hard to cause for real: disk errors, **corrupted bytes** (the replacement for step 6's debug route), missing checksums. The `calls` list lets tests check that storage was **never called** after a rejected request.

The struct fields are function types (`func(...) ...`), so functions are values in Go, just like lambdas stored in fields in Java.

### 3. Subtests: `t.Run`
```go
t.Run("missing chunk returns 404", func(t *testing.T) { ... })
```
Each `t.Run` is a named sub-test that shows up in `go test -v` output as `TestGetChunk/missing_chunk_returns_404`. It's similar to JUnit 5's `@Nested`/`@DisplayName`.

### 4. Table-driven tests
The most common Go testing style: a slice of cases and one loop.

```go
cases := []struct {            // an anonymous struct type, defined inline
    name     string
    err      error
    wantCode int
}{
    {"existing chunk", nil, 204},
    {"missing chunk is idempotent", storage.ErrNotFound, 204},
    {"storage failure", errors.New("boom"), 500},
}
for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) { ... })
}
```
Adding a case means adding one line. It's like JUnit's `@ParameterizedTest`. `invalidIDPaths` uses the same idea to try seven bad IDs against both PUT and GET.

### 5. Test helpers: `t.Helper()`
Helpers such as `expectError` call `t.Helper()`, so a failure is reported at the line *in the test* that called the helper, not inside the helper itself.

## What the tests cover
- **PUT:** 201 with the correct `ChunkInfo`; maximum-length and empty bodies; seven kinds of invalid ID; exactly 1 MiB accepted and 1 MiB + 1 rejected with 413; checksum header matching, mismatching and malformed; **rejected PUTs still read the whole body** (the step 5 lesson); a storage error gives 500 without leaking details; a wrapped `ErrInvalidChunkID` gives 400; inconsistent info from storage gives 500.
- **GET:** bytes and all four headers, and the reader is closed; HEAD; 404; corrupted, truncated and empty-checksum chunks give `chunk_corrupted`; `ErrMissingChecksum` and read errors give 500; invalid IDs.
- **DELETE:** 204 for both existing and missing chunks, including wrapped not-found errors; 500 on failure.
- **List:** an empty store gives `[]`, not `null`; the `{worker_id}` in the path is ignored; storage failure.
- **Routing and health:** 405, 404 and `/healthz`.
- **Round trip:** a real server backed by `storage.NewMemory()`, running PUT → GET → list → DELETE twice → GET returns 404.

## What changed since step 7
- **New:** `worker/internal/api/chunk_handlers_test.go`. No production code changed.

## How this compares to the real worker
The real [chunk_handlers_test.go](../../worker/internal/api/chunk_handlers_test.go) has one more test, `TestPanicRecovery`, which needs the middleware from step 9.

## Try it yourself
1. Break something on purpose: in `handleDeleteChunk`, remove `&& !errors.Is(err, storage.ErrNotFound)`. Run the tests and read which cases fail and why.
2. Add a case to `invalidIDPaths`, e.g. `{"tilde", "a~b"}`, and run `go test -v -run "TestPutChunk/invalid" ./learn/step08-tests/...`.
3. Write a new test, `TestHealthIncludesWorkerID`, that checks `/healthz` returns `"worker_id":"worker-test"`.

**Next, step 9:** logging, crash recovery, configuration and graceful shutdown. That step's code is the real worker.
