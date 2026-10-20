import { useEffect, useRef, useState } from 'react';
import type { ApiClient } from '../api/client';
import type { Room, RoomMember } from '../api/types';
import { errorMessage } from '../api/http';

export function MembersPanel({ api, room, onClose, onLeave, leaving, leaveDisabled, leaveError }: {
  api: ApiClient; room: Room; onClose: () => void; onLeave: () => void; leaving: boolean; leaveDisabled: boolean; leaveError: string;
}) {
  const close = useRef<HTMLButtonElement>(null);
  const [offset, setOffset] = useState(0);
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState({ members: [] as RoomMember[], loading: true, error: '', more: false });
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null; close.current?.focus();
    return () => { if (previous?.isConnected) previous.focus(); };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setState((previous) => ({ ...previous, loading: true, error: '' }));
    void api.members(room.id, offset, controller.signal).then((members) => {
      if (!controller.signal.aborted) setState((previous) => ({
        members: [...new Map([...(offset === 0 ? [] : previous.members), ...members].map((member) => [member.user_id, member])).values()],
        loading: false, error: '', more: members.length === 50,
      }));
    }).catch((error: unknown) => { if (!controller.signal.aborted) setState((previous) => ({ ...previous, loading: false, error: errorMessage(error) })); });
    return () => controller.abort();
  }, [api, room.id, offset, attempt]);
  return <aside className="members-panel" id="room-members-panel" aria-label="Room members" onKeyDown={(event) => {
    if (event.key === 'Escape') onClose();
  }}>
    <header><div><h2>Members</h2><p>{room.name || 'Conversation'}</p></div>
      <button type="button" ref={close} className="text-button" aria-label="Close members" onClick={onClose}>Close</button>
    </header>
    <div className="members-list" aria-busy={state.loading}>
      {state.loading && <p role="status">Loading members…</p>}
      {state.error && <div><p role="alert">{state.error}</p><button className="text-button" onClick={() => setAttempt((value) => value + 1)}>Retry members</button></div>}
      {!state.loading && !state.error && !state.members.length && <p>No members to display.</p>}
      <ul>{state.members.map((member) => <li key={member.user_id}>
        <strong title={member.user_id}>User {member.user_id.slice(0, 8)}</strong>
        <span>Joined {new Date(member.joined_at).toLocaleDateString()}</span>
      </li>)}</ul>
      {state.more && !state.error && <button className="text-button" disabled={state.loading} onClick={() => {
        setState((previous) => ({ ...previous, loading: true })); setOffset((value) => value + 50);
      }}>Load more members</button>}
    </div>
    {room.type === 'group' && <footer>
      <button type="button" className="leave-room-button" disabled={leaveDisabled} onClick={onLeave}>{leaving ? 'Leaving…' : 'Leave room'}</button>
      {leaveError && <p role="alert" className="error-message">{leaveError}</p>}
    </footer>}
  </aside>;
}
