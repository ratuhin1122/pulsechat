import { afterEach, beforeEach, expect, it, onTestFinished, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { ApiClient } from '../api/client';
import { HttpClient } from '../api/http';
import { AuthProvider } from '../auth/AuthProvider';
import { Session } from '../auth/session';
import { App } from '../App';
import { sampleMessage, TestSocket } from './testSocket';
import type { Message } from './types';

beforeEach(() => { TestSocket.instances = []; vi.stubGlobal('WebSocket', TestSocket); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function message(number: number): Message {
  return { ...sampleMessage(`00000000-0000-4000-8000-${String(number).padStart(12, '0')}`,
    new Date(Date.UTC(2026, 9, 19, 12, 0, number)).toISOString()), body: `Message ${number}` };
}

async function setup(history: (before: string | null) => Promise<Message[]> = async () => [message(2)]) {
  const fetcher = vi.fn(async (path: RequestInfo | URL) => {
    const url = new URL(String(path), 'https://chat.test');
    if (url.pathname === '/auth/refresh') return Response.json({ access_token: 'token', token_type: 'Bearer', expires_in: 900 });
    if (url.pathname === '/rooms') return Response.json([{ id: 'room-1', type: 'group', name: 'Engineering', created_at: '2026-10-19T00:00:00Z' }]);
    if (url.pathname === '/presence') return Response.json([{ user_id: 'user-1', online: true }]);
    return Response.json(await history(url.searchParams.get('before')));
  });
  const http = new HttpClient(fetcher);
  const session = new Session(http); onTestFinished(() => session.dispose());
  render(<MemoryRouter><AuthProvider services={{ session, api: new ApiClient(http, session) }}><App /></AuthProvider></MemoryRouter>);
  const user = userEvent.setup();
  await user.click(await screen.findByRole('button', { name: 'Engineering, Group room' }));
  await waitFor(() => expect(TestSocket.instances).toHaveLength(1));
  act(() => TestSocket.instances[0].open());
  return { user, socket: TestSocket.instances[0], fetcher };
}

it('renders timestamped history, sends text, merges replay duplicates, and updates presence dots', async () => {
  const { user, socket } = await setup();
  await screen.findByText('Message 2');
  expect(screen.getByText('Message 2').closest('article')?.querySelector('time')?.dateTime).toBe(message(2).created_at);
  const log = within(screen.getByRole('log', { name: 'Messages' }));
  await waitFor(() => expect(log.getByRole('img', { name: 'Online' })).toBeTruthy());
  const draft = screen.getByRole('textbox', { name: 'Message' });
  await user.type(draft, 'A new message'); await user.click(screen.getByRole('button', { name: 'Send message' }));
  expect(JSON.parse(socket.sent[0])).toEqual({ type: 'message', room_id: 'room-1', body: 'A new message' });
  expect((draft as HTMLTextAreaElement).value).toBe('');
  act(() => {
    socket.receive({ ...message(2), type: 'message', message_type: 'text' });
    socket.receive({ ...message(3), type: 'message', message_type: 'text' });
    socket.receive({ type: 'presence', user_id: 'user-1', online: false });
  });
  await screen.findByText('Message 3'); expect(screen.getAllByText('Message 2')).toHaveLength(1);
  expect(log.getAllByRole('img', { name: 'Offline' })).toHaveLength(2);
  await user.type(draft, 'Keep this draft'); act(() => socket.drop());
  expect((draft as HTMLTextAreaElement).value).toBe('Keep this draft');
  expect((screen.getByRole('button', { name: 'Send message' }) as HTMLButtonElement).disabled).toBe(true);
});

it('loads older history on scroll up and preserves the viewport position', async () => {
  const latest = Array.from({ length: 50 }, (_, index) => message(51 - index));
  let resolveOlder!: (messages: Message[]) => void;
  const requested: (string | null)[] = [];
  await setup(async (before) => {
    requested.push(before);
    if (!before) return latest;
    return new Promise((resolve) => { resolveOlder = resolve; });
  });
  await screen.findByText('Message 51');
  const log = screen.getByRole('log', { name: 'Messages' });
  let height = 1000;
  Object.defineProperty(log, 'scrollHeight', { configurable: true, get: () => height });
  Object.defineProperty(log, 'clientHeight', { configurable: true, value: 200 });
  log.scrollTop = 20;
  fireEvent.scroll(log);
  await waitFor(() => expect(requested.at(-1)).toBe(message(2).id));
  height = 1400;
  await act(async () => resolveOlder([message(1)]));
  await screen.findByText('Message 1');
  expect(log.scrollTop).toBe(420);
  expect(log.querySelector('article')?.getAttribute('data-message-id')).toBe(message(1).id);
});

it('retains a draft when the socket cannot accept it and supports Shift+Enter', async () => {
  const { user, socket } = await setup(); await screen.findByText('Message 2');
  const draft = screen.getByRole('textbox', { name: 'Message' });
  await user.type(draft, 'First'); await user.keyboard('{Shift>}{Enter}{/Shift}second');
  expect((draft as HTMLTextAreaElement).value).toBe('First\nsecond');
  socket.bufferedAmount = 100_000;
  await user.click(screen.getByRole('button', { name: 'Send message' }));
  expect((await screen.findByRole('alert')).textContent).toContain('draft is still here');
  expect((draft as HTMLTextAreaElement).value).toBe('First\nsecond');
});
