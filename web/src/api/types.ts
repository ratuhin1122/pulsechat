export interface TokenResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_in: number;
}

export interface LoginInput { email: string; password: string }
export interface RegisterInput extends LoginInput { username: string }

export interface ChatUser { id: string; username: string; display_name?: string }
export interface RoomMember { user_id: string; joined_at: string }

export function isChatUser(value: unknown): value is ChatUser {
  if (!value || typeof value !== 'object') return false;
  const user = value as ChatUser;
  return typeof user.id === 'string' && user.id.length > 0 && typeof user.username === 'string' && user.username.length > 0
    && (user.display_name === undefined || typeof user.display_name === 'string');
}

export interface MessagePreview { id: string; sender_id: string; type: string; body: string; created_at: string }

export interface Room {
  id: string;
  type: 'dm' | 'group';
  name: string;
  created_at: string;
  is_public?: boolean;
  unread_count?: number;
  last_message?: MessagePreview | null;
}

export function isRoom(value: unknown): value is Room {
  if (!value || typeof value !== 'object') return false;
  const room = value as Record<string, unknown>;
  return typeof room.id === 'string' && (room.type === 'dm' || room.type === 'group')
    && typeof room.name === 'string' && typeof room.created_at === 'string'
    && (room.unread_count === undefined || typeof room.unread_count === 'number' && Number.isSafeInteger(room.unread_count) && room.unread_count >= 0)
    && (room.last_message == null || typeof room.last_message === 'object' && ['id', 'sender_id', 'type', 'body', 'created_at'].every((key) => typeof (room.last_message as Record<string, unknown>)[key] === 'string'))
    && (room.is_public === undefined || typeof room.is_public === 'boolean');
}
