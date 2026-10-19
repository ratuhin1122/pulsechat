import { useState } from 'react';
import type { FormEvent, KeyboardEvent } from 'react';
import { Icon } from '../components/Icon';
import type { ConnectionStatus } from './types';

export function Composer({ roomID, status, ready, send }: {
  roomID: string; status: ConnectionStatus; ready: boolean; send: (roomID: string, body: string) => boolean;
}) {
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');
  const connected = status === 'connected';
  const encoder = new TextEncoder();
  const tooLong = encoder.encode(draft).length > 4096
    || encoder.encode(JSON.stringify({ type: 'message', room_id: roomID, body: draft })).length > 8192;
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError('');
    if (!draft.trim() || tooLong || !connected || !ready) return;
    if (send(roomID, draft)) setDraft('');
    else setError('Your message could not be sent. Your draft is still here.');
  }
  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) {
      event.preventDefault(); event.currentTarget.form?.requestSubmit();
    }
  }
  return <form className="composer" onSubmit={submit} aria-label="Message composer">
    <label className="sr-only" htmlFor="message-draft">Message</label>
    <div className="composer-input"><textarea id="message-draft" name="message" rows={2} placeholder="Write a message…"
      value={draft} onChange={(event) => { setDraft(event.target.value); setError(''); }} onKeyDown={keyDown}
      aria-describedby="composer-hint" aria-invalid={tooLong} />
      <button className="primary-button send-button" type="submit" aria-label="Send message"
        disabled={!connected || !ready || !draft.trim() || tooLong}><Icon name="arrow" /></button>
    </div>
    <p id="composer-hint" className="composer-hint">{tooLong ? 'This message is too long. Please shorten it.'
      : !connected ? (status === 'offline' ? 'You’re offline. Your draft stays here.' : 'Reconnecting… Your draft stays here.')
        : !ready ? 'Loading this conversation…' : 'Enter to send · Shift + Enter for a new line'}</p>
    {error && <p className="error-message" role="alert">{error}</p>}
  </form>;
}
