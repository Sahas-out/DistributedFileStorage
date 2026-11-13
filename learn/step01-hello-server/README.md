# Step 1: Hello, server

**Goal:** the smallest Go program that answers HTTP requests. It has one endpoint, `GET /healthz`, which returns `{"status":"ok"}`. That's the same thing the real worker's health check returns.

## Run it

From the repo root, in one terminal:

```powershell
go run ./learn/step01-hello-server
```

You should see `listening on http://localhost:8091`. Stop it with **Ctrl+C**.

In a second terminal:

```powershell
curl.exe -i http://localhost:8091/healthz           # 200 {"status":"ok"}
curl.exe -i http://localhost:8091/nope              # 404: no such route
curl.exe -i -X POST http://localhost:8091/healthz   # 405: route exists, wrong method
```

`-i` makes curl print the status line and headers too. In PowerShell, write `curl.exe`, because plain `curl` is an alias for a different command.

## The code, piece by piece

### `package main`
Every Go file starts by naming its **package**. A package is a folder of `.go` files that are compiled together; it's roughly a Java package or a C# namespace, except that **one folder = one package**.

`main` is a reserved name. It means this package builds into an executable rather than a library. Packages with any other name are libraries; we'll create some in step 7.

### `import (...)`
This lists the packages the file uses. `log` and `net/http` are part of Go's **standard library**, so there's nothing to download. Go's built-in HTTP server is production-grade, which is why the real worker doesn't use a web framework the way Java would use Spring.

You call things from a package using its last path segment, e.g. `http.HandleFunc` or `log.Fatal`. Go refuses to compile if you import a package and don't use it, so imports never pile up unused.

### `func main()`
`func` declares a function. `main()` in package `main` is the program's entry point. The Java equivalent is:

```java
public class Main { public static void main(String[] args) { ... } }
```

Go has **no classes**. Functions can live at the top level of a file.

### `http.HandleFunc("GET /healthz", func(w, r) {...})`
This registers a **route**: when a request matches `GET /healthz`, Go calls the function.

- Putting the method inside the pattern (`"GET /healthz"`) is new in Go 1.22. Older code checked `r.Method` by hand.
- The second argument is an **anonymous function**, much like a Java lambda `(w, r) -> {...}` or a C# lambda.
- In the parameter list `w http.ResponseWriter`, the **name comes before the type**. Java writes `ResponseWriter w`; Go writes `w http.ResponseWriter`.
- The `*` in `r *http.Request` means `r` is a **pointer** to a Request. For now, read it as "a reference to the request object", which is what every Java object variable already is. We'll come back to pointers in step 6.

| Go | Spring (Java) | ASP.NET (C#) |
|---|---|---|
| `http.HandleFunc("GET /healthz", fn)` | `@GetMapping("/healthz")` | `app.MapGet("/healthz", ...)` |
| `w http.ResponseWriter` | `HttpServletResponse` | `HttpResponse` |
| `r *http.Request` | `HttpServletRequest` | `HttpRequest` |

### Writing the response
```go
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusOK)
w.Write([]byte(`{"status":"ok"}`))
```

The order matters: **set headers, then the status code, then the body**.
- Once `WriteHeader` has been called, headers can't be changed any more.
- If you never call `WriteHeader`, Go sends 200 on the first `Write`.
- `http.StatusOK` is just a named constant for `200`.
- `[]byte(...)` converts a string into a **byte slice**, Go's equivalent of `byte[]`, because `Write` sends raw bytes.
- The backticks `` `...` `` make a **raw string**: no escapes are processed, so the JSON quotes don't need backslashes.

(We're ignoring the error that `w.Write` returns. Step 2 explains errors.)

### `log.Fatal(http.ListenAndServe(":8091", nil))`
- `":8091"` means "listen on port 8091 on every network interface".
- `nil` (Go's `null`) means "use the default router". That's the one `http.HandleFunc` registered our route on.
- `ListenAndServe` **blocks**: it runs the server until something breaks. If it ever returns, it returns an error (for example, "address already in use"), and `log.Fatal` prints it and exits with code 1.

### One thing to know already
Go handles **every request in its own goroutine**, a very lightweight thread, so requests run in parallel. That doesn't matter yet because there's no shared data, but it will in step 3.

## How this compares to the real worker
| Here | Real code |
|---|---|
| One route on the default router | `http.NewServeMux()` with five routes, in [worker/internal/api/routes.go](../../worker/internal/api/routes.go) |
| JSON written as a string by hand | `writeJSON` using `encoding/json`, in [worker/internal/api/response.go](../../worker/internal/api/response.go) |
| `http.ListenAndServe` | A configured `http.Server` with timeouts and graceful shutdown, in [worker/cmd/worker/main.go](../../worker/cmd/worker/main.go) |

## Try it yourself
1. Add a `GET /hello` route that returns the plain text `hello` with `Content-Type: text/plain`.
2. Change the port to 9000 and confirm the old URL stops working.
3. Remove the `w.WriteHeader(...)` line. Is the status still 200? Why?
4. Add `"fmt"` to the imports without using it, then run the program. Read the compiler error.

**Next, step 2:** storing chunks. PUT, GET and DELETE on `/internal/chunks/{chunk_id}`, backed by a map.
