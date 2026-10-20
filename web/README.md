# Pulsechat web

Use **New chat** to open a modal with **Direct message** and **Rooms** tabs. User search calls `/users?q=` after a 250ms debounce, requires at least two characters, and displays usernames and display names. Choosing a user opens their existing DM or creates one. The Rooms tab creates groups with a public toggle, browses/searches public rooms in pages of 20, and joins a room before opening it. Already loaded memberships show an Open button. Requests cancel on query changes or modal dismissal; failures offer retry without automatically repeating mutations.

Use **Members** in the conversation header to open the side panel. Members load in pages of 50 with loading, empty, error, and retry states. The current members endpoint returns only user IDs and join dates, so the panel uses short ID labels with full IDs in tooltips. Group rooms have a **Leave room** button; successful leave removes the room from the sidebar and clears its active chat. Direct messages cannot be left through this API. Modal tabs support arrow/Home/End keys, native dialog focus containment and Escape, and return focus on dismissal. On narrow screens, the members panel overlays the conversation.

React, TypeScript, and Vite with plain CSS. Requires Node.js 22.12+ (Node 24 is also supported).

```sh
npm ci
npm run dev
```

Open `https://localhost:5173` and accept Vite's local development certificate. Start the Go API separately on port 8080. To change its address, copy `.env.example` to `.env` and set `API_TARGET`.

The development server proxies `/auth`, `/rooms`, `/users`, `/files`, `/presence`, and `/ws` to the API. HTTPS preserves the API's Secure HttpOnly refresh cookie without changing backend cookie policy. The browser sends same-origin requests; no CORS changes are needed.

In production, serve `dist/` over HTTPS, route those API prefixes to Go before the SPA fallback, and fall back to `index.html` for client routes such as `/login` and `/register`. Use `npm run preview` only for local build previews.

Access tokens stay in memory. A fresh page restores the session with `POST /auth/refresh`; subsequent refreshes run before expiry and after one authenticated 401. Concurrent refreshes share one request, and cookie mutations are serialized with logout. A rejected refresh signs out; a connection failure offers retry. No tokens or passwords are written to browser storage.

For local chat development, start the Go API with `WS_ORIGIN=https://localhost:5173`. Its strict Origin check must match the public browser origin, including behind Vite's proxy. Use the deployed HTTPS origin in production.

The UI includes login/registration, room selection, timestamped messages, text composition, older history on upward scroll, and presence dots. Enter sends a message; Shift+Enter inserts a newline. A disconnected composer retains its draft and disables sending. Messages appear after the server broadcasts them, without optimistic duplicates or automatic resend.

One WebSocket stays open across room switches. It sends an app-level ping every 25 seconds, reconnects with jittered exponential delays capped at 30 seconds, and resumes with the last received message ID. The initial history page can seed the cursor before the first live message. History and replay overlap are deduplicated by ID. Presence snapshots load on room entry/reconnect, with newer live transitions taking precedence. Presence becomes unknown while disconnected.

Sender labels use short user IDs because the API does not expose display names. Attach files with the picker, drop them into the conversation, or paste an image. Review local previews before sending. Optional image compression resizes JPEG/PNG/WebP to at most 1600px and keeps the result only when smaller; GIFs retain animation. Server validation remains authoritative.

Uploads show progress, then remain processing until a ready/rejected event arrives (a ready message also confirms completion). Switching rooms preserves drafts and active uploads; leaving the workspace cancels active requests and releases local blob URLs. An interrupted upload is never automatically resent. If a rejection event was missed while disconnected, its draft can remain processing because the API has no attachment-status lookup; dismiss it after checking the conversation.

Images use blurhash placeholders and open a full display image in a keyboard-accessible zoom dialog. Documents download through authenticated requests; voice bubbles provide playback and duration. Files use temporary blob URLs so access tokens never appear in file URLs. Audio is fetched in full before local playback/seeking. Voice recording requires a secure browser context (HTTPS or localhost), microphone permission, and a browser supporting MediaRecorder with WebM/Opus or Ogg/Opus. Recording stops just below five minutes; browsers that only record MP4 must use an allowed audio file instead.

```sh
npm test
npm run build
```

The conversation header includes Logout and an account label. The current auth API returns only tokens, so the label uses display-name/username claims when available, registration username or login email prefix during that session, and the user ID after a cookie-only restore. A server profile endpoint is needed to show the stored display name consistently.

Room summaries show unread badges and the latest message preview, refreshed after realtime messages/read events and reconnects. Opening a visible conversation marks its latest loaded message read; failures are shown and retried on focus/reconnect. Own messages show one tick for sent and two when another member's live read marker covers them. Existing read markers are not returned by the API, so historical read receipts cannot be restored after a page reload. Failed uploads remain visible as rejected until dismissed. Every WebSocket reconnect awaits refresh before opening the socket.
