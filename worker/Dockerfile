# Build context is the repository root (the Go module root):
#   docker build -f worker/Dockerfile -t dos-worker .

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
# COPY go.sum ./   # uncomment once the module has dependencies
RUN go mod download
COPY pkg/ ./pkg/
COPY worker/ ./worker/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./worker/cmd/worker

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app \
    && mkdir -p /data && chown app:app /data
COPY --from=build /out/worker /usr/local/bin/worker
USER app
ENV PORT=8080 DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/worker"]
