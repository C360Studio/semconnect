# syntax=docker/dockerfile:1.6

FROM golang:1.26.3-bookworm@sha256:386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b AS builder

WORKDIR /src
COPY deploy/probe/main.go ./main.go
RUN CGO_ENABLED=0 GO111MODULE=off go build -trimpath -ldflags="-s -w" -o /out/canonical-smoke ./main.go

FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b

COPY --from=builder /out/canonical-smoke /usr/local/bin/canonical-smoke
COPY deploy/canonical-system.v1.json /fixtures/canonical-system.v1.json

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/canonical-smoke"]
