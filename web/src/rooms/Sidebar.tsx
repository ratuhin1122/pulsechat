import { useState } from 'react';
import type { Room } from '../api/types';
import { errorMessage } from '../api/http';
import { useAuth } from '../auth/AuthProvider';
import { Brand } from '../components/Brand';
import { Icon } from '../components/Icon';
import { useRooms } from './useRooms';

export function roomName(room: Room) {
  return room.name || (room.type === 'dm' ? `Direct message · ${room.id.slice(0, 8)}` : 'Untitled room');
}

export function Sidebar({ open, selected, onSelect }: { open: boolean; selected: Room | null; onSelect: (room: Room) => void }) {
  const { api, session } = useAuth();
  const { rooms, loading, error, more, retry, reload, loadMore } = useRooms(api);
  const [search, setSearch] = useState('');
  const [signingOut, setSigningOut] = useState(false);
  const [logoutError, setLogoutError] = useState('');
  const filtered = rooms.filter((room) => roomName(room).toLowerCase().includes(search.trim().toLowerCase()));

  async function logout() {
    setSigningOut(true); setLogoutError('');
    try { await session.logout(); }
    catch (error) { setLogoutError(errorMessage(error)); }
    finally { setSigningOut(false); }
  }

  return <aside id="room-sidebar" className={`sidebar${open ? ' is-open' : ''}`} aria-label="Your rooms">
    <div className="sidebar-brand"><Brand /><span className="workspace-tag">YOUR SPACE</span></div>
    <div className="sidebar-heading"><h2>Rooms <span className="count-badge">{rooms.length}</span></h2>
      <button className="icon-button" type="button" aria-label="Refresh rooms" title="Refresh rooms" disabled={loading} onClick={reload}>
        <Icon name="refresh" />
      </button>
    </div>
    <label className="room-search"><Icon name="search" /><span className="sr-only">Find a room</span>
      <input type="search" placeholder="Find a room" value={search} onChange={(event) => setSearch(event.target.value)} />
    </label>
    <nav className="room-list" aria-label="Room list" aria-busy={loading}>
      {filtered.map((room) => <button className={`room-button${room.id === selected?.id ? ' is-selected' : ''}`}
        type="button" key={room.id} aria-label={`${roomName(room)}, ${room.type === 'group' ? 'Group room' : 'Direct conversation'}`}
        aria-pressed={room.id === selected?.id} onClick={() => onSelect(room)}>
        <span className={`room-avatar ${room.type}`}><Icon name={room.type === 'group' ? 'hash' : 'message'} /></span>
        <span className="room-details"><span className="room-name">{roomName(room)}</span>
          <span className="room-kind">{room.type === 'group' ? 'Group room' : 'Direct conversation'}</span>
        </span>
      </button>)}
      {loading && <p className="sidebar-note" role="status">{rooms.length ? 'Updating rooms…' : 'Loading your rooms…'}</p>}
      {error && <div className="sidebar-error"><p role="alert">{error}</p><button className="text-button" onClick={retry}>Try again</button></div>}
      {!loading && !error && !rooms.length && <div className="sidebar-empty"><Icon name="rooms" /><h3>No rooms yet</h3><p>Rooms you join will appear here.</p></div>}
      {!loading && rooms.length > 0 && !filtered.length && <p className="sidebar-note">No rooms match “{search}”.</p>}
      {more && !error && <button className="load-more text-button" disabled={loading} onClick={loadMore}>Load more rooms</button>}
    </nav>
    <div className="sidebar-account">
      {logoutError && <p className="error-message" role="alert">{logoutError}</p>}
      <div className="account-row"><span className="account-avatar" aria-hidden="true">P</span>
        <div><strong>Your account</strong><span>Make yourself at home.</span></div>
        <button className="icon-button" aria-label={signingOut ? 'Signing out' : 'Sign out'} title="Sign out"
          disabled={signingOut} onClick={() => { void logout(); }}><Icon name="logout" /></button>
      </div>
    </div>
  </aside>;
}
