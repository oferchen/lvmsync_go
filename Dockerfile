# syntax=docker/dockerfile:1.4
# Dockerfile

FROM alpine:3.20  # ✅ CKV_DOCKER_7: pinned tag

# hadolint ignore=DL3018
RUN addgroup -S appgroup && adduser -S appuser -G appgroup \
    && apk add --no-cache ca-certificates

WORKDIR /app

COPY --chown=appuser:appgroup lvmsync_go /usr/local/bin/lvmsync_go
RUN chmod 0755 /usr/local/bin/lvmsync_go

USER appuser

ENTRYPOINT ["/usr/local/bin/lvmsync_go"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=2 \
  CMD [ "/usr/local/bin/lvmsync_go", "--healthcheck" ]
