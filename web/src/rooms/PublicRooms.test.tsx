import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import { ApiError } from '../api/http';
import { RoomsTab } from './RoomsTab';
import { DirectMessageTab } from './DirectMessageTab';

afterEach(cleanup);
const room = (id: number): Room => ({ id: String(id), type: 'group', name: `Room ${id}`, is_public: true, created_at: '2026-10-19T00:00:00Z' });

it('paginates public rooms and resets to page zero when searches change and return', async () => {
  const publicRooms = vi.fn(async (q: string, offset: number) => q ? [room(100)] : offset === 0 ? Array.from({ length: 20 }, (_, i) => room(i)) : [room(20)]);
  render(<RoomsTab api={{ publicRooms } as unknown as ApiClient} rooms={[]} busy={false} run={vi.fn()} />);
  fireEvent.click(await screen.findByRole('button', { name: 'Load more public rooms' }));
  await screen.findByText('Room 20'); expect(publicRooms).toHaveBeenLastCalledWith('', 20, expect.any(AbortSignal));
  fireEvent.change(screen.getByLabelText('Search public rooms'), { target: { value: 'music' } });
  await screen.findByText('Room 100'); expect(publicRooms).toHaveBeenLastCalledWith('music', 0, expect.any(AbortSignal));
  expect(screen.queryByText('Room 20')).toBeNull();
  fireEvent.change(screen.getByLabelText('Search public rooms'), { target: { value: '' } });
  await screen.findByText('Room 0'); expect(publicRooms).toHaveBeenLastCalledWith('', 0, expect.any(AbortSignal));
});

it('shows browse loading/error states and retries without losing the form', async () => {
  const publicRooms = vi.fn().mockRejectedValueOnce(new ApiError(503, 'Search unavailable')).mockResolvedValue([]);
  render(<RoomsTab api={{ publicRooms } as unknown as ApiClient} rooms={[]} busy={false} run={vi.fn()} />);
  expect(screen.getByText('Loading public rooms…')).toBeTruthy();
  fireEvent.change(screen.getByLabelText('Room name'), { target: { value: 'My draft' } });
  expect((await screen.findByRole('alert')).textContent).toBe('Search unavailable');
  fireEvent.click(screen.getByRole('button', { name: 'Retry public rooms' }));
  await screen.findByText(/No public rooms found/); expect((screen.getByLabelText('Room name') as HTMLInputElement).value).toBe('My draft');
});

it('retries failed user searches and debounces intermediate queries', async () => {
  const searchUsers = vi.fn().mockRejectedValueOnce(new ApiError(500, 'User search failed')).mockResolvedValue([]);
  render(<DirectMessageTab api={{ searchUsers } as unknown as ApiClient} busy={false} run={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Search users'), { target: { value: 'al' } });
  fireEvent.change(screen.getByLabelText('Search users'), { target: { value: 'alice' } });
  await screen.findByRole('button', { name: 'Retry user search' });
  expect(searchUsers).toHaveBeenCalledExactlyOnceWith('alice', expect.any(AbortSignal));
  fireEvent.click(screen.getByRole('button', { name: 'Retry user search' })); await screen.findByText(/No matching users/);
});

it('does not replace a newer public search with a stale page', async () => {
  let resolveOld!: (rooms: Room[]) => void;
  const publicRooms = vi.fn((q: string) => q === '' ? new Promise((resolve) => { resolveOld = resolve; }) : Promise.resolve([room(9)]));
  render(<RoomsTab api={{ publicRooms } as unknown as ApiClient} rooms={[]} busy={false} run={vi.fn()} />);
  await waitFor(() => expect(publicRooms).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText('Search public rooms'), { target: { value: 'new' } });
  await screen.findByText('Room 9'); await act(async () => resolveOld([room(1)]));
  expect(screen.queryByText('Room 1')).toBeNull();
});
