import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { usePresence } from './usePresence';
import type { Presence, PresenceChange } from './types';

afterEach(cleanup);

it('keeps a live transition newer than an in-flight presence snapshot', async () => {
  let resolve!: (members: Presence[]) => void;
  const api = { presence: vi.fn(() => new Promise<Presence[]>((done) => { resolve = done; })) };
  const initial: Record<string, PresenceChange> = {};
  const { result, rerender } = renderHook(({ events, connected }) => usePresence(api, 'room-1', 1, events, connected),
    { initialProps: { events: initial, connected: true } });
  rerender({ events: { 'user-1': { online: true, revision: 1 } }, connected: true });
  await act(async () => resolve([{ user_id: 'user-1', online: false }]));
  expect(result.current.online['user-1']).toBe(true);
  expect(api.presence).toHaveBeenCalledTimes(1);
  rerender({ events: { 'user-1': { online: false, revision: 2 } }, connected: true });
  expect(result.current.online['user-1']).toBe(false);
  rerender({ events: { 'user-1': { online: false, revision: 2 } }, connected: false });
  expect(result.current.online['user-1']).toBeUndefined();
});

it('refreshes snapshots after reconnect and aborts stale requests on unmount', async () => {
  const api = { presence: vi.fn(async () => [{ user_id: 'user-1', online: false }]) };
  const { result, rerender, unmount } = renderHook(({ epoch }) => usePresence(api, 'room-1', epoch, {}, true), { initialProps: { epoch: 1 } });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ epoch: 2 });
  await waitFor(() => expect(api.presence).toHaveBeenCalledTimes(2));
  unmount();
  expect((api.presence.mock.calls.at(-1) as unknown as [string, AbortSignal])[1].aborted).toBe(true);
});
