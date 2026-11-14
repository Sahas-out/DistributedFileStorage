# Step 10: Docker

**Goal:** package the worker the way the project requires it to be deployed (§2 and §9 of the project statement): as a Docker container with persistent storage, started through Docker Compose. There's no new Go code. This step builds step 9's code, which is the real worker.

## Run it (from the repo root)

```powershell
docker compose -f learn/step10-docker/docker-compose.yml up --build -d
docker ps                                   # STATUS shows "(healthy)" after ~10 s
docker logs -f learn-worker                 # JSON logs; Ctrl+C stops following, not the container

curl.exe -X PUT --data-binary "hello" http://localhost:8100/internal/chunks/c1
curl.exe http://localhost:8100/internal/workers/x/chunks

docker stop learn-worker                    # sends SIGTERM -> "shutting down", "shutdown complete"
docker start learn-worker
docker compose -f learn/step10-docker/docker-compose.yml down        # remove the container
docker compose -f learn/step10-docker/docker-compose.yml down -v     # ...and delete the volume
```

After `docker stop` / `docker start`, the chunk `c1` is **gone**, because the worker still uses the in-memory store. Once Person 1's filesystem store writes to `/data`, it will survive, because `/data` is a Docker volume. That's the last item in the Phase 1 definition of done.

## The Dockerfile, explained
It's a **multi-stage build**: two images, of which only the second is shipped.

| Line | What and why |
|---|---|
| `FROM golang:1.22-alpine AS build` | Stage 1 uses an image with the Go 1.22 toolchain, named `build` |
| `WORKDIR /src` | Like `cd /src`, creating it if needed |
| `COPY go.mod ./` then `RUN go mod download` | Dependencies are downloaded in their own **layer**. Docker caches layers, so when only `.go` files change, this step is reused. (We have no dependencies yet, so it's quick.) |
| `COPY learn/step09-production/ ...` | Copy the source in. The real Dockerfile copies `pkg/` and `worker/` |
| `RUN CGO_ENABLED=0 GOOS=linux go build ...` | Compile a **static Linux binary**. Go cross-compiles easily, so the same source builds for Windows or Linux. `CGO_ENABLED=0` means no C libraries are needed at runtime |
| `FROM alpine:3.20` | Stage 2 is a ~8 MB Linux image. The Go toolchain (hundreds of MB) is **not** included |
| `adduser ... app` / `USER app` | Don't run as root. If the worker were ever compromised, the attacker wouldn't be root in the container |
| `COPY --from=build /out/worker ...` | Take only the compiled binary from stage 1 |
| `ENV PORT=8080 DATA_DIR=/data` | Defaults that step 9's `config.Load()` reads |
| `VOLUME ["/data"]` | Marks `/data` as where persistent data lives |
| `EXPOSE 8080` | Documents the port; it doesn't publish it, Compose's `ports:` does that |
| `HEALTHCHECK ... wget .../healthz` | Docker calls `/healthz` every 10 s. After 3 failures, the container is marked `unhealthy` |
| `ENTRYPOINT [...]` | The command the container runs |

Compared with a Java image: there's no JRE. A Go binary contains everything, so the final image is tiny and starts in milliseconds.

## docker-compose.yml, explained
| Key | Meaning |
|---|---|
| `build.context: ../..` | Send the **repo root** to Docker as the build context, because the build needs `go.mod`. The path is relative to the compose file |
| `build.dockerfile` | Which Dockerfile to use, relative to the context |
| `container_name` | A fixed name, so `docker stop learn-worker` works. The project demo uses `docker stop worker2` |
| `environment` | Sets environment variables. This is how one image becomes `worker1` … `worker4` later |
| `ports: "8100:8080"` | `host:container`. Your PC's port 8100 forwards to the worker's 8080 |
| `volumes: learn-worker-data:/data` | A **named volume**: storage managed by Docker that survives container stop, start and removal. Only `down -v` deletes it |

In Phase 2, the real `docker-compose.yml` will list `controller`, `worker1`…`worker4`, each worker with its own `WORKER_ID` and its own volume.

## Journey complete: steps compared with the real code
| Concept | Introduced in | Real file |
|---|---|---|
| HTTP server, routes | 1–2 | [api/routes.go](../../worker/internal/api/routes.go) |
| JSON, structs, locks | 3 | [pkg/protocol/chunk.go](../../pkg/protocol/chunk.go), [storage/memory.go](../../worker/internal/storage/memory.go) |
| Checksums and integrity check | 4 | [api/chunk_handlers.go](../../worker/internal/api/chunk_handlers.go) |
| Validation, JSON errors, size limit | 5 | [api/chunk_handlers.go](../../worker/internal/api/chunk_handlers.go), [api/response.go](../../worker/internal/api/response.go) |
| Storage interface, Server struct | 6 | [storage/storage.go](../../worker/internal/storage/storage.go) |
| Packages | 7 | the whole `pkg/` + `worker/` layout |
| Tests | 8 | [api/chunk_handlers_test.go](../../worker/internal/api/chunk_handlers_test.go) |
| Logging, middleware, config, shutdown | 9 | [config/config.go](../../worker/internal/config/config.go), [cmd/worker/main.go](../../worker/cmd/worker/main.go) |
| Docker | 10 | [worker/Dockerfile](../../worker/Dockerfile), [docker-compose.yml](../../docker-compose.yml) |

The only differences between this step's files and the real ones:
- The Dockerfile `COPY`s `learn/step09-production/` instead of `pkg/` + `worker/`.
- Compose uses the names `learn-worker` and port 8100 instead of `worker1` and 8081.

You've now seen every line of the real worker get built up. The next code to read is the real thing, under [`worker/`](../../worker/).

## Try it yourself
1. Run `docker images learn-worker` and note the size. Then change the second `FROM` to `golang:1.22-alpine`, rebuild, and compare.
2. Set `MAX_CHUNK_BYTES: "10"` under `environment`, run `up -d` again, and PUT 11 bytes.
3. Run `docker exec learn-worker wget -qO- http://127.0.0.1:8080/healthz`. That's what the healthcheck runs.
4. Copy the service block into a second service, `learn-worker2`, with its own name, port `8101`, `WORKER_ID` and volume. You've just written part of Phase 2's compose file.
