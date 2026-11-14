# build stage: runs on the builder's own platform and cross-compiles, so the
# multi-platform release needs no emulation.
# The Go version must equal go.mod's toolchain line (CI checks), so the image
# ships the Go that govulncheck scanned. GOTOOLCHAIN=local stops a silent
# toolchain download if go.mod ever asks for a newer Go than this image has.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
# modules are platform-independent, so this layer is shared by all six targets;
# the ARGs below must stay after it, since declared build args key every later RUN
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# the release workflow passes the tag's version; the build context has no
# git history to read it from
ARG VERSION=0.0.0-src
ARG TARGETOS TARGETARCH TARGETVARIANT
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build \
  -trimpath \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o /tmp/bin
# run stage
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY --from=build /tmp/bin /app/bin
ENTRYPOINT ["/app/bin"]
