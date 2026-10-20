import { useEffect, useState } from 'react';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import { errorMessage } from '../api/http';

export function usePublicRooms(api: ApiClient, term: string) {
  const [page, setPage] = useState({ term, offset: 0 });
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState({ term: '', rooms: [] as Room[], loading: true, error: '', more: false });
  const offset = page.term === term ? page.offset : 0;
  const tooLong = new TextEncoder().encode(term).length > 100;
  useEffect(() => {
    setPage((previous) => previous.term === term ? previous : { term, offset: 0 });
    if (tooLong) return;
    const controller = new AbortController();
    setState((previous) => ({ ...previous, term, rooms: previous.term === term ? previous.rooms : [], loading: true, error: '' }));
    const timer = setTimeout(() => {
      void api.publicRooms(term, offset, controller.signal).then((rooms) => {
        if (controller.signal.aborted) return;
        setState((previous) => ({ term,
          rooms: [...new Map([...(offset === 0 ? [] : previous.rooms), ...rooms].map((room) => [room.id, room])).values()],
          loading: false, error: '', more: rooms.length === 20,
        }));
      }).catch((error: unknown) => {
        if (!controller.signal.aborted) setState((previous) => ({ ...previous, loading: false, error: errorMessage(error) }));
      });
    }, offset === 0 ? 250 : 0);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [api, term, offset, attempt, tooLong]);
  return { ...(state.term === term ? state : { rooms: [], loading: true, error: '', more: false }), tooLong,
    retry: () => setAttempt((value) => value + 1), loadMore: () => { setState((previous) => ({ ...previous, loading: true })); setPage({ term, offset: offset + 20 }); } };
}
