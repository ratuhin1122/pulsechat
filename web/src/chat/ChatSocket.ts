import type { Session } from '../auth/session';
import { compareMessages, parseEvent } from './types';
import type { ConnectionStatus, Message, ServerEvent } from './types';

interface Handlers {
  status: (status: ConnectionStatus) => void;
  event: (event: ServerEvent) => void;
  connected: () => void;
}
interface Options { create?: (url: string) => WebSocket; random?: () => number; origin?: string }

export class ChatSocket {
  private socket: WebSocket | null = null;
  private stopped = true;
  private online = true;
  private generation = 0;
  private attempts = 0;
  private cursor: Message | null = null;
  private replayAfterSeed = false;
  private refreshToken: string | undefined;
  private retry: ReturnType<typeof setTimeout> | undefined;
  private ping: ReturnType<typeof setInterval> | undefined;
  private pong: ReturnType<typeof setTimeout> | undefined;
  private stable: ReturnType<typeof setTimeout> | undefined;
  private handshake: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly session: Pick<Session, 'getAccessToken' | 'refresh'>,
    private readonly handlers: Handlers, private readonly options: Options = {}) {}

  start(online = true) { this.stopped = false; this.online = online; void this.connect(); }
  stop() { this.stopped = true; this.generation++; this.clear(); this.close(); }
  setOnline(online: boolean) {
    this.online = online;
    if (!online) {
      if (!this.cursor) this.replayAfterSeed = true;
      this.generation++; this.clear(); this.close(); this.handlers.status('offline');
    } else if (!this.stopped && !this.socket) { clearTimeout(this.retry); void this.connect(); }
  }
  seedCursor(message: Message) {
    if (this.cursor) return;
    this.cursor = message;
    // History may finish after a reconnect that had no known cursor yet.
    if (this.replayAfterSeed) { this.replayAfterSeed = false; this.socket?.close(); }
  }

  send(roomID: string, body: string): boolean {
    if (!this.socket || this.socket.readyState !== 1 || this.socket.bufferedAmount > 64 * 1024) return false;
    const frame = JSON.stringify({ type: 'message', room_id: roomID, body });
    if (!body.trim() || new TextEncoder().encode(body).length > 4096 || new TextEncoder().encode(frame).length > 8192) return false;
    try { this.socket.send(frame); return true; }
    catch { return false; }
  }

  private clear() {
    clearTimeout(this.retry); clearInterval(this.ping); clearTimeout(this.pong);
    clearTimeout(this.stable); clearTimeout(this.handshake);
    this.pong = undefined;
  }
  private close() { const socket = this.socket; this.socket = null; socket?.close(); }
  private reconnect() {
    this.clear(); this.socket = null;
    if (!this.cursor) this.replayAfterSeed = true;
    if (this.stopped || !this.online) return;
    this.handlers.status('reconnecting');
    const base = Math.min(30_000, 1000 * 2 ** Math.min(this.attempts++, 5));
    this.retry = setTimeout(() => { void this.connect(); }, base * (0.5 + (this.options.random ?? Math.random)() * 0.5));
  }

  private async connect() {
    if (this.stopped) return;
    if (!this.online) { this.handlers.status('offline'); return; }
    const generation = ++this.generation;
    this.handlers.status(this.attempts ? 'reconnecting' : 'connecting');
    try {
      const token = this.refreshToken ? await this.session.refresh(this.refreshToken) : await this.session.getAccessToken();
      this.refreshToken = undefined;
      if (this.stopped || generation !== this.generation) return;
      const url = new URL('/ws', this.options.origin ?? window.location.origin);
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      url.searchParams.set('access_token', token);
      if (this.cursor) url.searchParams.set('last_message_id', this.cursor.id);
      const socket = (this.options.create ?? ((url: string) => new WebSocket(url)))(url.toString());
      this.socket = socket;
      let opened = false;
      const current = () => !this.stopped && this.socket === socket && generation === this.generation;
      const awaitPong = () => {
        clearTimeout(this.pong);
        this.pong = setTimeout(() => { if (current()) socket.close(); }, 10_000);
      };
      this.handshake = setTimeout(() => { if (current()) socket.close(); }, 10_000);
      socket.onopen = () => {
        if (!current()) return;
        opened = true; clearTimeout(this.handshake);
        this.handlers.status('connected'); this.handlers.connected();
        this.stable = setTimeout(() => { this.attempts = 0; }, 30_000);
        this.ping = setInterval(() => {
          if (!current()) return;
          try { socket.send(JSON.stringify({ type: 'ping' })); }
          catch { socket.close(); return; }
          awaitPong();
        }, 25_000);
      };
      socket.onmessage = ({ data }) => {
        if (!current()) return;
        const event = parseEvent(data);
        if (!event) return;
        if (event.type === 'pong') { clearTimeout(this.pong); this.pong = undefined; }
        else if (this.pong !== undefined) awaitPong(); // Replay frames prove liveness while its pong waits behind the replay.
        if (event.type === 'message' && (!this.cursor || compareMessages(this.cursor, event.message) < 0)) this.cursor = event.message;
        this.handlers.event(event);
      };
      socket.onerror = () => { if (current()) socket.close(); };
      socket.onclose = (event) => {
        if (!current()) return;
        if ((!opened && this.attempts === 0) || event.code === 1008 && event.reason.includes('access token expired')) this.refreshToken = token;
        this.reconnect();
      };
    } catch { if (!this.stopped && generation === this.generation) this.reconnect(); }
  }
}
