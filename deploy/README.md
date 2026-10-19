# HTTPS deployment

Set `DOMAIN` to a public hostname (without a scheme or path), `ACME_EMAIL` to your certificate contact email, and `JWT_SECRET` to a random secret of at least 32 bytes. Point the hostname's DNS records at this host and allow inbound TCP 80/443 (and UDP 443 for HTTP/3).

```sh
docker compose up --build -d
```

Apply the existing migrations before using the app. Caddy builds and serves `web/dist`, redirects HTTP to HTTPS, obtains/renews certificates, and proxies the API routes and WebSocket upgrades. Certificate state persists in `caddy_data` and `caddy_config`. The Go API has no published host port. Its WebSocket Origin is set to `https://DOMAIN` automatically.

Security headers include HSTS, CSP, nosniff, frame denial, no-referrer, and a permissions policy permitting the app's microphone. Caddy overwrites `X-Forwarded-For`; the API trusts forwarding only from configured private proxy ranges (`TRUSTED_PROXY_CIDRS`). Keep the API private. For a different topology, narrow those CIDRs to the actual proxy peers; never trust client-supplied forwarding headers on a publicly reachable API.

Auth endpoints share a limit of 20 POSTs/minute/client IP. Uploads allow 60 POSTs/minute/client IP in addition to the existing 10/minute/authenticated user. Limits are per API instance, return 429 with `Retry-After`, and expire idle entries with bounded memory. Redis retains its queue/pubsub/presence role.

Caddy writes JSON access logs to stdout, redacting access-token query parameters; credentials in headers remain redacted by Caddy. API slog request logs include route patterns, status, response bytes and duration, without queries, credentials, bodies, or raw room/file identifiers. WebSocket request duration includes the connection lifetime.

Prometheus can scrape `http://api:8080/metrics` from the Compose network. Caddy blocks public `/metrics`. Metrics include Go/process collectors, HTTP counts/statuses, durations, and in-flight requests. Labels use registered route patterns and bounded methods. WebSocket counts complete on disconnect; active WebSockets are included in the in-flight gauge. No monitoring service is added.

GitHub Actions vets, lints, and runs race tests with the `libvips` build tag and system libvips/ffmpeg installed, then builds both the API/worker image and Caddy/frontend image. Image builds are local to CI and are not pushed or deployed. The Caddy image is also checked with `caddy validate`.
