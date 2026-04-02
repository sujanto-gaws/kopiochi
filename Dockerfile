# Build stage
FROM golang:1.25.0-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/api

# Runtime stage
FROM alpine:latest

WORKDIR /root/

# Install ca-certificates for HTTPS and tzdata for timezone support
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/config/default.yaml ./config/

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main", "serve", "--config", "config/default.yaml"]
