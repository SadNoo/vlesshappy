# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=vendor \
    -trimpath -ldflags="-s -w -buildid=" -o /out/vlesshappy ./cmd/vlesshappy
RUN mkdir -p /out/data && chmod 0700 /out/data

FROM --platform=$TARGETPLATFORM caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648 AS caddy
RUN setcap -r /usr/bin/caddy

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/vlesshappy /vlesshappy
COPY --from=caddy /usr/bin/caddy /caddy
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
VOLUME ["/data"]
ENTRYPOINT ["/vlesshappy"]
CMD ["run", "-config", "/data/runtime/config.json"]
