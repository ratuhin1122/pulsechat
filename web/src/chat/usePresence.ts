import { useEffect, useMemo, useState } from 'react';
import type { ApiClient } from '../api/client';
import type { Presence, PresenceChange } from './types';

export function usePresence(api: Pick<ApiClient, 'presence'>, roomID: string, epoch: number,
  events: Record<string, PresenceChange>, connected: boolean) {
  const [snapshot, setSnapshot] = useState<{ members: Presence[]; revision: number; error: boolean; loading: boolean }>({ members: [], revision: 0, error: false, loading: true });
  // Fetch on room entry and reconnect; live transitions newer than the request
  // take precedence even when the snapshot response arrives later.
  const revision = Object.values(events).reduce((maximum, event) => Math.max(maximum, event.revision), 0);
  useEffect(() => {
    const controller = new AbortController();
    const atStart = revision;
    setSnapshot((current) => ({ ...current, loading: true }));
    void api.presence(roomID, controller.signal).then((members) => {
      if (!controller.signal.aborted) setSnapshot({ members, revision: atStart, error: false, loading: false });
    }).catch(() => {
      if (!controller.signal.aborted) setSnapshot({ members: [], revision: atStart, error: true, loading: false });
    });
    return () => controller.abort();
    // A transition is merged below; it must not cause a new snapshot request.
  }, [api, roomID, epoch]);

  const online = useMemo(() => {
    const result: Record<string, boolean | undefined> = {};
    for (const member of snapshot.members) result[member.user_id] = connected ? member.online : undefined;
    for (const [id, event] of Object.entries(events)) {
      if (connected && event.revision > snapshot.revision) result[id] = event.online;
    }
    return result;
  }, [snapshot, events, connected]);
  return { members: snapshot.members.map((member) => member.user_id), online, error: snapshot.error, loading: snapshot.loading };
}
