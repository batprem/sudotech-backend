# syntax=docker/dockerfile:1.7

# ---- build stage --------------------------------------------------------------
FROM golang:1.25-alpine AS builder

# go-sqlite3 needs CGO, which needs a C toolchain.
RUN apk add --no-cache build-base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1 \
    GOOS=linux

RUN go build -trimpath -ldflags="-s -w" -o /out/todo-api .

# ---- runtime stage ------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app \
 && mkdir -p /data && chown app:app /data

WORKDIR /data
COPY --from=builder /out/todo-api /usr/local/bin/todo-api

USER app
EXPOSE 8080

ENV GIN_MODE=release

# todos.db is created in the working directory; mount /data as a volume to persist.
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/todo-api"]
