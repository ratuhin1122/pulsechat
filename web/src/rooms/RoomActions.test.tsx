import { afterEach, beforeEach, expect, it, onTestFinished, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { HttpClient } from '../api/http';
import { ApiClient } from '../api/client';
import { Session } from '../auth/session';
import { AuthProvider } from '../auth/AuthProvider';
import { Sidebar } from './Sidebar';
import { useRooms } from './useRooms';
import type { Room } from '../api/types';
import { RoomActions } from './RoomActions';

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (this: HTMLDialogElement) { this.setAttribute('open', ''); });
  HTMLDialogElement.prototype.close = vi.fn(function (this: HTMLDialogElement) { this.removeAttribute('open'); });
});
afterEach(() => cleanup());
const group: Room = { id: 'group-1', type: 'group', name: 'Study group', is_public: true, created_at: '2026-10-19T00:00:00Z' };
const direct: Room = { ...group, id: 'dm-1', type: 'dm', name: 'alice', is_public: false };

async function setup(handler?: (path: string, init?: RequestInit) => Promise<Response>) {
  const fetcher = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (String(path) === '/auth/refresh') return Response.json({ access_token: 'token', token_type: 'Bearer', expires_in: 900 });
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer token');
    return handler ? handler(String(path), init) : Response.json([]);
  });
  const http = new HttpClient(fetcher); const session = new Session(http); onTestFinished(() => session.dispose());
  const api = new ApiClient(http, session); const select = vi.fn();
  function Harness() { const store = useRooms(api); return <Sidebar open selected={null} onSelect={select} store={store} />; }
  render(<MemoryRouter><AuthProvider services={{ session, api }}><Harness /></AuthProvider></MemoryRouter>);
  await waitFor(() => expect((screen.getByRole('button', { name: /New chat/ }) as HTMLButtonElement).disabled).toBe(false));
  return { fetcher, select, user: userEvent.setup() };
}

it.each([false, true])('creates and selects a room with public=%s, retaining it after refresh', async (isPublic) => {
  const { select, user } = await setup(async (path, init) => {
    if (path === '/rooms' && init?.method === 'POST') {
      expect(JSON.parse(String(init.body))).toEqual({ name: 'Study group', is_public: isPublic }); return Response.json({ ...group, is_public: isPublic }, { status: 201 });
    }
    return Response.json([]);
  });
  await user.click(screen.getByRole('button', { name: /New chat/ }));
  await user.click(screen.getByRole('tab', { name: 'Rooms' }));
  await user.type(screen.getByLabelText('Room name'), 'Study group');
  if (isPublic) await user.click(screen.getByLabelText('Public room'));
  await user.click(screen.getByRole('button', { name: 'Create room' }));
  await waitFor(() => expect(select).toHaveBeenCalledWith({ ...group, is_public: isPublic }));
  expect(screen.queryByRole('dialog')).toBeNull();
  await user.click(screen.getByRole('button', { name: 'Refresh rooms' }));
  await waitFor(() => expect((screen.getByRole('button', { name: 'Refresh rooms' }) as HTMLButtonElement).disabled).toBe(false));
  expect(within(screen.getByRole('region', { name: 'Group rooms' })).getByRole('button', { name: 'Study group, Group room' })).toBeTruthy();
});

it('searches users via q, shows display names, and opens the DM', async () => {
  const { user, select, fetcher } = await setup(async (path, init) => {
    if (path === '/users?q=ali') return Response.json([{ id: 'user-2', username: 'alice', display_name: 'Alice River' }]);
    if (path === '/rooms/dm') { expect(JSON.parse(String(init?.body))).toEqual({ user_id: 'user-2' }); return Response.json(direct); }
    return Response.json([]);
  });
  await user.click(screen.getByRole('button', { name: /New chat/ }));
  await user.type(screen.getByLabelText('Search users'), 'a');
  expect(fetcher.mock.calls.some(([path]) => String(path).startsWith('/users'))).toBe(false);
  await user.type(screen.getByLabelText('Search users'), 'li');
  await screen.findByText('Alice River');
  await user.click(screen.getByRole('button', { name: 'Start chat with alice' }));
  await waitFor(() => expect(select).toHaveBeenCalledWith(direct));
  expect(within(screen.getByRole('region', { name: 'Direct messages' })).getByRole('button', { name: 'alice, Direct conversation' })).toBeTruthy();
});

it('shows empty results and creation errors while preserving the room form', async () => {
  const { user, select } = await setup(async (_path, init) => init?.method === 'POST' ? new Response('name rejected', { status: 400 }) : Response.json([]));
  await user.click(screen.getByRole('button', { name: /New chat/ }));
  await user.type(screen.getByLabelText('Search users'), 'nobody'); await screen.findByText(/No matching users/);
  await user.click(screen.getByRole('tab', { name: 'Rooms' }));
  await screen.findByText(/No public rooms found/);
  await user.type(screen.getByLabelText('Room name'), 'Study group');
  await user.click(screen.getByRole('button', { name: 'Create room' }));
  expect((await screen.findByRole('alert')).textContent).toBe('name rejected');
  expect((screen.getByLabelText('Room name') as HTMLInputElement).value).toBe('Study group'); expect(select).not.toHaveBeenCalled();
});

it('joins a browsed public room and opens it only after successful membership', async () => {
  let succeed = false;
  const { user, select } = await setup(async (path) => {
    if (path.startsWith('/rooms/public?')) return Response.json([group]);
    if (path === '/rooms/group-1/join') return succeed ? new Response(null, { status: 204 }) : new Response('Join failed', { status: 403 });
    return Response.json([]);
  });
  await user.click(screen.getByRole('button', { name: /New chat/ })); await user.click(screen.getByRole('tab', { name: 'Rooms' }));
  await user.click(await screen.findByRole('button', { name: 'Join Study group' }));
  expect((await screen.findByRole('alert')).textContent).toBe('Join failed'); expect(select).not.toHaveBeenCalled();
  succeed = true; await user.click(screen.getByRole('button', { name: 'Join Study group' }));
  await waitFor(() => expect(select).toHaveBeenCalledWith(group));
});

it('opens an already joined public room without a join POST', async () => {
  const { user, select, fetcher } = await setup(async (path) => Response.json(path.startsWith('/rooms/public?') || path.startsWith('/rooms?') ? [group] : []));
  await user.click(screen.getByRole('button', { name: /New chat/ })); await user.click(screen.getByRole('tab', { name: 'Rooms' }));
  await user.click(await screen.findByRole('button', { name: 'Open Study group' }));
  expect(select).toHaveBeenCalledWith(group); expect(fetcher.mock.calls.some(([path]) => String(path).endsWith('/join'))).toBe(false);
});

it('ignores stale user results when the query changes and cancels searches on close', async () => {
  let finish!: (users: { id: string; username: string }[]) => void;
  let firstSignal!: AbortSignal;
  const searchUsers = vi.fn((term: string, signal: AbortSignal) => {
    if (term === 'al') { firstSignal = signal; return new Promise((resolve) => { finish = resolve; }); }
    return Promise.resolve([{ id: 'b', username: 'bob' }]);
  });
  render(<RoomActions api={{ searchUsers } as unknown as ApiClient} disabled={false} onCreated={vi.fn()} />);
  fireEvent.click(screen.getByRole('button', { name: /New chat/ }));
  fireEvent.change(screen.getByLabelText('Search users'), { target: { value: 'al' } });
  await waitFor(() => expect(searchUsers).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText('Search users'), { target: { value: 'bo' } });
  await screen.findByRole('button', { name: 'Start chat with bob' });
  await act(async () => finish([{ id: 'a', username: 'alice' }]));
  expect(firstSignal.aborted).toBe(true); expect(screen.queryByRole('button', { name: 'Start chat with alice' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Close new chat' })); expect(screen.queryByRole('dialog')).toBeNull();
});

it('supports tab keyboard navigation and returns focus when dismissed', async () => {
  const { user } = await setup(); const trigger = screen.getByRole('button', { name: /New chat/ });
  await user.click(trigger);
  const directTab = screen.getByRole('tab', { name: 'Direct message' }); directTab.focus();
  await user.keyboard('{ArrowRight}'); expect(screen.getByRole('tab', { name: 'Rooms' }).getAttribute('aria-selected')).toBe('true');
  await user.keyboard('{Home}'); expect(directTab.getAttribute('aria-selected')).toBe('true');
  fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }));
  expect(screen.queryByRole('dialog')).toBeNull(); expect(document.activeElement).toBe(trigger);
});

it.each([0, 5, 120])('shows message previews and unread badge for count %s', async (count) => {
  await setup(async () => Response.json([{ ...group, unread_count: count,
    last_message: { id: 'last', sender_id: 'other', type: 'text', body: 'Latest message preview', created_at: group.created_at } }]));
  expect(screen.getByText('Latest message preview')).toBeTruthy();
  if (count) expect(screen.getByLabelText(`${count} unread messages`).textContent).toBe(count > 99 ? '99+' : String(count));
  else expect(screen.queryByLabelText('0 unread messages')).toBeNull();
});
