import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ApiClient } from '../api/client';
import { errorMessage } from '../api/http';
import { mergeMessages } from './types';
import type { Message } from './types';

export function useHistory(api: Pick<ApiClient, 'history'>, roomID: string, live: Message[], epoch: number, seed: (message: Message) => void) {
  const [history, setHistory] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const [initialized, setInitialized] = useState(false);
  const [more, setMore] = useState(false);
  const [error, setError] = useState('');
  const cursor = useRef<string | undefined>(undefined);
  const flight = useRef<AbortController | null>(null);
  const failed = useRef<'latest' | 'older'>('latest');
  const previousEpoch = useRef(epoch);
  const active = useRef(false);
  const pendingHead = useRef(false);

  const load = useCallback(async (kind: 'latest' | 'older'): Promise<void> => {
    if (!active.current) return;
    if (flight.current) { if (kind === 'latest') pendingHead.current = true; return; }
    const controller = new AbortController();
    flight.current = controller; failed.current = kind;
    setLoading(true); setError('');
    try {
      const page = await api.history(roomID, { before: kind === 'older' ? cursor.current : undefined, signal: controller.signal });
      if (controller.signal.aborted) return;
      setHistory((current) => mergeMessages(page, current));
      // A fresh head page after reconnect must not skip the older-page boundary.
      if (kind === 'older' || !cursor.current) {
        cursor.current = page.at(-1)?.id;
        setMore(page.length === 50);
      }
      setInitialized(true);
      if (page[0]) seed(page[0]);
    } catch (error) { if (!controller.signal.aborted) setError(errorMessage(error)); }
    finally {
      if (flight.current === controller) {
        flight.current = null;
        if (!controller.signal.aborted) {
          setLoading(false);
          if (pendingHead.current && active.current) { pendingHead.current = false; void load('latest'); }
        }
      }
    }
  }, [api, roomID, seed]);

  useEffect(() => {
    active.current = true;
    void load('latest');
    return () => { active.current = false; pendingHead.current = false; flight.current?.abort(); flight.current = null; };
  }, [load]);
  useEffect(() => {
    if (epoch > previousEpoch.current && previousEpoch.current > 0) void load('latest');
    previousEpoch.current = epoch;
  }, [epoch, load]);

  const messages = useMemo(() => mergeMessages(history, live), [history, live]);
  return { messages, loading, initialized, more, error,
    loadOlder: () => load('older'), retry: () => load(failed.current) };
}
