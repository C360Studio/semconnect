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

FROM golang:1.26.3-bookworm@sha256:386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b AS builder

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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b

COPY --from=builder /out/cs-api-server /usr/local/bin/cs-api-server

EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=12 \
    CMD ["/usr/local/bin/cs-api-server", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/cs-api-server"]
