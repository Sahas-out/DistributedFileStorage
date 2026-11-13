# Step 10: package step 9's worker as a Docker image.
# Same as the real worker/Dockerfile, except it builds the copy in learn/.
#
# Build context is the REPOSITORY ROOT (where go.mod is):
#   docker build -f learn/step10-docker/Dockerfile -t learn-worker .

# ---------- Stage 1: build ----------
# A full Go toolchain image, used only to compile.
FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy go.mod first and download dependencies in their own layer, so Docker
# can reuse this layer when only source code changes.
COPY go.mod ./
RUN go mod download

COPY learn/step09-production/ ./learn/step09-production/

# CGO_ENABLED=0 -> a fully static binary that needs no C libraries.
# -trimpath and -ldflags="-s -w" -> smaller binary without local paths.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/worker ./learn/step09-production/worker/cmd/worker

# ---------- Stage 2: run ----------
# A tiny base image. Only the compiled binary is copied in, no Go toolchain.
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
