// Test double for the browser's socket surface; never imported by the app.
import type { Message } from './types';

export function sampleMessage(id = '00000000-0000-4000-8000-000000000001', created_at = '2026-10-19T12:00:00.000001Z'): Message {
  return { id, room_id: 'room-1', sender_id: 'user-1', type: 'text', body: 'Hello', created_at };
}

export class TestSocket {
  static instances: TestSocket[] = [];
  readyState = 0;
  bufferedAmount = 0;
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((event: { code: number; reason: string }) => void) | null = null;
  constructor(public readonly url: string) { TestSocket.instances.push(this); }
  open() { this.readyState = 1; this.onopen?.(); }
  receive(data: unknown) { this.onmessage?.({ data: typeof data === 'string' ? data : JSON.stringify(data) }); }
  send(data: string) { if (this.readyState !== 1) throw new Error('not open'); this.sent.push(data); }
  close() { this.drop(1000); }
  drop(code = 1006, reason = '') { this.readyState = 3; this.onclose?.({ code, reason }); }
}
