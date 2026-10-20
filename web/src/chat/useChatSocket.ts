import { useCallback, useEffect, useRef, useState } from 'react';
import type { Session } from '../auth/session';
import { ChatSocket } from './ChatSocket';
import { mergeMessages } from './types';
import type { ConnectionStatus, Message, PresenceChange } from './types';

export function useChatSocket(session: Session) {
  const socket = useRef<ChatSocket | null>(null);
  const revision = useRef(0);
  const [deletedChats, setDeletedChats] = useState<{ roomID: string; revision: number } | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [epoch, setEpoch] = useState(0);
  const [attachments, setAttachments] = useState<Record<string, 'ready' | 'rejected'>>({});
  const [messages, setMessages] = useState<Record<string, Message[]>>({});
  const [reads, setReads] = useState<Record<string, Record<string, string[]>>>({});
  const [presence, setPresence] = useState<Record<string, PresenceChange>>({});
  useEffect(() => {
    const client = new ChatSocket(session, {
      status: setStatus,
      connected: () => setEpoch((value) => value + 1),
      event: (event) => {
        if (event.type === 'message') {
          const message = event.message;
          if (message.attachments?.length) setAttachments((previous) => ({ ...previous,
            ...Object.fromEntries(message.attachments!.map((item) => [item.id, 'ready' as const])),
          }));
          setMessages((previous) => ({ ...previous,
            [message.room_id]: mergeMessages(previous[message.room_id] ?? [], [message]),
          }));
        } else if (event.type === 'chat_deleted') {
          setMessages((previous) => { const next = { ...previous }; delete next[event.room_id]; return next; });
          setReads((previous) => { const next = { ...previous }; delete next[event.room_id]; return next; });
          setDeletedChats({ roomID: event.room_id, revision: ++revision.current });
        } else if (event.type === 'attachment_status') {
          setAttachments((previous) => ({ ...previous, [event.attachment_id]: event.status }));
        } else if (event.type === 'read') {
          setReads((previous) => ({ ...previous, [event.room_id]: { ...previous[event.room_id],
            [event.user_id]: [...new Set([...(previous[event.room_id]?.[event.user_id] ?? []), event.message_id])] } }));
        } else if (event.type === 'presence') {
          const change = { online: event.online, revision: ++revision.current };
          setPresence((previous) => ({ ...previous, [event.user_id]: change }));
        }
      },
    });
    socket.current = client;
    const online = () => client.setOnline(true);
    const offline = () => client.setOnline(false);
    client.start(navigator.onLine);
    window.addEventListener('online', online); window.addEventListener('offline', offline);
    return () => {
      client.stop(); socket.current = null;
      window.removeEventListener('online', online); window.removeEventListener('offline', offline);
    };
  }, [session]);
  const send = useCallback((roomID: string, body: string) => socket.current?.send(roomID, body) ?? false, []);
  const seedCursor = useCallback((message: Message) => socket.current?.seedCursor(message), []);
  const clearRoom = useCallback((roomID: string) => {
    setMessages((previous) => { const next = { ...previous }; delete next[roomID]; return next; });
    setReads((previous) => { const next = { ...previous }; delete next[roomID]; return next; });
  }, []);
  return { deletedChats, clearRoom, status, epoch, reads, messages, presence, attachments, send, seedCursor };
}
