# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.0

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH
RUN mkdir -p /out/data/tmp && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/pamie ./cmd/pamie

FROM scratch

COPY --from=build /out/pamie /pamie
COPY --from=build --chown=65532:65532 /out/data /data

USER 65532:65532

ENV PAMIE_ADDR=0.0.0.0:8080
ENV PAMIE_DATA_DIR=/data
ENV TMPDIR=/data/tmp

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/pamie"]
