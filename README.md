[![CI](https://github.com/espebra/pastebin/actions/workflows/ci.yaml/badge.svg)](https://github.com/espebra/pastebin/actions/workflows/ci.yaml)
[![Release](https://github.com/espebra/pastebin/actions/workflows/release.yaml/badge.svg)](https://github.com/espebra/pastebin/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/espebra/stupid-simple-s3)](https://goreportcard.com/report/github.com/espebra/stupid-simple-s3)
[![Go Reference](https://pkg.go.dev/badge/github.com/espebra/stupid-simple-s3.svg)](https://pkg.go.dev/github.com/espebra/stupid-simple-s3)

# Pastebin

A simple, self-hosted pastebin service with S3-compatible storage backend.

## Features

- Store and share text snippets with unique URLs
- Configurable time-to-live (TTL) for pastes
- Automatic cleanup of expired pastes
- S3 as the storage backend (AWS S3, MinIO, etc.)
- Data integrity verification on retrieval
- Copy to clipboard functionality
- Raw paste view

## Configuration

Configuration is done via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PASTEBIN_S3_BUCKET` | S3 bucket name (required) | - |
| `PASTEBIN_S3_ENDPOINT` | S3 endpoint URL | `s3.amazonaws.com` |
| `PASTEBIN_S3_REGION` | S3 region | `us-east-1` |
| `PASTEBIN_S3_USE_SSL` | Use HTTPS for S3 | `true` |
| `PASTEBIN_S3_ACCESS_KEY` | S3 access key | - |
| `PASTEBIN_S3_SECRET_KEY` | S3 secret key | - |
| `PASTEBIN_HOST` | Listen host | `127.0.0.1` |
| `PASTEBIN_PORT` | Listen port | `8080` |
| `PASTEBIN_MAX_PASTE_SIZE` | Maximum paste size in bytes | `1048576` (1MB) |
| `PASTEBIN_DEFAULT_TTL` | Default paste TTL | `8760h` (1 year) |
| `PASTEBIN_CLEANUP_INTERVAL` | Interval between cleanup runs | `1h` |
| `PASTEBIN_LOG_FORMAT` | Log format (`text` or `json`) | `text` |
| `PASTEBIN_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `PASTEBIN_SECURE_COOKIES` | Set Secure flag on cookies (enable for HTTPS) | `false` |

## Getting started

### From container image

Container images are available from GitHub Container Registry:

```bash
docker pull ghcr.io/espebra/pastebin:latest
```

Available tags:
- `latest` - Latest release
- `vX.Y.Z` - Specific version (e.g., `v1.0.0`)
- `X.Y` - Minor version (e.g., `1.0`)
- `X` - Major version (e.g., `1`)

Run with:

```bash
docker run -p 8080:8080 \
  -e PASTEBIN_HOST=0.0.0.0 \
  -e PASTEBIN_S3_BUCKET=pastebin \
  -e PASTEBIN_S3_ENDPOINT=your-s3-endpoint:9000 \
  -e PASTEBIN_S3_ACCESS_KEY=your-access-key \
  -e PASTEBIN_S3_SECRET_KEY=your-secret-key \
  ghcr.io/espebra/pastebin:latest
```

### With Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: "3"
services:
  s3:
    image: ghcr.io/espebra/stupid-simple-s3:latest
    ports:
      - "5553:5553"
    environment:
      - STUPID_RW_ACCESS_KEY=accesskey
      - STUPID_RW_SECRET_KEY=secretkey
    volumes:
      - s3-data:/var/lib/stupid-simple-s3/
    restart: unless-stopped
  pastebin:
    image: ghcr.io/espebra/pastebin:latest
    ports:
      - "8080:8080"
    environment:
      - PASTEBIN_HOST=0.0.0.0
      - PASTEBIN_S3_BUCKET=pastebin
      - PASTEBIN_S3_ENDPOINT=s3:5553
      - PASTEBIN_S3_USE_SSL=false
      - PASTEBIN_S3_ACCESS_KEY=accesskey
      - PASTEBIN_S3_SECRET_KEY=secretkey
    depends_on:
      - s3
volumes:
  s3-data:
```

Then run:

```bash
docker-compose up
```

The pastebin service will be available at http://localhost:8080.

### From package

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

### From binary

Download the binary for your platform from the [releases page](https://github.com/espebra/pastebin/releases).

### From source code

```bash
# Build
go build -o pastebin ./cmd/pastebin

# Run (with MinIO example)
export PASTEBIN_S3_BUCKET=pastebin
export PASTEBIN_S3_ENDPOINT=localhost:9000
export PASTEBIN_S3_USE_SSL=false
export PASTEBIN_S3_ACCESS_KEY=minioadmin
export PASTEBIN_S3_SECRET_KEY=minioadmin

./pastebin
```

## Check current version

```bash
./pastebin --version
# pastebin v1.0.0 (commit: abc1234)
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
make test
make fuzz
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
