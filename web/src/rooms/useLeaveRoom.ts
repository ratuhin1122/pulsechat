import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiClient } from '../api/client';
import { errorMessage } from '../api/http';

export function useLeaveRoom(api: ApiClient, onLeft: (id: string) => void) {
  const flight = useRef<AbortController | null>(null);
  const [state, setState] = useState({ pendingID: '', errorID: '', error: '' });
  useEffect(() => () => flight.current?.abort(), []);
  const leave = useCallback((id: string) => {
    if (flight.current) return;
    const controller = new AbortController(); flight.current = controller;
    setState({ pendingID: id, errorID: '', error: '' });
    void api.leaveRoom(id, controller.signal).then(() => { if (!controller.signal.aborted) onLeft(id); })
      .catch((error: unknown) => { if (!controller.signal.aborted) setState({ pendingID: '', errorID: id, error: errorMessage(error) }); })
      .finally(() => {
        if (!controller.signal.aborted) { flight.current = null; setState((previous) => ({ ...previous, pendingID: '' })); }
      });
  }, [api, onLeft]);
  return { ...state, leave };
}
