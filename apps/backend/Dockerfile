# syntax=docker/dockerfile:1

# -----------------------------------------------------------------------------
# Build Stage
# -----------------------------------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/server ./cmd/server

# -----------------------------------------------------------------------------
# Final Minimal Runtime Stage
# -----------------------------------------------------------------------------
FROM alpine:3.20 AS runner

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S scribe && adduser -S scribe -G scribe

# Copy static assets and binary
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/static /app/static
COPY --from=builder /app/templates /app/templates

USER scribe:scribe

EXPOSE 8080

ENTRYPOINT ["/app/server"]
