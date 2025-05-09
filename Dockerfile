# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api-watchtower ./cmd/api

# Final stage
FROM alpine:3.18

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/api-watchtower .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./api-watchtower"]
