import { afterEach, expect, it, onTestFinished, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import { ApiClient as Client } from '../api/client';
import { HttpClient } from '../api/http';
import { AuthProvider } from '../auth/AuthProvider';
import { Session } from '../auth/session';
import { Workspace } from '../pages/Workspace';
import { TestSocket } from '../chat/testSocket';
import { MembersPanel } from './MembersPanel';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const group: Room = { id: 'group-1', type: 'group', name: 'Study group', created_at: '2026-10-19T00:00:00Z' };
const member = (i: number) => ({ user_id: `member-${i}`, joined_at: '2026-10-19T00:00:00Z' });
const props = { room: group, onClose: vi.fn(), onLeave: vi.fn(), leaving: false, leaveDisabled: false, leaveError: '' };

it('loads paginated members and offers leave only for a group', async () => {
  const members = vi.fn(async (_id: string, offset: number) => offset === 0 ? Array.from({ length: 50 }, (_, i) => member(i)) : [member(50)]);
  const view = render(<MembersPanel {...props} api={{ members } as unknown as ApiClient} />);
  expect(screen.getByText('Loading members…')).toBeTruthy();
  fireEvent.click(await screen.findByRole('button', { name: 'Load more members' }));
  await screen.findByTitle('member-50'); expect(members).toHaveBeenLastCalledWith('group-1', 50, expect.any(AbortSignal));
  expect(screen.getByRole('button', { name: 'Leave room' })).toBeTruthy();
  view.rerender(<MembersPanel {...props} api={{ members } as unknown as ApiClient} room={{ ...group, type: 'dm' }} />);
  expect(screen.queryByRole('button', { name: 'Leave room' })).toBeNull();
});

it('handles member errors, retry, empty results, and aborts on unmount', async () => {
  const members = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue([]);
  const view = render(<MembersPanel {...props} api={{ members } as unknown as ApiClient} />);
  fireEvent.click(await screen.findByRole('button', { name: 'Retry members' }));
  await screen.findByText('No members to display.');
  const signal = members.mock.calls.at(-1)![2] as AbortSignal;
  view.unmount(); expect(signal.aborted).toBe(true);
});

it('leaves a selected room after confirmation from the API and removes it from the sidebar', async () => {
  vi.stubGlobal('WebSocket', TestSocket); TestSocket.instances = [];
  let left = false; let success = false;
  const fetcher = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(path), 'http://chat.test');
    if (url.pathname === '/auth/refresh') return Response.json({ access_token: 'token', token_type: 'Bearer', expires_in: 900 });
    if (url.pathname === '/rooms') return Response.json(left ? [] : [group]);
    if (url.pathname.endsWith('/leave')) {
      expect(init?.method).toBe('POST'); expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer token');
      if (!success) return new Response('Could not leave this room', { status: 403 });
      left = true; return new Response(null, { status: 204 });
    }
    if (url.pathname.endsWith('/members')) return Response.json([member(1)]);
    return Response.json([]);
  });
  const http = new HttpClient(fetcher); const session = new Session(http); onTestFinished(() => session.dispose());
  render(<MemoryRouter><AuthProvider services={{ session, api: new Client(http, session) }}><Workspace /></AuthProvider></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: 'Study group, Group room' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Message' }), { target: { value: 'Keep this draft' } });
  fireEvent.click(screen.getByRole('button', { name: 'Members' }));
  await screen.findByTitle('member-1'); fireEvent.click(screen.getByRole('button', { name: 'Leave room' }));
  expect((await screen.findByRole('alert')).textContent).toBe('Could not leave this room');
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Study group');
  expect((screen.getByRole('textbox', { name: 'Message' }) as HTMLTextAreaElement).value).toBe('Keep this draft');
  success = true; fireEvent.click(screen.getByRole('button', { name: 'Leave room' }));
  await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Room members' })).toBeNull());
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Your space');
  expect(within(screen.getByRole('navigation', { name: 'Room list' })).queryByRole('button', { name: 'Study group, Group room' })).toBeNull();
});
