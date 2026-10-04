# syntax=docker/dockerfile:1

# Runs `rv export /project` on a project mounted at /project:
#
#   docker run --rm --network none -v "$PWD":/project ghcr.io/badimirzai/architon:<version>
#
# Build locally with the version that `rv version` should report:
#
#   docker build --build-arg VERSION=v0.15.0 -t architon:v0.15.0 .

ARG GO_VERSION=1.25
ARG DEBIAN_RELEASE=trixie

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-${DEBIAN_RELEASE} AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# VERSION is spliced into -ldflags, so it must be a single plain token.
RUN case "$VERSION" in \
      ''|*[!A-Za-z0-9._+-]*) echo "invalid VERSION: '$VERSION'" >&2; exit 1 ;; \
    esac \
 && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X github.com/badimirzai/architon-cli/internal/version.Version=$VERSION" \
      -o /out/rv ./cmd/rv

FROM debian:${DEBIAN_RELEASE}-slim

ARG DEBIAN_RELEASE
ARG VERSION=dev
LABEL org.opencontainers.image.title="architon" \
      org.opencontainers.image.description="Architon rv export for Architon Studio" \
      org.opencontainers.image.source="https://github.com/badimirzai/architon-cli" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="$VERSION"

# kicad-cli generates a netlist when the project only has a .kicad_sch.
# Symbol, footprint, and 3D libraries are not needed for netlist export.
# Backports carries the latest KiCad 9.0.x; the base release is an older 9.0.
RUN echo "deb http://deb.debian.org/debian ${DEBIAN_RELEASE}-backports main" \
      > /etc/apt/sources.list.d/backports.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends -t "${DEBIAN_RELEASE}-backports" kicad \
 && rm -rf /var/lib/apt/lists/* \
 && command -v kicad-cli

COPY --from=build /out/rv /usr/local/bin/rv

# kicad-cli writes its config under $HOME. `docker run --user <uid>:<gid>`
# keeps HOME from ENV, so the directory must be writable by any uid.
RUN groupadd --gid 1000 architon \
 && useradd --uid 1000 --gid 1000 --home-dir /home/architon --create-home \
      --shell /usr/sbin/nologin architon \
 && chmod 1777 /home/architon
ENV HOME=/home/architon

USER 1000:1000
WORKDIR /project

CMD ["rv", "export", "/project"]
