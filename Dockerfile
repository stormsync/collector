FROM golang:1.22-alpine3.19 AS builder
MAINTAINER org.opencontainers.image.authors="email@jasoncostello.com"
RUN apk update && apk add ca-certificates

ADD ./ /appdir/
RUN cd /appdir && \
    go mod tidy && \
    go mod vendor && \
    cd ./cmd/transform && \
    go build -a -tags netgo -ldflags="-w -s" -o ../../app

## Build scratch container and only copy over binary and certs
FROM scratch
COPY --from=builder /appdir/app /usr/local/bin/app

USER 1001
EXPOSE :8088
ENTRYPOINT [ "app" ]