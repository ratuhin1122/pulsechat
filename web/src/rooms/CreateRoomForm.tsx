import { useState } from 'react';
import type { ApiClient } from '../api/client';
import type { OpenConversation } from './NewChatModal';

export function CreateRoomForm({ api, busy, run }: { api: ApiClient; busy: boolean; run: OpenConversation }) {
  const [name, setName] = useState('');
  const [isPublic, setPublic] = useState(false);
  const bytes = new TextEncoder().encode(name.trim()).length;
  return <form className="create-room-form" aria-label="Create room" onSubmit={(event) => {
    event.preventDefault(); if (bytes > 0 && bytes <= 100) run((signal) => api.createGroup(name.trim(), signal, isPublic));
  }}>
    <h3>Create a room</h3>
    <label htmlFor="new-room-name">Room name</label>
    <input id="new-room-name" value={name} onChange={(event) => setName(event.target.value)} disabled={busy} maxLength={100} required />
    <label className="public-toggle"><input type="checkbox" checked={isPublic} disabled={busy} onChange={(event) => setPublic(event.target.checked)} />Public room</label>
    <p className="modal-note">{isPublic ? 'Anyone can find this room in public search.' : 'This room will not appear in public search.'}</p>
    {bytes > 100 && <p role="alert">Please choose a shorter room name.</p>}
    <button className="primary-button" type="submit" disabled={busy || !bytes || bytes > 100}>Create room</button>
  </form>;
}
