import { useCallback, useEffect, useRef, useState } from 'react';
import type { Session } from '../auth/session';
import { ChatSocket } from './ChatSocket';
import { mergeMessages } from './types';
import type { ConnectionStatus, Message, PresenceChange } from './types';

export function useChatSocket(session: Session) {
  const socket = useRef<ChatSocket | null>(null);
  const revision = useRef(0);
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [epoch, setEpoch] = useState(0);
  const [messages, setMessages] = useState<Record<string, Message[]>>({});
  const [presence, setPresence] = useState<Record<string, PresenceChange>>({});
  useEffect(() => {
    const client = new ChatSocket(session, {
      status: setStatus,
      connected: () => setEpoch((value) => value + 1),
      event: (event) => {
        if (event.type === 'message') {
          const message = event.message;
          setMessages((previous) => ({ ...previous,
            [message.room_id]: mergeMessages(previous[message.room_id] ?? [], [message]),
          }));
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
  return { status, epoch, messages, presence, send, seedCursor };
}
