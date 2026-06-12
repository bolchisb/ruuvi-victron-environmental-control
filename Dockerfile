# syntax=docker/dockerfile:1
#
# Cross-compiles a static ARM binary for GX devices. The default target is
# ARMv7 (Cerbo GX MK1/MK2, Cortex-A7); pass GOARCH=arm64 for aarch64 devices.
# CGO_ENABLED=0 => fully static, no libc dependency. godbus is pure Go, so the
# cross-compile runs inside the amd64 image with no QEMU.
# Build never happens on the host — only here.

FROM golang:1.22-alpine AS builder
WORKDIR /src

# -mod=mod lets the build resolve/pin deps (writes go.sum) on first build.
ENV GOFLAGS=-mod=mod

COPY . .

ARG VERSION=dev
ARG GOOS=linux
ARG GOARCH=arm
ARG GOARM=7
RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOARM=${GOARM} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/ruuvi-control ./cmd/ruuvi-control

# Export-only stage: `docker build --target artifact --output type=local,dest=dist`
# drops the bare binary into ./dist on the host.
FROM scratch AS artifact
COPY --from=builder /out/ruuvi-control /ruuvi-control
