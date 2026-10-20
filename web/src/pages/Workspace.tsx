import { useCallback, useEffect, useRef, useState } from 'react';
import type { Room } from '../api/types';
import { errorMessage } from '../api/http';
import { Brand } from '../components/Brand';
import { Icon } from '../components/Icon';
import { roomName, Sidebar } from '../rooms/Sidebar';
import { useAuth } from '../auth/AuthProvider';
import { useChatSocket } from '../chat/useChatSocket';
import { ChatView } from '../chat/ChatView';
import { useUploads } from '../chat/useUploads';
import { useRooms } from '../rooms/useRooms';
import { useLeaveRoom } from '../rooms/useLeaveRoom';
import { MembersPanel } from '../rooms/MembersPanel';

export function Workspace() {
  const { session, api, userID, displayName } = useAuth();
  const [signingOut, setSigningOut] = useState(false);
  const [logoutError, setLogoutError] = useState('');
  async function logout() {
    setSigningOut(true); setLogoutError('');
    try { await session.logout(); } catch (error) { setLogoutError(errorMessage(error)); }
    finally { setSigningOut(false); }
  }
  const uploads = useUploads(api);
  const realtime = useChatSocket(session);
  const rooms = useRooms(api);
  const reloadRooms = useRef(rooms.reload);
  reloadRooms.current = rooms.reload;
  useEffect(() => {
    const timer = setTimeout(() => reloadRooms.current(), 200);
    return () => clearTimeout(timer);
  }, [realtime.messages, realtime.reads, realtime.epoch]);
  const [selected, setSelected] = useState<Room | null>(null);
  const [open, setOpen] = useState(false);
  const [membersID, setMembersID] = useState<string | null>(null);
  const removed = useCallback((id: string) => {
    rooms.removeRoom(id);
    setSelected((current) => current?.id === id ? null : current);
    setMembersID((current) => current === id ? null : current);
  }, [rooms.removeRoom]);
  const uploadsRef = useRef(uploads);
  uploadsRef.current = uploads;
  useEffect(() => {
    if (!realtime.deletedChats) return;
    const id = realtime.deletedChats.roomID;
    uploadsRef.current.items.filter((item) => item.roomID === id).forEach((item) => uploadsRef.current.remove(item.key));
    removed(id);
  }, [realtime.deletedChats, removed]);
  const leave = useLeaveRoom(api, removed);
  const menuButton = useRef<HTMLButtonElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { document.title = `${selected ? roomName(selected) : 'Your space'} · Pulsechat`; }, [selected]);
  useEffect(() => { heading.current?.focus(); }, [selected]);

  useEffect(() => {
    const closeChat = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('dialog[open]')
        || event.target instanceof Element && event.target.closest('[role="dialog"]')) return;
      setSelected(null); setMembersID(null); setOpen(false);
      if (open) menuButton.current?.focus();
    };
    window.addEventListener('keydown', closeChat);
    return () => window.removeEventListener('keydown', closeChat);
  }, [open]);

  return <div className="workspace">
    <header className="mobile-header"><Brand />
      <button ref={menuButton} className="icon-button" aria-label={open ? 'Close rooms' : 'Open rooms'}
        aria-expanded={open} aria-controls="room-sidebar" onClick={() => setOpen(!open)}><Icon name={open ? 'close' : 'menu'} /></button>
    </header>
    <Sidebar onDelete={async (room) => {
      await api.deleteChat(room.id, new AbortController().signal);
      uploads.items.filter((item) => item.roomID === room.id).forEach((item) => uploads.remove(item.key));
      realtime.clearRoom(room.id); removed(room.id);
    }} open={open} selected={selected} store={rooms} onSelect={(room) => { setSelected(room); setMembersID(null); setOpen(false); }} />
    <main className="conversation-panel" id="main-content">
      <header className="conversation-header"><div className="room-heading-icon"><Icon name={selected?.type === 'group' ? 'hash' : 'rooms'} /></div>
        <div><h1 ref={heading} tabIndex={-1}>{selected ? roomName(selected) : 'Your space'}</h1>
          <p>{selected ? (selected.type === 'group' ? 'Group room' : 'Direct conversation') : 'A little closer, wherever you are.'}</p>
        </div>
        {selected && <span className="room-label">{selected.type === 'dm' ? 'DIRECT' : 'GROUP'}</span>}
        {selected && <button type="button" className="text-button" aria-expanded={membersID === selected.id} aria-controls="room-members-panel"
          onClick={() => setMembersID(membersID === selected.id ? null : selected.id)}>Members</button>}
        <div className="header-account"><span>{displayName || (userID ? `User ${userID.slice(0, 8)}` : 'Your account')}</span>
          <button type="button" className="text-button" disabled={signingOut} onClick={() => void logout()}>{signingOut ? 'Logging out…' : 'Logout'}</button>
          {logoutError && <span role="alert" className="error-message">{logoutError}</span>}
        </div>
      </header>
      <div className="conversation-body">
      {selected ? <ChatView key={`chat-${selected.id}`} roomID={selected.id} realtime={realtime} uploads={uploads} /> : <div className="conversation-empty"><div className="welcome-mark" aria-hidden="true"><Icon name="message" /><span /></div>
        <span className="eyebrow">ROOM FOR CONNECTION</span>
        <h2>{selected ? `You're in ${roomName(selected)}.` : 'Your people. One place.'}</h2>
        <p>{selected ? 'Make space for a good conversation.' : 'Choose a room from the sidebar to make yourself at home.'}</p>
        <div className="welcome-line" aria-hidden="true" />
      </div>}
      {selected && membersID === selected.id && <MembersPanel key={`members-${selected.id}`} api={api} room={selected} onClose={() => setMembersID(null)}
        onLeave={() => leave.leave(selected.id)} leaving={leave.pendingID === selected.id} leaveDisabled={!!leave.pendingID}
        leaveError={leave.errorID === selected.id ? leave.error : ''} />}
      </div>
    </main>
  </div>;
}
