# Enhanced version of your existing Dockerfile with security improvements

# Build stage
FROM golang:1.22-alpine AS builder

# Install git for go mod download
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better Docker layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o vantage-exporter main.go

# Final stage - minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS requests to Vantage API
RUN apk --no-cache add ca-certificates tzdata curl

# Create non-root user for security
RUN addgroup -S vantage && adduser -S vantage -G vantage

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/vantage-exporter .

# Change ownership to non-root user
RUN chown vantage:vantage vantage-exporter

# Switch to non-root user
USER vantage

# Expose the metrics port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/metrics || exit 1

# Default command
CMD ["./vantage-exporter"]