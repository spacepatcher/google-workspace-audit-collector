FROM golang:1.19-alpine

RUN apk add --no-cache \
    alpine-sdk

ENV CGO_ENABLED=1

WORKDIR /app
COPY app/ .

RUN go build -tags musl -ldflags="-s -w" -o gws

ENTRYPOINT ["/app/gws"]