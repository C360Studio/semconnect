# syntax=docker/dockerfile:1.6
#
# Multi-stage build for cs-api-server. Builder produces a static-linked
# binary; runtime is distroless static-debian12 (no shell, no libc).
#
# Used by:
#   - conformance/compose.yml (Stage 6 harness)
#   - operator deployments (eventually)
#
# Build:    docker build -t cs-api-server .
# Run:      docker run --rm -p 8080:8080 cs-api-server

FROM golang:1.26.3-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# CGO_ENABLED=0 + -ldflags="-s -w" → static, stripped binary for distroless.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" \
        -o /out/cs-api-server ./cmd/cs-api-server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/cs-api-server /usr/local/bin/cs-api-server

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/cs-api-server"]
