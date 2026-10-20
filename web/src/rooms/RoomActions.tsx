import { useState } from 'react';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import { NewChatModal } from './NewChatModal';

export function RoomActions({ api, disabled, onCreated, rooms = [] }: {
  api: ApiClient; disabled: boolean; onCreated: (room: Room) => void; rooms?: Room[];
}) {
  const [open, setOpen] = useState(false);
  return <div className="room-actions">
    <button type="button" className="primary-button" disabled={disabled} onClick={() => setOpen(true)}>＋ New chat</button>
    {open && <NewChatModal api={api} rooms={rooms} onOpen={(room) => { onCreated(room); setOpen(false); }} onClose={() => setOpen(false)} />}
  </div>;
}
