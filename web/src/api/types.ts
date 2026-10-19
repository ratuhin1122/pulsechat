export interface TokenResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_in: number;
}

export interface LoginInput { email: string; password: string }
export interface RegisterInput extends LoginInput { username: string }

export interface Room {
  id: string;
  type: 'dm' | 'group';
  name: string;
  created_at: string;
}

export function isRoom(value: unknown): value is Room {
  if (!value || typeof value !== 'object') return false;
  const room = value as Record<string, unknown>;
  return typeof room.id === 'string' && (room.type === 'dm' || room.type === 'group')
    && typeof room.name === 'string' && typeof room.created_at === 'string';
}
