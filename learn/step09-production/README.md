# Step 9: Production-ready

**Goal:** add what a program needs to run unattended in a container: **structured logging**, **crash recovery**, **configuration from environment variables**, **server timeouts** and **graceful shutdown**.

**This step's code is the real worker.** Every `.go` file here is identical to the one under `pkg/` and `worker/` at the repo root; only the import paths differ, so that the copy can live inside `learn/`.

## Run it

```powershell
$env:PORT = "8099"; go run ./learn/step09-production/worker/cmd/worker
```
Each log line is now JSON:

```json
{"time":"...","level":"INFO","msg":"worker listening","worker_id":"DESKTOP-ABC","addr":":8099","data_dir":"/data","max_chunk_bytes":1048576}
{"time":"...","level":"INFO","msg":"request","worker_id":"DESKTOP-ABC","method":"PUT","path":"/internal/chunks/c1","status":201,"bytes":95,"duration_ms":0}
```

Press **Ctrl+C** and you'll see `shutting down` followed by `shutdown complete`.

To run the tests (the full real suite, including `TestPanicRecovery`):

```powershell
go test -v ./learn/step09-production/...
```

## What's new, file by file

### [`worker/internal/config/config.go`](worker/internal/config/config.go): settings from the environment
```go
cfg.DataDir = envOr("DATA_DIR", "/data")
if v := os.Getenv("PORT"); v != "" {
    p, err := strconv.Atoi(v)
    if err != nil || p < 1 || p > 65535 {
        return Config{}, fmt.Errorf("invalid PORT %q", v)
    }
    cfg.Port = p
}
```
- Containers are configured through **environment variables**: Docker Compose sets `WORKER_ID=worker2` and so on, so the same image runs as all four workers. This is the Spring `application.properties` + `@Value("${PORT:8080}")` idea, written by hand.
- `if v := os.Getenv("PORT"); v != "" {` is an **if with an init statement**. `v` exists only inside the `if`.
- `fmt.Errorf` builds an error message, and `%q` prints the value in quotes.
- Bad values are rejected at **startup**, with a clear error, instead of causing strange failures later.
- `WORKER_ID` defaults to the machine's hostname (`os.Hostname()`). Inside Docker, that's the container ID.

### [`worker/internal/api/routes.go`](worker/internal/api/routes.go): middleware
**Middleware** is a function that takes a handler and returns a new handler that does something before and/or after it. It plays the role of a Servlet `Filter` or ASP.NET middleware:

```go
func (s *Server) logRequests(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rec := &statusRecorder{ResponseWriter: w}
        next.ServeHTTP(rec, r)                         // run the real handler
        s.log.Info("request", "method", r.Method, "status", rec.status, ...)
    })
}
...
return s.recoverPanics(s.logRequests(mux))   // request → recover → log → router → handler
```

- **`http.HandlerFunc(func...)`** converts a plain function into an `http.Handler`. `HandlerFunc` is a function type with a `ServeHTTP` method, a neat Go trick.
- **Struct embedding:** a `ResponseWriter` doesn't tell you which status was sent, so `statusRecorder` wraps it:
  ```go
  type statusRecorder struct {
      http.ResponseWriter      // embedded: no field name
      status int
      bytes  int64
  }
  ```
  An embedded field's methods are **promoted**, so `statusRecorder` automatically has `Header()`, `Write()` and `WriteHeader()` from the inner writer. We override `WriteHeader` and `Write` to record the status and byte count, then call the inner one. This is Go's alternative to inheritance: **composition with automatic delegation**, like a Java decorator without the boilerplate.
- **`recoverPanics`:** a **panic** is Go's crash, used for bugs (nil pointer, index out of range) and never for normal errors. `recover()` inside a `defer`red function catches it, much like a `catch (Throwable t)` at the outermost level, so one buggy request gets a JSON 500 instead of a dropped connection, and the bug is logged. `http.ErrAbortHandler` is re-panicked because it's how Go deliberately aborts a response.
- **`slog`** (structured logging, standard library since Go 1.21): `s.log.Error("storage failure", "chunk_id", id, "error", err)` takes a message plus key/value pairs, and the JSON handler turns them into fields. Logs can then be searched with `jq` or a log system, e.g. "all requests for chunk X". Compare SLF4J's `log.info("...", kv("chunk_id", id))`.
- `NewServer` now takes a logger. Passing `nil` gives a logger that discards everything, which is what the tests do. The `discard` type is a tiny `io.Writer` that throws bytes away.

### [`worker/internal/api/chunk_handlers.go`](worker/internal/api/chunk_handlers.go)
The only change from step 8 is `log.Printf(...)` → `s.log.Error/Warn(...)` with key/value pairs.

### [`worker/cmd/worker/main.go`](worker/cmd/worker/main.go): a proper server lifecycle
```go
func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    if err := run(logger); err != nil { logger.Error(...); os.Exit(1) }
}
```
**The `run() error` pattern:** `main` can't return an error, so all the work goes into `run`, which returns one. `main` then has exactly one place that logs and exits. It also makes sure `defer`s inside `run` actually execute, which `os.Exit` would skip.

```go
srv := &http.Server{
    Addr:              fmt.Sprintf(":%d", cfg.Port),
    Handler:           api.NewServer(store, cfg.WorkerID, cfg.MaxChunkBytes, logger).Handler(),
    ReadHeaderTimeout: 5 * time.Second,
    ...
}
```
**Timeouts:** `http.ListenAndServe` has none, so a client that opens a connection and sends nothing would hold it open forever (the "Slowloris" attack). An explicit `http.Server` sets limits. `5 * time.Second` works because `time.Duration` is just a number of nanoseconds.

**Graceful shutdown** uses goroutines, channels and `select`:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

errCh := make(chan error, 1)                  // a channel: a typed pipe between goroutines
go func() {                                   // `go` starts a goroutine (a lightweight thread)
    errCh <- srv.ListenAndServe()             // send the result into the channel when it returns
}()

select {                                      // wait for whichever happens first:
case err := <-errCh:                          //   the server failed to start (e.g. port in use)
    ...
case <-ctx.Done():                            //   Ctrl+C, or `docker stop` sent SIGTERM
}

shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
srv.Shutdown(shutdownCtx)   // stop accepting new requests, let in-flight ones finish (max 10 s)
```

| Go | Java equivalent |
|---|---|
| `go f()` | `executor.submit(f)` / `new Thread(f).start()` |
| `chan error` | `BlockingQueue<Exception>` with one slot |
| `ch <- v` / `<-ch` | `queue.put(v)` / `queue.take()` |
| `select { case ...: case ...: }` | Waiting on several futures and taking the first that completes |
| `signal.NotifyContext(..., SIGTERM)` | `Runtime.getRuntime().addShutdownHook(...)` |

**Why it matters here:** `docker stop worker2`, which the project demo requires, sends SIGTERM and gives the container 10 seconds before killing it. Without graceful shutdown, a chunk upload in progress at that moment would be cut off halfway.

The `// TODO(person1)` line is where the filesystem store will replace `storage.NewMemory()`.

## What changed since step 8
- **New:** the `config` package, the `logRequests`/`recoverPanics` middleware with `statusRecorder`, `slog` everywhere, `NewServer(..., logger)`, the `run()` pattern, `http.Server` with timeouts, and graceful shutdown.
- **New test:** `TestPanicRecovery`. The test file is now identical to the real one.

## How this compares to the real worker
Here's how to check that it's the same code yourself, from Git Bash at the repo root:

```bash
for f in $(cd learn/step09-production && find . -name '*.go'); do
  diff -q <(sed 's#/learn/step09-production##' "learn/step09-production/$f") "$f" > /dev/null \
    && echo "same:      $f" || echo "DIFFERENT: $f"
done
```
Or simply open any file here next to its counterpart under `worker/`. They match line for line apart from the `import` paths.

## Try it yourself
1. Start it with `$env:PORT = "abc"` and read the startup error.
2. Start two copies on the same port. Which branch of the `select` fires in the second one?
3. Add a `panic("test")` at the top of `handleHealth`, call `/healthz`, and look at both the response and the log. Then remove it.
4. Add a `"user_agent", r.UserAgent()` pair to the request log line.

**Next, step 10:** package it in Docker, exactly as the real worker is deployed.
