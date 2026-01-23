# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /pastebin ./cmd/pastebin

# Runtime stage
FROM gcr.io/distroless/static:nonroot

# Copy binary
COPY --from=builder /pastebin /pastebin

# Expose port
EXPOSE 8080

# Set default environment variables
ENV PASTEBIN_HOST=0.0.0.0
ENV PASTEBIN_PORT=8080

ENTRYPOINT ["/pastebin"]
