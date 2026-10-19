# Pulsechat web

React, TypeScript, and Vite with plain CSS. Requires Node.js 22.12+ (Node 24 is also supported).

```sh
npm ci
npm run dev
```

Open `https://localhost:5173` and accept Vite's local development certificate. Start the Go API separately on port 8080. To change its address, copy `.env.example` to `.env` and set `API_TARGET`.

The development server proxies `/auth`, `/rooms`, `/files`, `/presence`, and `/ws` to the API. HTTPS preserves the API's Secure HttpOnly refresh cookie without changing backend cookie policy. The browser sends same-origin requests; no CORS changes are needed.

In production, serve `dist/` over HTTPS, route those API prefixes to Go before the SPA fallback, and fall back to `index.html` for client routes such as `/login` and `/register`. Use `npm run preview` only for local build previews.

Access tokens stay in memory. A fresh page restores the session with `POST /auth/refresh`; subsequent refreshes run before expiry and after one authenticated 401. Concurrent refreshes share one request, and cookie mutations are serialized with logout. A rejected refresh signs out; a connection failure offers retry. No tokens or passwords are written to browser storage.

For local chat development, start the Go API with `WS_ORIGIN=https://localhost:5173`. Its strict Origin check must match the public browser origin, including behind Vite's proxy. Use the deployed HTTPS origin in production.

The UI includes login/registration, room selection, timestamped messages, text composition, older history on upward scroll, and presence dots. Enter sends a message; Shift+Enter inserts a newline. A disconnected composer retains its draft and disables sending. Messages appear after the server broadcasts them, without optimistic duplicates or automatic resend.

One WebSocket stays open across room switches. It sends an app-level ping every 25 seconds, reconnects with jittered exponential delays capped at 30 seconds, and resumes with the last received message ID. The initial history page can seed the cursor before the first live message. History and replay overlap are deduplicated by ID. Presence snapshots load on room entry/reconnect, with newer live transitions taking precedence. Presence becomes unknown while disconnected.

Sender labels use short user IDs because the API does not expose display names. Attachment messages show their filenames; attachment playback/download controls, room creation/joining, and uploading are outside this step.

```sh
npm test
npm run build
```
