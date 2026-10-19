import { useEffect, useState } from 'react';
import type { ApiClient } from '../api/client';
import { errorMessage } from '../api/http';
import type { Room } from '../api/types';

const pageSize = 50;

export function useRooms(api: ApiClient) {
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState({ rooms: [] as Room[], loading: true, error: '', more: false });
  useEffect(() => {
    const controller = new AbortController();
    setState((previous) => ({ ...previous, loading: true, error: '' }));
    void api.listRooms({ limit: pageSize, offset, signal: controller.signal }).then((rooms) => {
      if (controller.signal.aborted) return;
      setState((previous) => ({
        rooms: offset === 0 ? rooms : [...new Map([...previous.rooms, ...rooms].map((room) => [room.id, room])).values()],
        more: rooms.length === pageSize, loading: false, error: '',
      }));
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setState((previous) => ({ ...previous, loading: false, error: errorMessage(error) }));
    });
    return () => controller.abort();
  }, [api, offset, revision]);
  return {
    ...state,
    retry: () => setRevision((value) => value + 1),
    reload: () => { setOffset(0); setRevision((value) => value + 1); },
    loadMore: () => setOffset((value) => value + pageSize),
  };
}
