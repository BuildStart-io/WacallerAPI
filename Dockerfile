# Multi-stage build for WacallerAPI
FROM golang:alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build standalone binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o wacaller ./cmd/wacaller

# Production image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/wacaller /app/wacaller

# Create directory for SQLite database and persistent data
RUN mkdir -p /app/data

EXPOSE 8090

VOLUME ["/app/data"]

ENTRYPOINT ["/app/wacaller"]
CMD ["-addr", ":8090", "-db", "/app/data/wacaller.db"]
