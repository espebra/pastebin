# Pastebin

A simple, self-hosted pastebin service with S3-compatible storage backend.

## Features

- Store and share text snippets with unique URLs
- Syntax highlighting with CodeMirror
- Configurable time-to-live (TTL) for pastes
- Automatic cleanup of expired pastes
- Content-addressable storage using SHA256 checksums
- S3-compatible storage backend (AWS S3, MinIO, etc.)
- Data integrity verification on retrieval
- Copy to clipboard functionality
- Raw paste view

## Security Features

- CSRF protection using double-submit cookie pattern
- Request body size limits to prevent memory exhaustion
- Security headers (X-Content-Type-Options, X-Frame-Options, X-XSS-Protection, Referrer-Policy)
- Input validation for all URL parameters
- Configurable secure cookies for HTTPS deployments

## Configuration

Configuration is done via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `S3_BUCKET` | S3 bucket name (required) | - |
| `S3_ENDPOINT` | S3 endpoint URL | `s3.amazonaws.com` |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_USE_SSL` | Use HTTPS for S3 | `true` |
| `AWS_ACCESS_KEY_ID` | AWS access key | - |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | - |
| `PASTEBIN_HOST` | Listen host | `127.0.0.1` |
| `PASTEBIN_PORT` | Listen port | `8080` |
| `MAX_PASTE_SIZE` | Maximum paste size in bytes | `1048576` (1MB) |
| `DEFAULT_TTL` | Default paste TTL | `8760h` (1 year) |
| `CLEANUP_INTERVAL` | Interval between cleanup runs | `1h` |
| `LOG_FORMAT` | Log format (`text` or `json`) | `text` |
| `LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `SECURE_COOKIES` | Set Secure flag on cookies (enable for HTTPS) | `false` |

## Running

### With Docker Compose

```bash
docker-compose up
```

### Manually

```bash
# Build
go build -o pastebin ./cmd/pastebin

# Run (with MinIO example)
export S3_BUCKET=pastebin
export S3_ENDPOINT=localhost:9000
export S3_USE_SSL=false
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin

./pastebin
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Home page with paste form |
| POST | `/` | Create a new paste |
| GET | `/{checksum}` | View a paste |
| GET | `/raw/{checksum}` | View raw paste content |
| POST | `/delete/{checksum}` | Delete a paste |
| GET | `/health` | Health check endpoint |

## Health Check

The `/health` endpoint returns `200 OK` with body `OK` when the service is running. This can be used for load balancer health checks and monitoring.

```bash
curl http://localhost:8080/health
# OK
```

## Storage

Pastes are stored in S3 with the following structure:

- `pastes/{checksum}` - The paste content
- `meta/{checksum}.json` - Metadata (created time, expiry, size)

The checksum is the SHA256 hash of the paste content, ensuring deduplication and data integrity verification on retrieval.

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o pastebin ./cmd/pastebin
```

## License

See LICENSE file.
