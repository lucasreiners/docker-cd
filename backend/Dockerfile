# syntax=docker/dockerfile:1
FROM golang:1-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /docker-cd ./cmd/docker-cd

# -------------------------------------------------------------------
FROM docker:27-cli

# Install docker compose plugin
RUN apk add --no-cache docker-cli-compose

COPY --from=builder /docker-cd /usr/local/bin/docker-cd

ENV PORT=8080
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
  CMD wget -q --spider http://localhost:${PORT}/ || exit 1

ENTRYPOINT ["docker-cd"]
