# syntax=docker/dockerfile:1.7
#
# evilginx-lab dashboard image.
#
# Multi-stage build: golang:1.25 builder produces the binary,
# alpine runtime stage runs it. CGO_ENABLED=1 because go-sqlite3
# is the chosen SQLite driver, so we install build-base + sqlite-dev
# in the builder and sqlite-libs at runtime.
#
# Build:
#   docker build -t auvalabs/phishlab-dashboard:dev .
#
# Run via compose (recommended) so the dashboard can read evilginx
# state via the shared volume:
#   docker compose up -d

ARG GO_VERSION=1.25.9
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

RUN apk add --no-cache build-base sqlite-dev

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ENV CGO_ENABLED=1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/evilginx-lab ./cmd/evilginx-lab

# ----------------------------------------------------------------

FROM alpine:${ALPINE_VERSION} AS runtime

RUN apk add --no-cache ca-certificates tzdata sqlite-libs && \
    addgroup -S evilginx-lab && adduser -S -G evilginx-lab evilginx-lab

WORKDIR /app
COPY --from=builder /out/evilginx-lab /usr/local/bin/evilginx-lab

VOLUME ["/data", "/etc/evilginx-lab"]

EXPOSE 9000

USER evilginx-lab

ENTRYPOINT ["/usr/local/bin/evilginx-lab"]
CMD ["deploy", "-c", "/etc/evilginx-lab/evilginx-lab.yaml"]
