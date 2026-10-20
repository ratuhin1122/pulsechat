import { ApiError, errorMessage, HttpClient } from '../api/http';
import type { LoginInput, RegisterInput, TokenResponse } from '../api/types';

export interface SessionSnapshot {
  status: 'checking' | 'anonymous' | 'authenticated' | 'unavailable';
  error: string | null;
  userID?: string;
  displayName?: string;
}

// Tokens live only in this instance. The refresh cookie is owned by the server.
export class Session {
  private token: string | null = null;
  private expiresAt = 0;
  private snapshot: SessionSnapshot = { status: 'checking', error: null };
  private readonly listeners = new Set<() => void>();
  private refreshFlight: Promise<string> | null = null;
  private authQueue: Promise<unknown> = Promise.resolve();
  private timer: ReturnType<typeof setTimeout> | undefined;
  private signingOut = false;
  private disposed = false;

  constructor(private readonly http: HttpClient) {}

  getSnapshot = (): SessionSnapshot => this.snapshot;
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };

  private update(status: SessionSnapshot['status'], error: string | null = null) {
    this.snapshot = { ...this.snapshot, status, error, ...(status === 'anonymous' ? { userID: undefined, displayName: undefined } : {}) };
    this.listeners.forEach((listener) => listener());
  }

  // Serialize cookie mutations: logout must revoke the cookie from any in-flight refresh.
  private enqueue<T>(operation: () => Promise<T>): Promise<T> {
    const next = this.authQueue.then(operation, operation);
    this.authQueue = next.catch(() => undefined);
    return next;
  }

  private accept(response: TokenResponse): string {
    if (!response || typeof response.access_token !== 'string' || !response.access_token
      || response.token_type !== 'Bearer' || !Number.isFinite(response.expires_in) || response.expires_in <= 0) {
      throw new ApiError(502, 'The server returned an invalid session.');
    }
    this.token = response.access_token;
    try {
      const encoded = this.token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
      const claims = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(encoded), (char) => char.charCodeAt(0))));
      this.snapshot = { ...this.snapshot, userID: typeof claims.sub === 'string' ? claims.sub : undefined,
        displayName: typeof claims.display_name === 'string' && claims.display_name.trim() ? claims.display_name
          : typeof claims.username === 'string' ? claims.username : this.snapshot.displayName };
    } catch { /* Identity labels are optional; authorization remains server-side. */ }
    this.expiresAt = Date.now() + response.expires_in * 1000;
    this.update('authenticated');
    this.schedule();
    return this.token;
  }

  private clear() {
    clearTimeout(this.timer);
    this.token = null;
    this.expiresAt = 0;
    this.update('anonymous');
  }

  private schedule(delay = Math.max(1000, this.expiresAt - Date.now() - 30_000)) {
    clearTimeout(this.timer);
    if (this.disposed || this.signingOut || !this.token) return;
    this.timer = setTimeout(() => {
      void this.refresh().catch(() => { if (this.token) this.schedule(15_000); });
    }, delay);
  }

  async restore(): Promise<void> {
    if (this.token && this.expiresAt - Date.now() > 30_000) return;
    if (this.snapshot.status === 'unavailable') this.update('checking');
    try { await this.refresh(); } catch { /* The snapshot drives the recovery UI. */ }
  }

  refresh(staleToken?: string): Promise<string> {
    if (this.signingOut) return Promise.reject(new ApiError(401, 'Signing out.'));
    if (staleToken && this.token && staleToken !== this.token && this.expiresAt > Date.now()) {
      return Promise.resolve(this.token);
    }
    if (staleToken && !this.token) return Promise.reject(new ApiError(401, 'Please sign in again.'));
    if (this.refreshFlight) return this.refreshFlight;
    const next = this.enqueue(async () => {
      try { return this.accept(await this.http.post<TokenResponse>('/auth/refresh')); }
      catch (error) {
        if (error instanceof ApiError && error.status === 401) this.clear();
        else if (!this.token) this.update('unavailable', errorMessage(error));
        throw error;
      }
    });
    this.refreshFlight = next;
    void next.finally(() => { if (this.refreshFlight === next) this.refreshFlight = null; }).catch(() => undefined);
    return next;
  }

  async getAccessToken(): Promise<string> {
    if (this.signingOut) throw new ApiError(401, 'Signing out.');
    if (this.token && this.expiresAt - Date.now() > 5000) return this.token;
    return this.refresh();
  }

  expire(token: string) { if (this.token === token) this.clear(); }

  async login(input: LoginInput): Promise<void> {
    await this.enqueue(async () => {
      const response = await this.http.post<TokenResponse>('/auth/login', input);
      this.snapshot = { ...this.snapshot, displayName: input.email.split('@')[0] };
      this.accept(response);
    });
  }

  async register(input: RegisterInput): Promise<void> {
    await this.enqueue(async () => {
      const response = await this.http.post<TokenResponse>('/auth/register', input);
      this.snapshot = { ...this.snapshot, displayName: input.username };
      this.accept(response);
    });
  }

  async deleteAccount(password: string): Promise<void> {
    const token = await this.getAccessToken();
    this.signingOut = true;
    clearTimeout(this.timer);
    try {
      await this.enqueue(() => this.http.send<void>('/auth/account', {
        method: 'DELETE', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ password }),
      }));
      this.clear();
    } finally { this.signingOut = false; this.schedule(); }
  }

  async logout(): Promise<void> {
    this.signingOut = true;
    clearTimeout(this.timer);
    try {
      await this.enqueue(() => this.http.post<void>('/auth/logout'));
      this.clear();
    } finally {
      this.signingOut = false;
      this.schedule();
    }
  }

  dispose() {
    this.disposed = true;
    clearTimeout(this.timer);
    this.listeners.clear();
  }
}
