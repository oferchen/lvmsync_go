# syntax=docker/dockerfile:1.4
# Dockerfile

# 1) Build
FROM golang:1.24-alpine AS build
ARG BIN
ARG BUILD_PATH=.
# To build the gRPC daemon instead of the CLI, set:
#   --build-arg BIN=lvmsync_grpcd --build-arg BUILD_PATH=./cmd/grpcd
ARG GIT_SHA=unknown

RUN apk add --no-cache \
      build-base \
      linux-headers \
      pkgconf \
      lvm2-dev \
      libaio-dev

ENV CGO_ENABLED=1 \
    GO111MODULE=on

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X lvmsync_go/config.BuildVersion=${GIT_SHA}" -o /out/${BIN} ${BUILD_PATH} && \

# 2) Strip
# checkov:skip=CKV_DOCKER_7: we intentionally track Alpine latest for this stage
FROM alpine:latest AS strip
ARG BIN
RUN apk add --no-cache binutils
COPY --from=build /out/${BIN} /work/${BIN}
RUN strip /work/${BIN} /work/${BIN}

# 3) Runtime
# checkov:skip=CKV_DOCKER_7: we intentionally track Alpine latest for runtime
FROM alpine:latest
ARG BIN

RUN apk add --no-cache \
      ca-certificates \
      device-mapper-libs \
      lvm2-libs \
      libaio \
  && addgroup -S app \
  && adduser -S -G app -H -s /sbin/nologin app

COPY --from=strip /work/${BIN} /usr/local/bin/${BIN}

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=2 \
  CMD [ "/usr/local/bin/${BIN}", "--healthcheck" ]

USER app:app
WORKDIR /home/app
ENTRYPOINT ["/usr/local/bin/${BIN}"]
