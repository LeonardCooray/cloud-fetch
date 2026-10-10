# build stage
FROM golang:alpine AS build
# the release workflow passes the tag's version; the build context has no
# git history to read it from
ARG VERSION=0.0.0-src
ADD . /src
WORKDIR /src
ENV CGO_ENABLED 0
RUN go build \
  -trimpath \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o /tmp/bin
# run stage
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY --from=build /tmp/bin /app/bin
ENTRYPOINT ["/app/bin"]