# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS controller
ARG VERSION=1.0.2-dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p /tmp/go-build && \
    CGO_ENABLED=0 GOCACHE=/tmp/go-build GOTMPDIR=/tmp/go-build go build -p 1 -trimpath \
    -ldflags "-s -w -X github.com/benchristian88/atlas-dns/internal/version.Version=${VERSION} -X github.com/benchristian88/atlas-dns/internal/version.Commit=${COMMIT} -X github.com/benchristian88/atlas-dns/internal/version.BuiltAt=${BUILT_AT}" \
    -o /out/atlas-dns ./cmd/controller
RUN CGO_ENABLED=0 GOCACHE=/tmp/go-build GOTMPDIR=/tmp/go-build go build -p 1 -trimpath \
    -ldflags "-s -w -X github.com/benchristian88/atlas-dns/internal/version.Version=${VERSION} -X github.com/benchristian88/atlas-dns/internal/version.Commit=${COMMIT} -X github.com/benchristian88/atlas-dns/internal/version.BuiltAt=${BUILT_AT}" \
    -o /out/atlas-dns-backup ./cmd/atlas-dns-backup

FROM postgres:17-bookworm
ARG VERSION=1.0.2-dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
LABEL org.opencontainers.image.title="Atlas DNS Controller" \
      org.opencontainers.image.description="Management plane for multiple AdGuard Home nodes" \
      org.opencontainers.image.source="https://github.com/benchristian88/atlas-dns" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILT_AT}"
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 atlas-dns \
    && useradd --system --uid 10001 --gid atlas-dns --home-dir /var/lib/atlas-dns atlas-dns \
    && install -d -o 10001 -g 10001 -m 0700 /var/lib/atlas-dns/tmp
COPY --from=controller /out/atlas-dns /usr/local/bin/atlas-dns
COPY --from=controller /out/atlas-dns-backup /usr/local/bin/atlas-dns-backup
COPY --from=web /src/web/dist /usr/local/share/atlas-dns/web
COPY LICENSE /usr/local/share/atlas-dns/LICENSE
USER 10001:10001
EXPOSE 8080
ENV APP_ENV=production HTTP_ADDR=:8080 WEB_DIST_DIR=/usr/local/share/atlas-dns/web AUTO_MIGRATE=true TMPDIR=/var/lib/atlas-dns/tmp
ENTRYPOINT ["/usr/local/bin/atlas-dns"]
