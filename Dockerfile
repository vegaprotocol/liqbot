# FILE IS AUTOMATICALLY MANAGED BY github.com/vegaprotocol/terraform//github
ARG GO_VERSION=1.18.0
ARG ALPINE_VERSION=3.15
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder
RUN mkdir /build
WORKDIR /build
ADD . .
ENV DEBIAN_FRONTEND noninteractive
RUN apk update && apk add gcc && apk add g++
RUN apk add make git
RUN make build
FROM alpine:${ALPINE_VERSION}
# USER nonroot:nonroot
# COPY --chown=nonroot:nonroot bin/liqbot /liqbot
COPY --from=builder /build/bin/liqbot /liqbot