# Multi-stage Docker build for production

# Stage 1: Build
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o todopro .

# Stage 2: Runtime
FROM alpine:latest

# Add CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S todopro && \
    adduser -u 1001 -S todopro -G todopro

WORKDIR /home/todopro

# Copy binary from builder
COPY --from=builder --chown=todopro:todopro /app/todopro .

# Switch to non-root user
USER todopro

# Expose port
EXPOSE 5000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:5000/health || exit 1

# Run the binary
CMD ["./todopro"]
