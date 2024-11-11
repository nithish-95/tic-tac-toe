# Builder Stage
FROM golang:alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build the application
COPY . .
RUN go build -o /app/bin/tictac .

# Runtime Stage
FROM alpine:latest

# Add non-root user and set up the application directory
RUN addgroup -S appgroup && \
    adduser -S appuser -G appgroup && \
    mkdir -p /app/src

WORKDIR /app

# Copy the binary and HTML files from the builder
COPY --from=builder /app/bin/tictac /app/bin/tictac
COPY --from=builder /app/src /app/src

# Adjust permissions
RUN chown -R appuser:appgroup /app
USER appuser

# Set entrypoint and expose the port
ENTRYPOINT ["/app/bin/tictac"]
EXPOSE 3000
