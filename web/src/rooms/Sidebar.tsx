import { useRef, useState } from 'react';
import type { Room } from '../api/types';
import { errorMessage } from '../api/http';
import { useAuth } from '../auth/AuthProvider';
import { Brand } from '../components/Brand';
import { Icon } from '../components/Icon';
import type { useRooms } from './useRooms';
import { DeleteAccount } from '../auth/DeleteAccount';
import { RoomActions } from './RoomActions';

export function roomName(room: Room) {
  return room.name || (room.type === 'dm' ? `Direct message · ${room.id.slice(0, 8)}` : 'Untitled room');
}

export function Sidebar({ open, selected, onSelect, onDelete, store }: {
  open: boolean; selected: Room | null; onSelect: (room: Room) => void; onDelete: (room: Room) => Promise<void>; store: ReturnType<typeof useRooms>;
}) {
  const { api, session } = useAuth();
  const { rooms, loading, error, more, retry, reload, loadMore, addRoom } = store;
  const [deleting, setDeleting] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const deleteFlight = useRef(false);
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

  async function deleteChat(room: Room) {
    if (deleteFlight.current || !window.confirm(`Delete “${roomName(room)}” from your account? This removes its messages and files from your history. Other participants keep their copies.`)) return;
    deleteFlight.current = true; setDeleting(room.id); setDeleteError('');
    try { await onDelete(room); } catch (error) { setDeleteError(errorMessage(error)); }
    finally { deleteFlight.current = false; setDeleting(null); }
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
    <RoomActions api={api} rooms={rooms} disabled={loading} onCreated={(room) => { addRoom(room); setSearch(''); onSelect(room); }} />
    {deleteError && <p role="alert" className="sidebar-error">{deleteError}</p>}
    <nav className="room-list" aria-label="Room list" aria-busy={loading}>
      {(['dm', 'group'] as const).map((kind) => <section className="conversation-section" key={kind} aria-label={kind === 'dm' ? 'Direct messages' : 'Group rooms'}>
      <h3>{kind === 'dm' ? 'Direct messages' : 'Group rooms'}</h3>
      {filtered.filter((room) => room.type === kind).map((room) => <div className="room-row" key={room.id}><button className={`room-button${room.id === selected?.id ? ' is-selected' : ''}`}
        type="button" key={room.id} aria-label={`${roomName(room)}, ${room.type === 'group' ? 'Group room' : 'Direct conversation'}`}
        aria-pressed={room.id === selected?.id} onClick={() => onSelect(room)}>
        <span className={`room-avatar ${room.type}`}><Icon name={room.type === 'group' ? 'hash' : 'message'} /></span>
        <span className="room-details"><span className="room-name">{roomName(room)}</span>
          <span className="room-kind room-preview">{room.last_message ? room.last_message.body || `${room.last_message.type} attachment` : room.type === 'group' ? 'Group room' : 'Direct conversation'}</span>
        </span>
        {!!room.unread_count && <span className="unread-badge" aria-label={`${room.unread_count} unread messages`}>{room.unread_count > 99 ? '99+' : room.unread_count}</span>}
      </button><button type="button" className="icon-button delete-chat-button" aria-label={`Delete ${roomName(room)} chat`} title="Delete chat from your account"
        disabled={deleting !== null} onClick={() => void deleteChat(room)}><Icon name="trash" /></button></div>)}
      {!loading && !search && !rooms.some((room) => room.type === kind) && <p className="sidebar-note">{kind === 'dm' ? 'Start a message to someone.' : 'Create a group for your people.'}</p>}
      </section>)}
      {loading && <p className="sidebar-note" role="status">{rooms.length ? 'Updating rooms…' : 'Loading your rooms…'}</p>}
      {error && <div className="sidebar-error"><p role="alert">{error}</p><button className="text-button" onClick={retry}>Try again</button></div>}
      {!loading && !error && !rooms.length && <div className="sidebar-empty"><Icon name="rooms" /><h3>No rooms yet</h3><p>Use New chat to find people or join a room.</p></div>}
      {!loading && rooms.length > 0 && !filtered.length && <p className="sidebar-note">No rooms match “{search}”.</p>}
      {more && !error && <button className="load-more text-button" disabled={loading} onClick={loadMore}>Load more rooms</button>}
    </nav>
    <div className="sidebar-account">
      <DeleteAccount />
      {logoutError && <p className="error-message" role="alert">{logoutError}</p>}
      <div className="account-row"><span className="account-avatar" aria-hidden="true">P</span>
        <div><strong>Your account</strong><span>Make yourself at home.</span></div>
        <button className="icon-button" aria-label={signingOut ? 'Signing out' : 'Sign out'} title="Sign out"
          disabled={signingOut} onClick={() => { void logout(); }}><Icon name="logout" /></button>
      </div>
    </div>
  </aside>;
}
