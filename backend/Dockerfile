# Development image. docker-compose.yaml bind-mounts the source over /app; this image
# only provides the toolchain and a warm module cache.
FROM golang:1.27

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go tool goose --version

# Build then exec, so the server is PID 1 and stops promptly on SIGTERM.
CMD ["sh", "-c", "go build -o /tmp/api ./cmd/api && exec /tmp/api"]
