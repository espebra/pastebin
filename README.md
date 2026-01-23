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

## Installation

### From Package (Recommended for Linux)

Download the latest `.deb` or `.rpm` package from the [releases page](https://github.com/espebra/pastebin/releases).

```bash
# Debian/Ubuntu
sudo dpkg -i pastebin_1.0.0_amd64.deb

# RHEL/CentOS/Fedora
sudo rpm -i pastebin-1.0.0.x86_64.rpm
```

After installation, configure `/etc/default/pastebin` and start the service:

```bash
sudo systemctl enable pastebin
sudo systemctl start pastebin
```

### From Binary

Download the binary for your platform from the [releases page](https://github.com/espebra/pastebin/releases).

## Running

### Version Information

```bash
./pastebin --version
# pastebin v1.0.0 (commit: abc1234)
```

### With Docker Compose

Create a `docker-compose.yml` file:

```yaml
services:
  pastebin:
    image: ghcr.io/espebra/pastebin:latest
    ports:
      - "8080:8080"
    environment:
      - PASTEBIN_HOST=0.0.0.0
      - PASTEBIN_PORT=8080
      - S3_BUCKET=pastebin
      - S3_ENDPOINT=s3:9000
      - S3_REGION=us-east-1
      - S3_USE_SSL=false
      - AWS_ACCESS_KEY_ID=accesskey
      - AWS_SECRET_ACCESS_KEY=secretkey
    depends_on:
      - s3

  s3:
    image: ghcr.io/espebra/s3s3:latest
    environment:
      - S3S3_ACCESS_KEY=accesskey
      - S3S3_SECRET_KEY=secretkey
      - S3S3_BUCKET=pastebin
    volumes:
      - s3-data:/data

volumes:
  s3-data:
```

Then run:

```bash
docker-compose up
```

The pastebin will be available at http://localhost:8080.

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

### Releasing

Releases are automated via GitHub Actions. To create a new release:

1. Ensure all changes are committed and pushed to the main branch
2. Create and push a version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

The release workflow will automatically:

- Build binaries for Linux (amd64, arm64) and macOS (amd64, arm64)
- Build and push multi-arch Docker images to `ghcr.io`
- Create a GitHub release with binaries and checksums

Docker images are tagged with:
- Full version (e.g., `v1.0.0`)
- Minor version (e.g., `1.0`)
- Major version (e.g., `1`)
- Git SHA

## License

See LICENSE file.
