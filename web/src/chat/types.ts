export type MessageKind = 'text' | 'image' | 'document' | 'voice';

export interface Attachment {
  id: string;
  kind: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  width?: number;
  height?: number;
  duration_sec?: number;
  blurhash?: string;
}

export interface Message {
  id: string;
  room_id: string;
  sender_id: string;
  type: MessageKind;
  body: string;
  created_at: string;
  attachments?: Attachment[];
}

export interface Presence { user_id: string; online: boolean }
export interface PresenceChange { online: boolean; revision: number }
export type ConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline';
export type ServerEvent = { type: 'message'; message: Message }
  | { type: 'attachment_status'; attachment_id: string; status: 'ready' | 'rejected' }
  | { type: 'presence'; user_id: string; online: boolean } | { type: 'pong' };

export function isMessage(value: unknown): value is Message {
  if (!value || typeof value !== 'object') return false;
  const m = value as Record<string, unknown>;
  return ['id', 'room_id', 'sender_id', 'body', 'created_at'].every((key) => typeof m[key] === 'string')
    && ['id', 'room_id', 'sender_id'].every((key) => (m[key] as string).length > 0)
    && ['text', 'image', 'document', 'voice'].includes(String(m.type))
    && Number.isFinite(Date.parse(String(m.created_at)))
    && (m.attachments === undefined || Array.isArray(m.attachments) && m.attachments.every((a: unknown) => {
      if (!a || typeof a !== 'object') return false;
      const item = a as Record<string, unknown>;
      return ['id', 'kind', 'original_name', 'mime_type'].every((key) => typeof item[key] === 'string')
        && typeof item.size_bytes === 'number' && Number.isFinite(item.size_bytes) && item.size_bytes >= 0
        && ['width', 'height', 'duration_sec'].every((key) => item[key] === undefined
          || typeof item[key] === 'number' && Number.isFinite(item[key]) && item[key] >= 0)
        && (item.blurhash === undefined || typeof item.blurhash === 'string' && item.blurhash.length <= 200);
    }));
}

export function isPresence(value: unknown): value is Presence {
  return !!value && typeof value === 'object' && typeof (value as Presence).user_id === 'string'
    && (value as Presence).user_id.length > 0
    && typeof (value as Presence).online === 'boolean';
}

export function parseEvent(raw: unknown): ServerEvent | null {
  if (typeof raw !== 'string' || raw.length > 64 * 1024) return null;
  try {
    const value = JSON.parse(raw) as Record<string, unknown> | null;
    if (!value || typeof value !== 'object') return null;
    if (value.type === 'pong') return { type: 'pong' };
    if (value.type === 'attachment_status' && typeof value.attachment_id === 'string' && value.attachment_id
      && (value.status === 'ready' || value.status === 'rejected')) {
      return { type: 'attachment_status', attachment_id: value.attachment_id, status: value.status };
    }
    if (value.type === 'presence' && isPresence(value)) return { ...value, type: 'presence' };
    if (value.type === 'message') {
      const message = { ...value, type: value.message_type ?? 'text' };
      if (isMessage(message)) return { type: 'message', message };
    }
  } catch { /* Ignore unrecognized server frames; never render raw protocol data. */ }
  return null;
}

// Retain sub-millisecond timestamp precision for the server's (created_at, id) cursor.
export function compareMessages(a: Message, b: Message): number {
  const millis = Date.parse(a.created_at) - Date.parse(b.created_at);
  if (millis) return millis;
  const submillis = (value: string) => (value.match(/\.(\d+)(?:Z|[+-]\d\d:\d\d)$/)?.[1] ?? '').padEnd(9, '0').slice(3);
  return submillis(a.created_at).localeCompare(submillis(b.created_at)) || a.id.localeCompare(b.id);
}

export function mergeMessages(...pages: Message[][]): Message[] {
  return [...new Map(pages.flat().map((message) => [message.id, message])).values()].sort(compareMessages);
}
