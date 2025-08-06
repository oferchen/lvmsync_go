# Dockerfile
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY lvmsync_go /usr/local/bin/lvmsync_go

ENTRYPOINT ["lvmsync_go"]
