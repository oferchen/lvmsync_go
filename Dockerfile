# syntax=docker/dockerfile:1.4
# Dockerfile
FROM alpine:latest

RUN addgroup -S appgroup && adduser -S appuser -G appgroup \
    && apk add --no-cache ca-certificates

WORKDIR /app

COPY --chown=appuser:appgroup lvmsync_go /usr/local/bin/lvmsync_go

RUN chmod 0755 /usr/local/bin/lvmsync_go

USER appuser

ENTRYPOINT ["/usr/local/bin/lvmsync_go"]
