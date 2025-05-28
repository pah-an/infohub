# Build stage
FROM golang:1.24-alpine AS builder

# Install necessary packages
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o infohub cmd/infohub/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 infohub && \
    adduser -D -s /bin/sh -u 1000 -G infohub infohub

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/infohub .

# Copy config files
COPY --from=builder /app/configs ./configs

# Create cache directory
RUN mkdir -p /app/cache && chown -R infohub:infohub /app

# Switch to non-root user
USER infohub

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/healthz || exit 1

# Run the application
CMD ["./infohub"]
