# Build stage
FROM golang:1.24-alpine AS builder

# Proxy settings can be passed at build time using --build-arg if needed
# ARG HTTP_PROXY
# ARG HTTPS_PROXY
# ARG NO_PROXY
# ENV http_proxy=${HTTP_PROXY} https_proxy=${HTTPS_PROXY} no_proxy=${NO_PROXY}

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS (proxy settings can be passed at build time)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/server .

# Copy static files and configuration
COPY --from=builder /app/static ./static
COPY --from=builder /app/.env ./.env

# Create directories for data, logs, and backups
RUN mkdir -p /app/data /app/logs /app/backups

# Run as non-root user with specific UID/GID to match host
RUN adduser -D -s /bin/sh -u 1000 appuser && \
    chown -R appuser:appuser /app

# Expose port
EXPOSE 8081

USER appuser

CMD ["./server"]
