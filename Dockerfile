# These are automatically provided by BuildKit but must be declared before use
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS builder

WORKDIR /app

# Install build dependencies in a separate layer for better caching
RUN apk add --no-cache git

# Copy go mod and sum files first (better layer caching)
COPY go.mod go.sum ./

# Download dependencies in a separate cached layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Copy source code
COPY . .

# Cross-compile the application with enhanced caching
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -trimpath -o trino-mcp ./cmd/

# Use a small image for the final container (explicit target platform)
FROM --platform=$TARGETPLATFORM alpine:3.22.2
RUN apk update && apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/trino-mcp .

# Default environment variables
ENV TRINO_HOST="trino" \
    TRINO_PORT="8080" \
    TRINO_USER="trino" \
    TRINO_CATALOG="memory" \
    TRINO_SCHEMA="default" \
    MCP_TRANSPORT="http" \
    MCP_HOST="0.0.0.0" \
    MCP_PORT="8080"

# Expose the port
EXPOSE ${MCP_PORT}

# Run the application
CMD ["./trino-mcp"] 
