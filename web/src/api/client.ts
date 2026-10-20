import { ApiError, HttpClient } from './http';
import { isRoom, isChatUser } from './types';
import type { Room, ChatUser, RoomMember } from './types';
import type { Session } from '../auth/session';
import { isMessage, isPresence } from '../chat/types';
import type { Message, Presence } from '../chat/types';
import { uploadFile } from './upload';

export class ApiClient {
  constructor(private readonly http: HttpClient, private readonly session: Session) {}

  private async authorized<T>(send: (token: string) => Promise<T>, signal?: AbortSignal): Promise<T> {
    let token = await this.session.getAccessToken();
    signal?.throwIfAborted();
    try { return await send(token); }
    catch (error) {
      if (!(error instanceof ApiError) || error.status !== 401) throw error;
      token = await this.session.refresh(token);
      signal?.throwIfAborted();
      try { return await send(token); }
      catch (retryError) {
        if (retryError instanceof ApiError && retryError.status === 401) this.session.expire(token);
        throw retryError;
      }
    }
  }

  file(id: string, variant: 'thumb' | 'display' | 'original', signal?: AbortSignal): Promise<Blob> {
    return this.authorized((token) => this.http.blob(`/files/${encodeURIComponent(id)}?variant=${variant}`, token, signal), signal);
  }

  upload(roomID: string, file: File, progress: (percent: number) => void, signal: AbortSignal) {
    return this.authorized((token) => uploadFile(roomID, file, token, progress, signal), signal);
  }

  private async postRoom(path: string, body: unknown, signal: AbortSignal): Promise<Room> {
    const result = await this.authorized((token) => this.http.send<unknown>(path, {
      method: 'POST', signal, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    }), signal);
    if (!isRoom(result)) throw new ApiError(502, 'The server returned an invalid conversation. Check your list before trying again.');
    return result;
  }

  createGroup(name: string, signal: AbortSignal, isPublic = false): Promise<Room> {
    return this.postRoom('/rooms', { name, is_public: isPublic }, signal);
  }

  directMessage(userID: string, signal: AbortSignal): Promise<Room> {
    return this.postRoom('/rooms/dm', { user_id: userID }, signal);
  }

  async searchUsers(search: string, signal: AbortSignal): Promise<ChatUser[]> {
    const result = await this.get<unknown>(`/users?${new URLSearchParams({ q: search })}`, signal);
    if (!Array.isArray(result) || !result.every(isChatUser)) throw new ApiError(502, 'Could not read the user search results.');
    return result;
  }

  private async get<T>(path: string, signal?: AbortSignal): Promise<T> {
    let token = await this.session.getAccessToken();
    signal?.throwIfAborted();
    const send = () => this.http.send<T>(path, { signal, headers: { Authorization: `Bearer ${token}` } });
    try { return await send(); }
    catch (error) {
      if (!(error instanceof ApiError) || error.status !== 401) throw error;
      token = await this.session.refresh(token);
      signal?.throwIfAborted();
      try { return await send(); }
      catch (retryError) {
        if (retryError instanceof ApiError && retryError.status === 401) this.session.expire(token);
        throw retryError;
      }
    }
  }

  async publicRooms(q: string, offset: number, signal: AbortSignal): Promise<Room[]> {
    const result = await this.get<unknown>(`/rooms/public?${new URLSearchParams({ q, limit: '20', offset: String(offset) })}`, signal);
    if (!Array.isArray(result) || !result.every(isRoom) || result.some((room) => room.type !== 'group' || !room.is_public)) throw new ApiError(502, 'Could not read public rooms.');
    return result;
  }

  private postEmpty(path: string, signal: AbortSignal): Promise<void> {
    return this.authorized((token) => this.http.send<void>(path, { method: 'POST', signal, headers: { Authorization: `Bearer ${token}` } }), signal);
  }

  markRead(id: string, messageID: string, signal: AbortSignal): Promise<void> {
    return this.authorized((token) => this.http.send<void>(`/rooms/${encodeURIComponent(id)}/read`, { method: 'POST', signal, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ message_id: messageID }) }), signal);
  }

  joinRoom(id: string, signal: AbortSignal): Promise<void> { return this.postEmpty(`/rooms/${encodeURIComponent(id)}/join`, signal); }
  leaveRoom(id: string, signal: AbortSignal): Promise<void> { return this.postEmpty(`/rooms/${encodeURIComponent(id)}/leave`, signal); }

  async members(id: string, offset: number, signal: AbortSignal): Promise<RoomMember[]> {
    const result = await this.get<unknown>(`/rooms/${encodeURIComponent(id)}/members?${new URLSearchParams({ limit: '50', offset: String(offset) })}`, signal);
    if (!Array.isArray(result) || !result.every((item: unknown) => {
      if (!item || typeof item !== 'object') return false;
      const member = item as RoomMember;
      return typeof member.user_id === 'string' && !!member.user_id && typeof member.joined_at === 'string' && Number.isFinite(Date.parse(member.joined_at));
    })) throw new ApiError(502, 'Could not read room members.');
    return result;
  }

  async listRooms(options: { limit?: number; offset?: number; signal?: AbortSignal } = {}): Promise<Room[]> {
    const params = new URLSearchParams({ limit: String(options.limit ?? 50), offset: String(options.offset ?? 0) });
    const response = await this.get<unknown>(`/rooms?${params}`, options.signal);
    if (!Array.isArray(response) || !response.every(isRoom)) {
      throw new ApiError(502, 'The server returned an invalid room list.');
    }
    return response;
  }

  async history(roomID: string, options: { before?: string; signal?: AbortSignal } = {}): Promise<Message[]> {
    const params = new URLSearchParams({ limit: '50' });
    if (options.before) params.set('before', options.before);
    const response = await this.get<unknown>(`/rooms/${encodeURIComponent(roomID)}/messages?${params}`, options.signal);
    if (!Array.isArray(response) || !response.every(isMessage) || response.some((message) => message.room_id !== roomID)) {
      throw new ApiError(502, 'The server returned an invalid conversation.');
    }
    return response;
  }

  async presence(roomID: string, signal?: AbortSignal): Promise<Presence[]> {
    const response = await this.get<unknown>(`/presence?${new URLSearchParams({ room_id: roomID })}`, signal);
    if (!Array.isArray(response) || !response.every(isPresence)) throw new ApiError(502, 'Could not read room presence.');
    return response;
  }
}
