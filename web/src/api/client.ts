import { ApiError, HttpClient } from './http';
import { isRoom } from './types';
import type { Room } from './types';
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
