# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o store ./cmd/store

# Runtime stage
FROM alpine:3.18

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/store .

# Create directories
RUN mkdir -p /app/templates /app/data/uploads

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["./store"]
