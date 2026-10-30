# syntax=docker/dockerfile:1
#
# Building this image requires libdave, which is compiled from source. That makes
# the build stage heavy and slow. The binary produced by `make build` on the host
# is usually the better deployment artifact; this Dockerfile exists for parity.
#
FROM golang:1.24-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        cmake \
        ninja-build \
        git \
        curl \
        zip \
        unzip \
        pkg-config \
        perl \
        python3 \
        libopus-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN ./scripts/install-libdave.sh
RUN CGO_ENABLED=1 go build -o /bridge ./cmd/bridge

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        libopus0 \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /bridge /usr/local/bin/bridge
ENTRYPOINT ["bridge"]
