# Build stage
FROM golang:1.25.7-alpine AS builder

ARG VERSION

WORKDIR /app

# Install git for VCS info embedding
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (including .git for VCS info)
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=${VERSION}" -o /pastebin ./cmd/pastebin

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
