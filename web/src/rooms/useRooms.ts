import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiClient } from '../api/client';
import { errorMessage } from '../api/http';
import type { Room } from '../api/types';

const pageSize = 50;

export function useRooms(api: ApiClient) {
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const created = useRef(new Map<string, Room>());
  const removed = useRef(new Map<string, number>());
  const mutation = useRef(0);
  const [state, setState] = useState({ rooms: [] as Room[], loading: true, error: '', more: false });
  useEffect(() => {
    const controller = new AbortController();
    const startedAt = mutation.current;
    setState((previous) => ({ ...previous, loading: true, error: '' }));
    void api.listRooms({ limit: pageSize, offset, signal: controller.signal }).then((rooms) => {
      if (controller.signal.aborted) return;
      setState((previous) => ({
        rooms: [...new Map([...created.current.values(), ...(offset === 0 ? rooms : [...previous.rooms, ...rooms])].map((room) => [room.id, room])).values()].filter((room) => (removed.current.get(room.id) ?? -1) <= startedAt),
        more: rooms.length === pageSize, loading: false, error: '',
      }));
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setState((previous) => ({ ...previous, loading: false, error: errorMessage(error) }));
    });
    return () => controller.abort();
  }, [api, offset, revision]);
  const addRoom = useCallback((room: Room) => {
    removed.current.delete(room.id);
    created.current.set(room.id, room);
    setState((previous) => ({ ...previous, error: '', rooms: [room, ...previous.rooms.filter((item) => item.id !== room.id)] }));
  }, []);
  const removeRoom = useCallback((id: string) => {
    removed.current.set(id, ++mutation.current); created.current.delete(id);
    setState((previous) => ({ ...previous, rooms: previous.rooms.filter((room) => room.id !== id) }));
    setOffset(0); setRevision((value) => value + 1);
  }, []);
  return {
    ...state, addRoom, removeRoom,
    retry: () => setRevision((value) => value + 1),
    reload: () => { setOffset(0); setRevision((value) => value + 1); },
    loadMore: () => setOffset((value) => value + pageSize),
  };
}
