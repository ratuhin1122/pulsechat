# PulseChat

PulseChat is a high-performance, secure real-time messaging platform built with Go, PostgreSQL, Redis, and modern web technologies. Engineered with a security-first mindset, PulseChat features an asynchronous, sandboxed file upload pipeline that isolates untrusted uploads, scans for malware via ClamAV, strips metadata and transcodes images using libvips, and broadcasts real-time updates through WebSockets.

---

## Upload Pipeline Architecture

PulseChat employs a defense-in-depth upload architecture. Uploaded files are immediately segregated into an isolated quarantine zone before validation, background malware scanning, and media transcoding occur. Clean assets are only served after passing all verification stages.

```mermaid
flowchart TD
    subgraph Client["Client Tier"]
        C["Web Client / Browser"]
    end

    subgraph API["PulseChat API Server"]
        U["Upload Handler<br/>(Auth, Rate Limit, Header Magic Check)"]
        QStore[("Quarantine Storage<br/>(/data/quarantine)")]
        DB[("PostgreSQL<br/>(Pending Upload Record)")]
        RedisQ[("Redis Asynq Queue")]
    end

    subgraph Processing["Background Worker Tier"]
        W["Attachment Worker"]
        Val["Header & Magic Byte Re-check"]
        AV["ClamAV Daemon<br/>(INSTREAM Virus Scan)"]
        Vips["libvips Processor<br/>(50MP Guard, WebP Transcode, Strip EXIF, Blurhash)"]
        Media["Media & Voice Probing<br/>(FFprobe Duration, Archive Inspection)"]
        CleanStore[("Clean Storage<br/>(/data/clean)")]
    end

    subgraph Realtime["Realtime Delivery"]
        PubSub[("Redis Pub/Sub")]
        WSHub["WebSocket Hub"]
    end

    %% Flow
    C -->|"1. POST /rooms/{id}/upload (Multipart)"| U
    U -->|"2. Write raw payload"| QStore
    U -->|"3. Record pending upload"| DB
    U -->|"4. Enqueue attachment ID"| RedisQ
    U -.->|"5. HTTP 202 Accepted"| C

    RedisQ -->|"6. Consume task"| W
    W -->|"7. Read quarantined file"| QStore
    W -->|"8. Validate format & bounds"| Val
    Val -->|"9. Stream bytes"| AV
    AV -->|"10. Scan OK"| Vips
    AV -->|"10. Scan OK (Media/Doc/Voice)"| Media
    Vips -->|"11. Save optimized WebP & thumbnail"| CleanStore
    Media -->|"11. Save sanitized media"| CleanStore
    W -->|"12. Delete original quarantine file"| QStore
    W -->|"13. Update status = ready"| DB

    W -->|"14. Publish completion event"| PubSub
    PubSub -->|"15. Receive event"| WSHub
    WSHub -->|"16. WS attachment_ready broadcast"| C
```

### Pipeline Lifecycle Stages

1. **Client Submission**: Client sends a multipart form request containing the file to `POST /rooms/{roomID}/upload` with a valid JWT bearer token.
2. **Ingestion & Quarantine**: The API validates room membership, enforces per-user and per-IP rate limits, validates MIME signatures via magic bytes, and streams the upload directly into `quarantine/` storage with a cryptographically generated UUID.
3. **Queue Dispatch**: A pending database record is created and an asynchronous job containing the attachment UUID is pushed to Redis via Asynq. The API returns `202 Accepted` to the client.
4. **Worker Processing**:
   - **Validation**: Verifies file size bounds, mime magic prefixes, and office/archive structures.
   - **ClamAV Anti-Malware**: Streams file content over TCP to the ClamAV daemon (`INSTREAM` command). Any detected threat triggers immediate rejection and purge.
   - **libvips Image Transcoding**: Images are decoded within safe memory bounds (50 MP limit), auto-rotated, stripped of EXIF and metadata, resized to display (1600px max) and thumbnail (320px max) dimensions in WebP format, and encoded with a Blurhash string for smooth placeholder rendering.
   - **Media & Audio Probing**: Voice notes are probed with FFprobe to enforce duration limits (<= 5 minutes). Optional deep archive inspection checks for zip bombs and traversal attacks.
5. **Clean Storage**: Sanitized outputs are committed to `clean/` storage. The original quarantine artifact is removed.
6. **WebSocket Notification**: The worker updates the database record to `ready`, publishes an event over Redis Pub/Sub, and connected WebSocket clients in the room receive real-time notification with attachment metadata.

---

## Features

- **Real-Time Messaging**: Low-latency, full-duplex communication over WebSockets with heartbeat keepalives, ping/pong health monitoring, and connection tracking.
- **Asynchronous File Processing Pipeline**: Zero-trust file handling isolating uploads in quarantine until passed by ClamAV and libvips.
- **Image Optimization & Blurhash**: WebP transcoding, thumbnail generation, EXIF stripping, and compact Blurhash strings for client placeholders.
- **Voice Messages**: Secure audio uploads with FFprobe format duration validation.
- **User Presence**: Ephemeral online/offline status tracking backed by Redis with heartbeats and automated timeouts.
- **Access Control & Authentication**: Secure JWT-based authentication with cryptographically random secrets, refresh token rotation, and room membership enforcement.
- **Multi-Layered Rate Limiting**: Dual-tier rate limiting covering authentication endpoints, upload quotas per authenticated user, and global per-IP limits.
- **Reverse Proxy & Auto-HTTPS**: Integrated Caddy reverse proxy providing automated TLS certificates via ACME/Let's Encrypt, HTTP/3 (QUIC) support, and hardened security headers.
- **Metrics & Observability**: Structured JSON logging via `log/slog` with route pattern labeling and Prometheus metrics (`/metrics`) monitoring HTTP durations, status codes, and active WebSockets.

---

## Run Instructions with Docker Compose

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (v24.0 or newer)
- [Docker Compose](https://docs.docker.com/compose/) (v2.20 or newer)
- Inbound access on ports `80/tcp`, `443/tcp`, and `443/udp` (for HTTP/3)
- A configured public DNS A/AAAA record pointing to your host

### 1. Configure Environment

Create a `.env` file in the project root:

```env
DOMAIN=chat.example.com
ACME_EMAIL=admin@example.com
JWT_SECRET=generate_a_random_secret_string_with_at_least_32_bytes_length
INSPECT_ARCHIVES=false
```

> [!TIP]
> Generate a strong 32-byte secret using OpenSSL:
> ```sh
> openssl rand -base64 32
> ```

### 2. Start Services

Build and run all services in detached mode:

```sh
docker compose up --build -d
```

Compose launches:
- `caddy`: Edge ingress proxy with automated HTTPS and static asset serving.
- `api`: PulseChat core REST and WebSocket API.
- `worker`: Background task processor for ClamAV scans, libvips image transformations, and media checks.
- `clamav`: ClamAV virus scanning daemon.
- `postgres`: PostgreSQL 17 database.
- `redis`: Redis 7.4 backing task queues, pub/sub, and presence tracking.

### 3. Apply Migrations

Run database migrations against the PostgreSQL container:

```sh
docker compose exec -T postgres psql -U pulsechat -d pulsechat < migrations/000001_initial.up.sql
docker compose exec -T postgres psql -U pulsechat -d pulsechat < migrations/000002_refresh_tokens.up.sql
docker compose exec -T postgres psql -U pulsechat -d pulsechat < migrations/000003_attachment_rejection.up.sql
docker compose exec -T postgres psql -U pulsechat -d pulsechat < migrations/000004_image_variants.up.sql
docker compose exec -T postgres psql -U pulsechat -d pulsechat < migrations/000005_attachment_delivery.up.sql
```

### 4. Verify Health

Check the status of running containers:

```sh
docker compose ps
```

Verify service health via the API endpoint:

```sh
curl -k https://localhost/healthz
```

---

## Environment Variables

| Variable | Description | Default / Example | Required |
|:---|:---|:---|:---:|
| `DOMAIN` | Public fully qualified domain name (FQDN) used for Caddy TLS and CORS/WS origin validation. | `chat.example.com` | **Yes** |
| `ACME_EMAIL` | Contact email address used by Let's Encrypt for ACME certificate registration and alerts. | `admin@example.com` | **Yes** |
| `JWT_SECRET` | Secret key used for signing and verifying JWT tokens (minimum 32 bytes). | *(Random string >= 32 bytes)* | **Yes** |
| `DATABASE_URL` | PostgreSQL connection URL including user credentials, host, and database name. | `postgres://pulsechat:pulsechat@postgres:5432/pulsechat?sslmode=disable` | **Yes** |
| `REDIS_URL` | Redis connection URL for queueing, pub/sub messaging, and presence tracking. | `redis://redis:6379/0` | No |
| `STORAGE_DIR` | Absolute path on disk where `quarantine` and `clean` file areas are created and stored. | `/app/data` | No |
| `HTTP_ADDR` | TCP listen address and port for the internal HTTP server. | `:8080` | No |
| `WS_ORIGIN` | Allowed origin for WebSocket upgrades. Set to `https://${DOMAIN}` by default. | `https://chat.example.com` | No |
| `TRUSTED_PROXY_CIDRS` | Comma-separated CIDR blocks for trusted reverse proxies (e.g., Caddy/internal Docker networks). | `172.16.0.0/12,10.0.0.0/8,192.168.0.0/16` | No |
| `CLAMD_ADDR` | Host and port address of the ClamAV daemon used by the worker. | `clamav:3310` | No |
| `INSPECT_ARCHIVES` | When `true`, enables deep archive inspection to reject zip bombs and path traversal attempts. | `false` | No |
| `SHUTDOWN_TIMEOUT` | Maximum duration allowed for graceful server and worker task completion upon termination. | `10s` | No |
| `POSTGRES_USER` | PostgreSQL superuser username for database initialization. | `pulsechat` | No |
| `POSTGRES_PASSWORD` | PostgreSQL superuser password for database initialization. | `pulsechat` | No |
| `POSTGRES_DB` | PostgreSQL default database name. | `pulsechat` | No |

---

## Security Notes

- **Quarantine Isolation**: Uploaded files are written directly into an isolated quarantine filesystem directory using randomized UUID paths. Quarantine files are unserved, never mapped to web routes, and completely inaccessible from external HTTP clients.
- **No Reliance on Client Metadata**: The server ignores client-provided `Content-Type` headers. Mime types and file signatures are derived through magic byte verification on upload and re-verified inside the worker before processing.
- **Deep Malware Scanning**: Files are scanned with ClamAV prior to any media extraction or transcode operation. Detected threats cause immediate rejection and quarantine file deletion.
- **Image Decompression Bomb & EXIF Protection**: libvips enforces a hard 50-megapixel bound to prevent memory exhaustion from decompression bombs. Images are re-encoded to WebP format with all EXIF and location metadata stripped.
- **Network Boundaries & Private API**: The API container exposes no ports directly to the host network in Docker Compose. All client traffic passes through Caddy. The API only accepts proxy forwarding headers (`X-Forwarded-For`) from explicitly configured CIDRs in `TRUSTED_PROXY_CIDRS`.
- **Dual-Tier Rate Limiting**:
  - Auth routes (`/auth/*`): 20 requests/minute per client IP.
  - Upload endpoints: 60 requests/minute per client IP and 10 uploads/minute per authenticated user.
- **Hardened HTTP Headers**: Caddy injects HTTP Strict Transport Security (HSTS), Content Security Policy (CSP), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and restrictive permissions policies.
- **Log Privacy & Redaction**: Access tokens, credentials, query parameters, and raw file identifiers are stripped and redacted from stdout access and application logs.

---

## Future Work

- **Content Delivery Network (CDN)**: Direct integration with cloud edge caches (e.g., Cloudflare, Amazon CloudFront) for high-speed, geographically distributed media delivery with authenticated signed URLs.
- **tus Resumable Uploads**: Implementation of the open [tus.io](https://tus.io/) protocol to support chunked, paused, and resumable uploads for unreliable mobile network connections and large assets.
- **MinIO / S3 Object Storage**: Decoupling the filesystem storage layer to support distributed S3-compatible object stores such as MinIO, AWS S3, or Google Cloud Storage via an abstracted bucket backend.
- **End-to-End Encryption (E2EE)**: Implementation of cryptographic ratchet protocols (such as Signal Protocol or MLS - Messaging Layer Security) providing end-to-end encrypted messaging and zero-knowledge client-side encrypted media attachments.
