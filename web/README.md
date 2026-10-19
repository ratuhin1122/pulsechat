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

The current UI covers login, registration, logout, paginated room listing, search within loaded rooms, and room selection. Message history, WebSockets, room creation/joining, and uploads are outside this step.

```sh
npm test
npm run build
```
