import { StrictMode } from 'react';
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { App } from './App';
import { AuthProvider } from './auth/AuthProvider';
import { Session } from './auth/session';
import { ApiClient } from './api/client';
import { HttpClient } from './api/http';
import { TestSocket } from './chat/testSocket';

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (this: HTMLDialogElement) { this.setAttribute('open', ''); });
  HTMLDialogElement.prototype.close = vi.fn(function (this: HTMLDialogElement) { this.removeAttribute('open'); });
  TestSocket.instances = []; vi.stubGlobal('WebSocket', TestSocket); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function setup(handler: typeof fetch, path = '/login') {
  const fetcher = vi.fn(handler);
  const http = new HttpClient(fetcher);
  const session = new Session(http);
  onTestFinished(() => session.dispose());
  render(<StrictMode><MemoryRouter initialEntries={[path]}>
    <AuthProvider services={{ session, api: new ApiClient(http, session) }}><App /></AuthProvider>
  </MemoryRouter></StrictMode>);
  return { user: userEvent.setup(), fetcher, session };
}

const signedIn = () => Response.json({ access_token: 'memory-token', token_type: 'Bearer', expires_in: 900 });
const rooms = [
  { id: 'room-1', type: 'group', name: 'Engineering', created_at: '2026-10-19T00:00:00Z' },
  { id: 'room-2', type: 'group', name: 'Design studio', created_at: '2026-10-19T00:00:00Z' },
];

it('signs in, lists and filters rooms, selects a room, and signs out', async () => {
  const { user, fetcher, session } = setup(async (path, init) => {
    expect(init?.credentials).toBe('include');
    if (path === '/auth/refresh') return new Response('no cookie', { status: 401 });
    if (path === '/auth/login') {
      expect(JSON.parse(String(init?.body))).toEqual({ email: 'you@example.com', password: 'password123' });
      return signedIn();
    }
    if (path === '/auth/logout') return new Response(null, { status: 204 });
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer memory-token');
    if (String(path).includes('/messages?') || String(path).startsWith('/presence?')) return Response.json([]);
    return Response.json(rooms);
  });
  await screen.findByRole('heading', { name: 'Welcome back.' });
  expect(screen.getByLabelText('Email address').getAttribute('autocomplete')).toBe('off');
  expect(screen.getByLabelText('Password', { exact: true }).getAttribute('autocomplete')).toBe('off');
  await user.type(screen.getByLabelText('Email address'), 'you@example.com');
  await user.type(screen.getByLabelText('Password', { exact: true }), 'password123');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  await screen.findByRole('button', { name: 'Engineering, Group room' });
  await user.type(screen.getByRole('searchbox', { name: 'Find a room' }), 'design');
  expect(screen.queryByRole('button', { name: 'Engineering, Group room' })).toBeNull();
  await user.clear(screen.getByRole('searchbox', { name: 'Find a room' }));
  await user.click(screen.getByRole('button', { name: 'Open rooms' }));
  expect(screen.getByRole('button', { name: 'Close rooms' }).getAttribute('aria-expanded')).toBe('true');
  await user.click(screen.getByRole('button', { name: 'Engineering, Group room' }));
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Engineering');
  expect(screen.getByRole('button', { name: 'Open rooms' }).getAttribute('aria-expanded')).toBe('false');
  await user.click(screen.getByRole('button', { name: 'Sign out' }));
  await screen.findByRole('heading', { name: 'Welcome back.' });
  expect(session.getSnapshot().status).toBe('anonymous');
  expect(fetcher.mock.calls.filter(([path]) => path === '/auth/refresh')).toHaveLength(1);
});

it('registers using the backend contract and shows an empty room state', async () => {
  const { user } = setup(async (path, init) => {
    if (path === '/auth/refresh') return new Response('no cookie', { status: 401 });
    if (path === '/auth/register') {
      expect(JSON.parse(String(init?.body))).toEqual({ username: 'river', email: 'river@example.com', password: 'newpassword' });
      return signedIn();
    }
    return Response.json([]);
  });
  await screen.findByRole('heading', { name: 'Welcome back.' });
  await user.click(screen.getByRole('link', { name: 'Create an account' }));
  await user.type(screen.getByLabelText('Username'), 'river');
  await user.type(screen.getByLabelText('Email address'), 'river@example.com');
  await user.type(screen.getByLabelText('Password', { exact: true }), 'newpassword');
  await user.click(screen.getByRole('button', { name: 'Create account' }));
  await screen.findByRole('heading', { name: 'No rooms yet' });
});

it('shows login errors accessibly without navigating into the workspace', async () => {
  const { user, session } = setup(async () => new Response('invalid credentials', { status: 401 }));
  await screen.findByRole('heading', { name: 'Welcome back.' });
  await user.type(screen.getByLabelText('Email address'), 'you@example.com');
  await user.type(screen.getByLabelText('Password', { exact: true }), 'wrong');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  expect((await screen.findByRole('alert')).textContent).toBe('Email or password is incorrect.');
  expect(session.getSnapshot().status).toBe('anonymous');
});

it('restores a session silently and retries a failed room request', async () => {
  let fail = true;
  const { user, fetcher } = setup(async (path) => {
    if (path === '/auth/refresh') return signedIn();
    return fail ? new Response('unavailable', { status: 503 }) : Response.json(rooms);
  }, '/');
  await screen.findByRole('alert');
  fail = false;
  await user.click(screen.getByRole('button', { name: 'Try again' }));
  await screen.findByRole('button', { name: 'Engineering, Group room' });
  await waitFor(() => expect(fetcher.mock.calls.filter(([path]) => path === '/auth/refresh')).toHaveLength(1));
});

it('loads subsequent room pages without losing the first page', async () => {
  const firstPage = Array.from({ length: 50 }, (_, index) => ({ ...rooms[0], id: `room-${index}`, name: `Room ${index}` }));
  const { user } = setup(async (path) => {
    if (path === '/auth/refresh') return signedIn();
    const offset = new URL(String(path), 'https://localhost').searchParams.get('offset');
    return Response.json(offset === '0' ? firstPage : [{ ...rooms[0], id: 'last-room', name: 'Last room' }]);
  }, '/');
  await screen.findByRole('button', { name: 'Load more rooms' });
  await user.click(screen.getByRole('button', { name: 'Load more rooms' }));
  await screen.findByRole('button', { name: 'Last room, Group room' });
  expect(screen.getByRole('button', { name: 'Room 0, Group room' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Load more rooms' })).toBeNull();
});

it('keeps chats closed until selected, closes with Escape, and places search before New chat', async () => {
  const { user } = setup(async (path) => {
    if (path === '/auth/refresh') return signedIn();
    if (String(path).includes('/messages?') || String(path).startsWith('/presence?')) return Response.json([]);
    return Response.json(rooms);
  }, '/');
  const room = await screen.findByRole('button', { name: 'Engineering, Group room' });
  expect(screen.queryByRole('region', { name: 'Conversation' })).toBeNull();
  const search = screen.getByRole('searchbox', { name: 'Find a room' });
  expect(search.compareDocumentPosition(screen.getByRole('button', { name: /New chat/ })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  await user.click(room);
  expect(screen.getByRole('region', { name: 'Conversation' })).toBeTruthy();
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('region', { name: 'Conversation' })).toBeNull();
  await user.click(room);
  expect(screen.getByRole('region', { name: 'Conversation' })).toBeTruthy();
  await user.click(screen.getByRole('button', { name: 'Members' }));
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('region', { name: 'Conversation' })).toBeNull();
});

it('deletes a chat only after confirmation and clears the selected conversation', async () => {
  let deleted = false;
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
  onTestFinished(() => confirm.mockRestore());
  const { user, fetcher } = setup(async (path, init) => {
    if (path === '/auth/refresh') return signedIn();
    if (path === '/rooms/room-1' && init?.method === 'DELETE') { deleted = true; return new Response(null, { status: 204 }); }
    if (String(path).includes('/messages?') || String(path).startsWith('/presence?')) return Response.json([]);
    return Response.json(deleted ? rooms.slice(1) : rooms);
  }, '/');
  await user.click(await screen.findByRole('button', { name: 'Engineering, Group room' }));
  await user.click(screen.getByRole('button', { name: 'Delete Engineering chat' }));
  expect(fetcher.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false);
  confirm.mockReturnValue(true);
  await user.click(screen.getByRole('button', { name: 'Delete Engineering chat' }));
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Engineering, Group room' })).toBeNull());
  expect(screen.queryByRole('region', { name: 'Conversation' })).toBeNull();
});

it('confirms account deletion with a password, handles errors, and clears the session', async () => {
  let attempts = 0;
  const { user, session } = setup(async (path, init) => {
    if (path === '/auth/refresh') return signedIn();
    if (path === '/auth/account') {
      expect(init?.method).toBe('DELETE');
      expect(JSON.parse(String(init?.body))).toEqual({ password: 'password123' });
      attempts++;
      return attempts === 1 ? new Response('password is incorrect', { status: 403 }) : new Response(null, { status: 204 });
    }
    return Response.json([]);
  }, '/');
  await user.click(await screen.findByRole('button', { name: 'Delete account' }));
  expect(screen.getByRole('dialog', { name: 'Delete your account?' })).toBeTruthy();
  await user.type(screen.getByLabelText('Confirm your password'), 'password123');
  await user.click(screen.getByRole('button', { name: 'Permanently delete account' }));
  expect((await screen.findByRole('alert')).textContent).toContain('password is incorrect');
  expect(session.getSnapshot().status).toBe('authenticated');
  await user.click(screen.getByRole('button', { name: 'Permanently delete account' }));
  await screen.findByRole('heading', { name: 'Welcome back.' });
  expect(session.getSnapshot().status).toBe('anonymous');
});
