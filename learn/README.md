# Learning path: from barebones Go to the worker

These steps start from a 20-line Go program and add one idea at a time until the code matches the real worker in [`worker/`](../worker/). Every step is a complete program you can run. The real code is not modified by anything in this folder.

## How to use it
1. Open the step's `README.md` and read it next to its `main.go`.
2. Run it from the repo root with `go run ./learn/<step-folder>`, and stop it with Ctrl+C.
3. Try the curl commands and the "Try it yourself" exercises.

Each step N listens on port **809N** (step 1 is on 8091), so the steps never clash with each other or with the real worker. Step 9 reads its port from `PORT`, and step 10 runs in Docker on 8100.

## If Windows blocks Go programs
If `go run` or `go test` fail with "An Application Control policy has blocked this file" or "Permission denied", Windows (usually Smart App Control) is blocking the unsigned programs Go builds. You can run any step inside Docker instead, from the repo root in PowerShell:

```powershell
# run a step's server (publish the step's port)
docker run --rm -p 8092:8092 -v "${PWD}:/src" -w /src golang:1.22 go run ./learn/step02-chunks-in-a-map
# run tests
docker run --rm -v "${PWD}:/src" -w /src golang:1.22 go test ./learn/...
```
Inside a container, a server must listen on all interfaces (`:8092`, which every step does) for `-p` to reach it. Steps 1–8 work as-is. For step 9 add `-e PORT=8099 -p 8099:8099`.

## Steps
| # | Step | Run with | Adds |
|---|---|---|---|
| 1 | [Hello, server](step01-hello-server/) | `go run ./learn/step01-hello-server` | A Go program, the HTTP server, one `/healthz` route |
| 2 | [Chunks in a map](step02-chunks-in-a-map/) | `go run ./learn/step02-chunks-in-a-map` | PUT/GET/DELETE, path values, `if err != nil`, maps |
| 3 | [JSON and locks](step03-json-and-locks/) | `go run ./learn/step03-json-and-locks` | Structs, JSON tags, slices, list endpoint, `sync.Mutex`, `defer` |
| 4 | [Checksums](step04-checksums/) | `go run ./learn/step04-checksums` | SHA-256, arrays vs slices, response headers, corruption detection |
| 5 | [Validation and errors](step05-validation-and-errors/) | `go run ./learn/step05-validation-and-errors` | ID regex, JSON error codes, 1 MiB limit, checksum header, `errors.As` |
| 6 | [Storage interface](step06-storage-interface/) | `go run ./learn/step06-storage-interface` | Interfaces, methods, pointers, constructors, sentinel errors, `context` |
| 7 | [Packages](step07-packages/) | `go run ./learn/step07-packages/worker/cmd/worker` | The real folder layout, exported names, `internal/`, type aliases |
| 8 | [Tests](step08-tests/) | `go test -v ./learn/step08-tests/...` | `testing`, `httptest`, a hand-written mock, table-driven tests |
| 9 | [Production-ready](step09-production/) | `$env:PORT="8099"; go run ./learn/step09-production/worker/cmd/worker` | Config, middleware, `slog`, panic recovery, timeouts, graceful shutdown. **This is the real worker code** |
| 10 | [Docker](step10-docker/) | `docker compose -f learn/step10-docker/docker-compose.yml up --build -d` | Multi-stage Dockerfile, compose, volumes, healthcheck |

Steps 1–6 are a single `main.go` each, so open the previous step's file next to the current one to see what changed. Each README also has a "What changed" section.

## Go for Java/C# developers: cheat-sheet
| Concept | Java / C# | Go |
|---|---|---|
| Program entry | `public static void main(String[] args)` in a class | `func main()` in `package main`, with no class |
| Variable | `int x = 5;` / `var x = 5;` | `x := 5` (declare and infer the type) or `var x int = 5` |
| Parameter types | `String name` | `name string` (the name comes first) |
| Function | `int add(int a, int b)` | `func add(a int, b int) int` |
| Multiple returns | Return an object or a tuple | `func f() (int, error)` and `n, err := f()` |
| Errors | `throw` / `try-catch` | Return an `error` value and check it with `if err != nil { ... }` |
| null | `null` | `nil` |
| Classes | `class Foo { ... }` | `type Foo struct { ... }` plus methods declared separately (step 6) |
| Interfaces | `class Foo implements Bar` | Implemented automatically: a type with the right methods satisfies the interface (step 6) |
| public / private | Keywords | **Capitalised names are public** (`Put`), lowercase names are package-private (`put`) |
| Arrays / lists | `byte[]`, `List<T>` | Slices: `[]byte`, `[]T` |
| Maps | `HashMap<K,V>` / `Dictionary<K,V>` | `map[K]V` |
| Threads | `Thread`, `Task` | Goroutines: `go f()`. The HTTP server starts one per request |
| `finally` / `using` | `try-finally`, `using`, try-with-resources | `defer x.Close()` runs when the function returns |
| Build and run | `mvn package`, `dotnet run` | `go build`, `go run .` |
| Format code | IDE settings | `gofmt` (one standard style; no arguments about formatting) |
