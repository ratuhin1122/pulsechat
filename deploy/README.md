# HTTPS deployment

## Local HTTP development

Start Docker Desktop first. With your existing root `.env`, run `make start` from the repository root, then open **http://localhost:8080**. This builds the frontend and Go services, starts PostgreSQL/Redis/ClamAV, applies pending migrations with golang-migrate, and starts the API, worker, and Caddy. The first run may take several minutes while images build and ClamAV loads.

`make stop` stops containers without deleting data, `make logs` follows logs, `make ps` shows status, and `make migrate` applies pending migrations. Run `make` for help. GNU Make and Docker Compose 2.24.4+ are required (the local override replaces HTTPS port mappings).

Local mode binds only `127.0.0.1:8080`, disables automatic HTTPS in Caddy, sets the WebSocket Origin to `http://localhost:8080`, and uses a separate HttpOnly/SameSite development refresh cookie without Secure. `DEV_HTTP` defaults to false and requires an explicit loopback HTTP origin. Use the exact localhost URL above. No changes to `.env` are needed. Production retains HTTPS and its Secure cookie.

If you previously applied SQL files manually rather than using golang-migrate, establish the correct existing migration version before running `make start`; never force a version without verifying the schema. Do not delete database volumes to resolve a migration error.

## Production HTTPS

`make prod-start` applies pending migrations and starts the production stack; `make prod-stop` stops it without deleting volumes. Alternatively, use the Compose commands below and apply migrations separately.

Set `DOMAIN` to a public hostname (without a scheme or path), `ACME_EMAIL` to your certificate contact email, and `JWT_SECRET` to a random secret of at least 32 bytes. Point the hostname's DNS records at this host and allow inbound TCP 80/443 (and UDP 443 for HTTP/3).

```sh
docker compose up --build -d
```

Apply the existing migrations before using the app. Caddy builds and serves `web/dist`, redirects HTTP to HTTPS, obtains/renews certificates, and proxies the API routes and WebSocket upgrades. Certificate state persists in `caddy_data` and `caddy_config`. The public API port is not published; its metrics listener is published on loopback only. Its WebSocket Origin is set to `https://DOMAIN` automatically.

Security headers include HSTS, CSP, nosniff, frame denial, no-referrer, and a permissions policy permitting the app's microphone. Caddy overwrites `X-Forwarded-For`; the API trusts forwarding only from configured private proxy ranges (`TRUSTED_PROXY_CIDRS`). Keep the API private. For a different topology, narrow those CIDRs to the actual proxy peers; never trust client-supplied forwarding headers on a publicly reachable API.

Auth endpoints share a limit of 20 POSTs/minute/client IP. Uploads allow 60 POSTs/minute/client IP in addition to the existing 10/minute/authenticated user. Limits are per API instance, return 429 with `Retry-After`, and expire idle entries with bounded memory. Redis retains its queue/pubsub/presence role.

Caddy writes JSON access logs to stdout, redacting access-token query parameters; credentials in headers remain redacted by Caddy. API slog request logs include route patterns, status, response bytes and duration, without queries, credentials, bodies, or raw room/file identifiers. WebSocket request duration includes the connection lifetime.

Prometheus scrapes API, worker, PostgreSQL exporter, Redis exporter and Caddy every 15 seconds. Caddy blocks `/metrics` on the public site and exposes its metrics on a dedicated internal listener, without exposing its admin API. Grafana provisions Prometheus automatically; Loki is provisioned for Docker logs; Tempo remains commented out until its service exists.

GitHub Actions vets, lints, and runs race tests with the `libvips` build tag and system libvips/ffmpeg installed, then builds both the API/worker image and Caddy/frontend image. Image builds are local to CI and are not pushed or deployed. The Caddy image is also checked with `caddy validate`.

Metrics use a separate API listener: `METRICS_ADDR` defaults to `127.0.0.1:9090` for local processes. Compose sets `:9090` inside the API container, published as `127.0.0.1:9092`; scrape `http://api:9090/metrics` from the private Compose network. The public API does not register `/metrics`, and Caddy does not proxy this listener. HTTP duration labels contain ServeMux patterns (or `unmatched`), never request paths or queries.

WebSocket gauges count this API instance's connections and distinct connected users. Disconnect reasons are bounded to `heartbeat_timeout`, `client_close`, and `error` (including eviction/shutdown). `chat_messages_total` counts live message events once per receiving API hub, including attachment messages, excluding replay and recipient fan-out; with Redis, each subscribed API counts its copy, so do not sum this metric as a cluster-wide persisted-message count. `uploads_total{result="accepted"}` counts persisted pending uploads even if enqueueing subsequently fails; `rejected_stage1` counts invalid multipart/file validation failures, excluding authentication, membership, throttling and infrastructure errors.

Worker metrics have a separate registry and listener: `WORKER_METRICS_ADDR` defaults to `127.0.0.1:9091`; Compose binds `:9091` internally, published as `127.0.0.1:9091`. Scrape `http://worker:9091/metrics` on the private network. Queue depth for the configured `default` queue refreshes immediately and every 10 seconds, summing pending, scheduled, retry and aggregating tasks. Active and archived tasks are excluded; inspection failures retain the last value and are logged.

Attachment metrics describe processing attempts, excluding terminal cleanup and notification-only retries. Errors before metadata loads use kind `unknown`. Scan failures count as `error` even when the safety policy successfully rejects the file; known malware counts as `infected`, validation failures as `rejected`. Returned processing/cleanup errors take precedence as `error`. Original bytes count each pending processing attempt; processed bytes count successfully committed clean outputs, including both image variants. Completion notification delivery is outside the processing timer. Counters reset on worker restart.

Monitoring data persists in the named `prometheus_data` and `grafana_data` volumes. Grafana is at `http://127.0.0.1:3000` (user `admin`, password `GRAFANA_ADMIN_PASSWORD` from the ignored `.env` file); Prometheus is at `http://127.0.0.1:9090`. Other loopback-only endpoints are API `:9092`, worker `:9091`, PostgreSQL exporter `:9187`, Redis exporter `:9121`, and Caddy metrics `:9180`, each at `/metrics`. Container targets use service names and container ports, independently of host port mappings. Production and local HTTP Compose configurations both retain these loopback bindings.

Grafana's admin environment settings initialize a new Grafana database. Changing `.env` later does not change an existing admin password; rotate it through Grafana's admin controls. Do not commit `.env` or print resolved Compose configuration containing credentials.

Exporter credentials come from the ignored `.env`: `POSTGRES_EXPORTER_USER` and `POSTGRES_EXPORTER_PASSWORD` feed the PostgreSQL exporter's separate user/password settings. `REDIS_EXPORTER_USER` and `REDIS_EXPORTER_PASSWORD` feed Redis ACL authentication; leave them empty for the current unauthenticated local Redis. These settings configure exporter connections, not database/Redis accounts; keep them aligned with the existing services. The Redis target continues to use `REDIS_URL`. PostgreSQL credentials do not need URL escaping because they are supplied separately from its URI.

Grafana automatically provisions four dashboards in the **Pulsechat** folder: **Chat Overview**, **HTTP and API**, **Upload Pipeline**, and **Infrastructure**. JSON sources are in `deploy/grafana/dashboards/`, mounted read-only; the file provider checks for updates every 30 seconds. Dashboards refresh every 15 seconds and share the provisioned Prometheus datasource. HTTP latency excludes WebSocket connection lifetimes. Online users and live chat events are shown per API instance; shared queue depth uses the maximum inspector sample rather than summing replicas.

Infrastructure includes optional slow-query and container resource panels. Slow-query means require PostgreSQL `pg_stat_statements` and the postgres-exporter `stat_statements` collector; container CPU/memory requires cAdvisor-compatible metrics with scrape job `cadvisor`. Those collectors are not part of this dashboard change and their panels show No data until configured. The existing longest-active-transaction panel is a separate diagnostic, not a substitute for query duration. Byte comparisons show processing throughput, including retries/rejections and image thumbnails, rather than an exact compression ratio.

Loki stores logs in `loki_data` and listens on loopback host port `3100`. Grafana Alloy discovers containers belonging to this Compose project through the Docker socket and forwards their original stdout/stderr lines to Loki. Alloy keeps its Docker log positions in `alloy_data`; its diagnostic endpoint is loopback-only at `12345`. The Docker socket is mounted read-only, though daemon API access remains privileged. Discovery uses Compose's project label, so unrelated Docker projects are excluded.

API and worker use JSON slog records with `service`, `request_id`, `user_id`, `room_id`, and `attachment_id`. Unavailable correlation fields are empty. Request middleware echoes a valid `X-Request-ID` (1â€“128 ASCII letters, digits, periods, underscores, or hyphens) or generates one. Authentication and handlers add known IDs to the request scope; websocket messages use isolated room scopes. Attachment queue payloads carry the upload request ID through retries; worker logs add canonical user/room/attachment IDs from the database. Older jobs fall back to their task ID or a generated ID. Request bodies, credentials and raw query strings remain excluded from API request logs.

The provisioned **Logs** dashboard shows JSON error logs, per-request logs, and all container logs. Paste the response `X-Request-ID` into its Request ID field; blank shows all. Loki indexes only the low-cardinality project/service/container labels; correlation IDs remain JSON fields parsed at query time. Grafana uses the Loki datasource at `http://loki:3100`. Project services are not started by configuration validation.

Tracing exports OTLP/HTTP from API and worker to the single-binary Tempo service at `http://tempo:4318`. Tempo stores traces in `tempo_data`; host access to its query API (`3200`) and OTLP receiver (`4318`) is loopback-only. Grafana provisions the Tempo datasource and a **View trace** link from Loki JSON `trace_id` fields, plus a trace-to-logs lookup. Trace IDs remain JSON fields, not Loki stream labels.

Configure `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` in `.env`. Defaults are `parentbased_traceidratio` and `0.1` (10% of new root traces). Set the argument to `1` for debugging or `0` to stop sampling new roots; child spans and jobs preserve the parent's decision. Explicit `always_on` / `always_off` samplers are also supported and override parent decisions. `OTEL_EXPORTER_OTLP_ENDPOINT` is the HTTP base URL (Compose defaults to Tempo; standalone processes default to `http://localhost:4318`); `/v1/traces` is appended. Exporters batch spans and flush with a bounded timeout during shutdown. An unsampled trace ID can appear in logs without a stored Tempo trace.

HTTP server spans use `otelhttp`, W3C Trace Context and ServeMux route names. HTTP span paths use route patterns and query strings are excluded. Pgx QueryTracer spans omit SQL statements/arguments. The upload HTTP span parents `attachment.enqueue`; the job payload contains `trace_id` and W3C `trace_context` (traceparent/tracestate, not baggage). `attachment.process`, `clamd.scan`, image/media processing and `websocket.notify` continue that trace. Redis transports the context in its internal envelope so API `websocket.notify.deliver` spans cover local hub delivery; it is not sent to browsers. Legacy jobs without context start a new trace. Tracing providers are injected rather than registered globally.
