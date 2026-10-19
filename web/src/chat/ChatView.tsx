import { useAuth } from '../auth/AuthProvider';
import { Composer } from './Composer';
import { MessageList } from './MessageList';
import { PresenceDot } from './PresenceDot';
import { useHistory } from './useHistory';
import { usePresence } from './usePresence';
import type { useChatSocket } from './useChatSocket';

export function ChatView({ roomID, realtime }: { roomID: string; realtime: ReturnType<typeof useChatSocket> }) {
  const { api } = useAuth();
  const history = useHistory(api, roomID, realtime.messages[roomID] ?? [], realtime.epoch, realtime.seedCursor);
  const presence = usePresence(api, roomID, realtime.epoch, realtime.presence, realtime.status === 'connected');
  const count = presence.members.filter((id) => presence.online[id]).length;
  return <section className="chat-view" aria-label="Conversation">
    <div className="chat-toolbar">
      <span className={`connection-status status-${realtime.status}`} role="status">
        {realtime.status === 'connected' ? 'Connected' : realtime.status === 'offline' ? 'Offline' : realtime.status === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
      </span>
      <details className="room-presence"><summary>{presence.error || realtime.status !== 'connected' ? 'Presence unavailable' : presence.loading ? 'Checking presence…' : `${count} online`}</summary>
        <ul>{presence.members.map((id) => <li key={id}><PresenceDot online={presence.online[id]} /><span title={id}>User {id.slice(0, 8)}</span></li>)}</ul>
      </details>
    </div>
    <MessageList {...history} online={presence.online} />
    <Composer roomID={roomID} status={realtime.status} ready={history.initialized} send={realtime.send} />
  </section>;
}
