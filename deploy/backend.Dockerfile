# syntax=docker/dockerfile:1.6

# SemConnect owns the composition root so its graph projection contracts are
# registered in the same exact SemStreams runtime as the HTTP gateway.
FROM golang:1.26.3-bookworm@sha256:386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b AS builder

ARG SEMSTREAMS_VERSION=v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download && \
    test "$(go list -m -f '{{.Version}}' github.com/c360studio/semstreams)" = "$SEMSTREAMS_VERSION"
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/cs-graph-backend ./cmd/cs-graph-backend

FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS production
ARG SEMSTREAMS_VERSION=v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
ARG SEMSTREAMS_COMMIT=8b99efe9c66a4faa4fa509f9f62cc6bad8392128
LABEL org.semconnect.semstreams.version=$SEMSTREAMS_VERSION \
      org.semconnect.semstreams.revision=$SEMSTREAMS_COMMIT
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S -g 1000 semstreams && \
    adduser -S -u 1000 -G semstreams -h /app semstreams
WORKDIR /app
COPY --from=builder --chown=semstreams:semstreams /out/cs-graph-backend /app/cs-graph-backend
USER semstreams
EXPOSE 8090
ENTRYPOINT ["/app/cs-graph-backend"]
