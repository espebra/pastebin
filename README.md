[![CI](https://github.com/espebra/pastebin/actions/workflows/ci.yaml/badge.svg)](https://github.com/espebra/pastebin/actions/workflows/ci.yaml)
[![Release](https://github.com/espebra/pastebin/actions/workflows/release.yaml/badge.svg)](https://github.com/espebra/pastebin/actions/workflows/release.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/espebra/pastebin.svg)](https://pkg.go.dev/github.com/espebra/pastebin)

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

## Getting started

### Running with Docker

```bash
docker run -p 8080:8080 \
  -e PASTEBIN_HOST=0.0.0.0 \
  -e PASTEBIN_S3_BUCKET=pastebin \
  -e PASTEBIN_S3_ENDPOINT=your-s3-endpoint:9000 \
  -e PASTEBIN_S3_ACCESS_KEY=your-access-key \
  -e PASTEBIN_S3_SECRET_KEY=your-secret-key \
  ghcr.io/espebra/pastebin:latest
```

### Running with Docker Compose

```yaml
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

### Installing from packages

Download the latest `.deb` or `.rpm` package from the [releases page](https://github.com/espebra/pastebin/releases).

```bash
## 1. Install package
# Debian/Ubuntu
sudo dpkg -i pastebin_1.0.0_amd64.deb

# RHEL/Fedora
sudo rpm -i pastebin-1.0.0.x86_64.rpm

## 2. Configure
# Edit /etc/default/pastebin

## 3. Start the service
sudo systemctl enable --now pastebin
```

### Building from source

```bash
# Build on your architecture and platform
make build

# Or build for multiple architectures and platforms
make build-all
```

## Configuration

The service is configured using environment variables:

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
| `PASTEBIN_TRUSTED_PROXIES` | Comma separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` header is trusted when logging the client IP | (none) |
| `PASTEBIN_METRICS_PORT` | Port for the Prometheus `/metrics` listener. `0` disables it | `0` (disabled) |
| `PASTEBIN_METRICS_HOST` | Listen host for the metrics listener | value of `PASTEBIN_HOST` |

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
| GET | `/robots.txt` | Disallows all crawlers |
| GET | `/static/...` | Static assets (CSS, images) |

Prometheus metrics are served on a separate port, see [Metrics](#metrics).

## Health Check

The `/health` endpoint returns `200 OK` with body `OK` when the service is running. This can be used for load balancer health checks and monitoring.

```bash
curl http://localhost:8080/health
# OK
```

## Metrics

When `PASTEBIN_METRICS_PORT` is set, Prometheus metrics are served at
`/metrics` on a separate listener. The metrics are never served on the main
port, so they are not exposed to the public through the same address as the
pastes.

```bash
PASTEBIN_METRICS_PORT=9091 ./pastebin
curl http://localhost:9091/metrics
```

| Metric | Description |
|--------|-------------|
| `pastebin_build_info{version,commit}` | Build information, always `1` |
| `pastebin_http_requests_total{method,route,code}` | HTTP requests handled. `route` is the matched route pattern, such as `/{checksum}` |
| `pastebin_http_request_duration_seconds{method,route}` | HTTP request latency histogram |
| `pastebin_s3_requests_total{operation,code}` | Requests made to S3, including retries. `code` is `error` when no response was received |
| `pastebin_s3_request_duration_seconds{operation}` | S3 request latency histogram |
| `pastebin_pastes_created_total` | Pastes created |
| `pastebin_pastes_deleted_total{reason}` | Pastes deleted, by `manual` or `expired` |
| `pastebin_cleanup_runs_total{result}` | Cleanup runs, by `success` or `error` |
| `pastebin_cleanup_duration_seconds` | Cleanup run duration histogram |
| `pastebin_cleanup_last_success_timestamp_seconds` | When a cleanup run last completed without error |

Go runtime (`go_*`) and process (`process_*`) metrics are included as well.

### Grafana dashboard

A Grafana dashboard for these metrics is included in
[`grafana/pastebin.json`](grafana/pastebin.json). Import it in Grafana under
*Dashboards → New → Import* and select your Prometheus data source.

### Kubernetes

Prometheus scrapes pods directly on their pod IP, so the metrics port only
has to be reachable inside the cluster. To keep it away from the internet:

1. Listen on all interfaces inside the pod and name the port, but never
   route it from the Ingress, Gateway or a `LoadBalancer`/`NodePort` Service.
   The public Service should only contain the `http` port.
2. Let Prometheus find the pods with a `PodMonitor` (Prometheus Operator) or
   with scrape annotations, rather than adding the port to a public Service.
3. Add a `NetworkPolicy` so that only Prometheus can connect to the metrics
   port, even from inside the cluster.

```yaml
# Deployment (excerpt)
spec:
  template:
    metadata:
      labels:
        app: pastebin
    spec:
      containers:
        - name: pastebin
          env:
            - name: PASTEBIN_HOST
              value: "0.0.0.0"
            - name: PASTEBIN_METRICS_PORT
              value: "9091"
          ports:
            - name: http
              containerPort: 8080
            - name: metrics
              containerPort: 9091
---
# Public Service: the metrics port is deliberately left out
apiVersion: v1
kind: Service
metadata:
  name: pastebin
spec:
  selector:
    app: pastebin
  ports:
    - name: http
      port: 80
      targetPort: http
---
# Prometheus Operator: scrape the pods' metrics port directly
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: pastebin
spec:
  selector:
    matchLabels:
      app: pastebin
  podMetricsEndpoints:
    - port: metrics
---
# Only allow Prometheus to reach the metrics port
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: pastebin
spec:
  podSelector:
    matchLabels:
      app: pastebin
  policyTypes:
    - Ingress
  ingress:
    - ports:
        - port: http
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
          podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
      ports:
        - port: metrics
```

Adjust the namespace and labels in the `NetworkPolicy` to match your
Prometheus installation. Without the Prometheus Operator, add the
`prometheus.io/scrape: "true"`, `prometheus.io/port: "9091"` and
`prometheus.io/path: /metrics` pod annotations instead of the `PodMonitor`,
if your scrape configuration uses them.

## Logging

Logs are written to standard output, as `text` or `json` depending on
`PASTEBIN_LOG_FORMAT`.

### Request IDs

Every request is given a request ID, which is returned to the client in the
`X-Request-ID` response header and attached as `request_id` to every log line
written while handling the request. If the client (or a reverse proxy) sends
an `X-Request-ID` header, that ID is used instead, as long as it is at most 64
characters and consists only of letters, digits, `-`, `_` and `.`. Otherwise a
new random ID is generated.

### Access log

One log line with the message `request` is written for each request, with the
following fields:

| Field | Description |
|-------|-------------|
| `request_id` | The request ID, see above |
| `client_ip` | Client IP address. Taken from `X-Forwarded-For` only when the connection comes from one of `PASTEBIN_TRUSTED_PROXIES` |
| `method`, `path`, `proto` | The request line |
| `status` | Response status code |
| `bytes_in`, `bytes_out` | Request and response body sizes |
| `duration_ms` | Time spent handling the request |
| `user_agent`, `referer` | Request headers |
| `s3_request_ids` | Request IDs returned by S3 for the requests made while handling this request, if any. Useful when correlating with the S3 provider's logs |
| `panic` | The panic value, if the handler panicked (logged at `error` level) |

Requests to `/health` are logged at `debug` level to reduce noise.

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

### Releasing

Releases are automated via GitHub Actions. To create a new release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

The release workflow will automatically:

- Build binaries for Linux (amd64, arm64) and macOS (amd64, arm64)
- Build and push multi-arch Docker images to `ghcr.io`
- Create a GitHub release with binaries and checksums

## License

See LICENSE file.
