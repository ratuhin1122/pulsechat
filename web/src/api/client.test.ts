import { afterEach, describe, expect, it, onTestFinished, vi } from 'vitest';
import { ApiClient } from './client';
import { ApiError, HttpClient } from './http';
import { Session } from '../auth/session';

function token(value = 'access-token') {
  return Response.json({ access_token: value, token_type: 'Bearer', expires_in: 900 });
}

function setup(handler: typeof fetch) {
  const fetcher = vi.fn(handler);
  const http = new HttpClient(fetcher);
  const session = new Session(http);
  onTestFinished(() => session.dispose());
  return { session, api: new ApiClient(http, session), fetcher };
}

afterEach(() => vi.useRealTimers());

describe('memory-only authentication', () => {
  it('restores once for concurrent callers, using the cookie without browser storage', async () => {
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    const { session, api, fetcher } = setup(async (path, init) => {
      expect(init?.credentials).toBe('include');
      if (path === '/auth/refresh') return token();
      expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer access-token');
      return Response.json([]);
    });
    await Promise.all([session.restore(), session.restore()]);
    expect(session.getSnapshot().status).toBe('authenticated');
    await api.listRooms();
    expect(fetcher.mock.calls.filter(([path]) => path === '/auth/refresh')).toHaveLength(1);
    expect(storage).not.toHaveBeenCalled();
    expect(JSON.stringify(session.getSnapshot())).not.toContain('access-token');
  });

  it.each([
    { name: 'expired cookie', response: () => new Response('invalid refresh token', { status: 401 }), status: 'anonymous' },
    { name: 'server outage', response: () => new Response('unavailable', { status: 503 }), status: 'unavailable' },
  ])('handles $name', async ({ response, status }) => {
    const { session } = setup(async () => response());
    await session.restore();
    expect(session.getSnapshot().status).toBe(status);
  });

  it('recovers from a network failure without treating it as a rejected session', async () => {
    let offline = true;
    const { session } = setup(async () => {
      if (offline) throw new TypeError('offline');
      return token();
    });
    await session.restore();
    expect(session.getSnapshot().status).toBe('unavailable');
    offline = false;
    await session.restore();
    expect(session.getSnapshot().status).toBe('authenticated');
  });

  it('refreshes before the 15-minute access token expires', async () => {
    vi.useFakeTimers();
    const { session, fetcher } = setup(async () => token('renewed'));
    await session.login({ email: 'you@example.com', password: 'password' });
    await vi.advanceTimersByTimeAsync(870_000);
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/auth/login', '/auth/refresh']);
  });

  it('waits for a rotating refresh before revoking its cookie on logout', async () => {
    let release!: (response: Response) => void;
    const { session, fetcher } = setup(async (path) => {
      if (path === '/auth/refresh') return new Promise<Response>((resolve) => { release = resolve; });
      return new Response(null, { status: 204 });
    });
    const refresh = session.refresh();
    await vi.waitFor(() => expect(release).toBeTypeOf('function'));
    const logout = session.logout();
    expect(fetcher).toHaveBeenCalledTimes(1);
    release(token());
    await Promise.all([refresh, logout]);
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/auth/refresh', '/auth/logout']);
    expect(session.getSnapshot().status).toBe('anonymous');
  });

  it('does not report successful logout when cookie revocation failed', async () => {
    const { session } = setup(async (path) => path === '/auth/login' ? token() : new Response('unavailable', { status: 503 }));
    await session.login({ email: 'you@example.com', password: 'password' });
    await expect(session.logout()).rejects.toBeInstanceOf(ApiError);
    expect(session.getSnapshot().status).toBe('authenticated');
  });
});

describe('typed protected API', () => {
  it('shares one refresh across concurrent 401 responses and retries with the new token', async () => {
    const { session, api, fetcher } = setup(async (path, init) => {
      if (path === '/auth/login') return token('old');
      if (path === '/auth/refresh') return token('new');
      return new Headers(init?.headers).get('Authorization') === 'Bearer old'
        ? new Response('expired', { status: 401 }) : Response.json([]);
    });
    await session.login({ email: 'you@example.com', password: 'password' });
    await Promise.all([api.listRooms(), api.listRooms()]);
    expect(fetcher.mock.calls.filter(([path]) => path === '/auth/refresh')).toHaveLength(1);
    expect(session.getSnapshot().status).toBe('authenticated');
  });

  it('stops after one retry and expires a rejected access token', async () => {
    const { session, api, fetcher } = setup(async (path) => {
      if (path === '/auth/login') return token('old');
      if (path === '/auth/refresh') return token('new');
      return new Response('unauthorized', { status: 401 });
    });
    await session.login({ email: 'you@example.com', password: 'password' });
    await expect(api.listRooms()).rejects.toMatchObject({ status: 401 });
    expect(session.getSnapshot().status).toBe('anonymous');
    expect(fetcher.mock.calls.filter(([path]) => String(path).startsWith('/rooms'))).toHaveLength(2);
  });

  it('keeps a forbidden request from rotating or destroying the session', async () => {
    const { session, api, fetcher } = setup(async (path) => path === '/auth/login' ? token() : new Response('forbidden', { status: 403 }));
    await session.login({ email: 'you@example.com', password: 'password' });
    await expect(api.listRooms()).rejects.toMatchObject({ status: 403 });
    expect(session.getSnapshot().status).toBe('authenticated');
    expect(fetcher.mock.calls.some(([path]) => path === '/auth/refresh')).toBe(false);
  });

  it('rejects malformed room responses', async () => {
    const { api } = setup(async (path) => path === '/auth/refresh' ? token() : Response.json([{ id: 1 }]));
    await expect(api.listRooms()).rejects.toMatchObject({ status: 502 });
  });

  it('cancels one caller without cancelling a shared refresh', async () => {
    let release!: (response: Response) => void;
    const { api } = setup(async (path) => path === '/auth/refresh'
      ? new Promise<Response>((resolve) => { release = resolve; }) : Response.json([]));
    const controller = new AbortController();
    const cancelled = api.listRooms({ signal: controller.signal }).catch((error: unknown) => error);
    const active = api.listRooms();
    await vi.waitFor(() => expect(release).toBeTypeOf('function'));
    controller.abort();
    release(token());
    expect(await cancelled).toMatchObject({ name: 'AbortError' });
    await expect(active).resolves.toEqual([]);
  });
});
