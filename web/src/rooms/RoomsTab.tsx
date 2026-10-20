import { useState } from 'react';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import type { OpenConversation } from './NewChatModal';
import { CreateRoomForm } from './CreateRoomForm';
import { usePublicRooms } from './usePublicRooms';

export function RoomsTab({ api, rooms, busy, run }: { api: ApiClient; rooms: Room[]; busy: boolean; run: OpenConversation }) {
  const [query, setQuery] = useState('');
  const browse = usePublicRooms(api, query.trim());
  const joined = new Set(rooms.map((room) => room.id));
  return <div>
    <CreateRoomForm api={api} busy={busy} run={run} />
    <section className="browse-rooms" aria-label="Public rooms"><h3>Browse public rooms</h3>
      <label htmlFor="public-room-search">Search public rooms</label>
      <input id="public-room-search" type="search" value={query} maxLength={100} onChange={(event) => setQuery(event.target.value)} disabled={busy} />
      {browse.tooLong ? <p role="alert">Please shorten your room search.</p> : <>
        {browse.loading && <p role="status">Loading public rooms…</p>}
        {browse.error && <div><p role="alert">{browse.error}</p><button className="text-button" onClick={browse.retry}>Retry public rooms</button></div>}
        {!browse.loading && !browse.error && !browse.rooms.length && <p role="status">No public rooms found. Create one above or try another search.</p>}
        <ul className="public-room-results">{browse.rooms.map((room) => <li key={room.id}>
          <span>{room.name || 'Untitled room'}</span>
          <button type="button" disabled={busy} aria-label={`${joined.has(room.id) ? 'Open' : 'Join'} ${room.name}`} onClick={() => run(async (signal) => {
            if (!joined.has(room.id)) await api.joinRoom(room.id, signal);
            return room;
          })}>{joined.has(room.id) ? 'Open' : 'Join'}</button>
        </li>)}</ul>
        {browse.more && !browse.error && <button type="button" className="text-button" disabled={browse.loading || busy} onClick={browse.loadMore}>Load more public rooms</button>}
      </>}
    </section>
  </div>;
}
