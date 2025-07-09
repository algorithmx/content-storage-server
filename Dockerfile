# Build stage
FROM golang:1.24-alpine AS builder

# Use system proxy settings (both uppercase and lowercase for compatibility)
ENV HTTP_PROXY=http://192.168.1.11:10078
ENV HTTPS_PROXY=http://192.168.1.11:10078
ENV NO_PROXY=FE80::/64,127.0.0.1,::1,FD00::/8,192.168.0.0/16,10.0.0.0/8,localhost
ENV http_proxy=http://192.168.1.11:10078
ENV https_proxy=http://192.168.1.11:10078
ENV no_proxy=FE80::/64,127.0.0.1,::1,FD00::/8,192.168.0.0/16,10.0.0.0/8,localhost

WORKDIR /app

# Install build dependencies
RUN echo "proxy $HTTP_PROXY" >> /etc/apk/repositories.conf && apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

# Runtime stage
FROM alpine:latest

# Use proxy settings for runtime stage
ENV HTTP_PROXY=http://192.168.1.11:10078
ENV HTTPS_PROXY=http://192.168.1.11:10078
ENV NO_PROXY=FE80::/64,127.0.0.1,::1,FD00::/8,192.168.0.0/16,10.0.0.0/8,localhost
ENV http_proxy=http://192.168.1.11:10078
ENV https_proxy=http://192.168.1.11:10078
ENV no_proxy=FE80::/64,127.0.0.1,::1,FD00::/8,192.168.0.0/16,10.0.0.0/8,localhost

# Install ca-certificates for HTTPS
RUN if [ -n "$HTTP_PROXY" ]; then \
        echo "proxy $HTTP_PROXY" >> /etc/apk/repositories.conf; \
    fi && \
    apk --no-cache add ca-certificates

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
