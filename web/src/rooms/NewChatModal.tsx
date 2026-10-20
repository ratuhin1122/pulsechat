import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { ApiClient } from '../api/client';
import type { Room } from '../api/types';
import { errorMessage } from '../api/http';
import { DirectMessageTab } from './DirectMessageTab';
import { RoomsTab } from './RoomsTab';

export type OpenConversation = (task: (signal: AbortSignal) => Promise<Room>) => void;

export function NewChatModal({ api, rooms, onOpen, onClose }: {
  api: ApiClient; rooms: Room[]; onOpen: (room: Room) => void; onClose: () => void;
}) {
  const id = useId();
  const dialog = useRef<HTMLDialogElement>(null);
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  const flight = useRef<AbortController | null>(null);
  const [tab, setTab] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const node = dialog.current; node?.showModal();
    return () => { flight.current?.abort(); node?.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  const run: OpenConversation = (task) => {
    if (flight.current) return;
    const controller = new AbortController(); flight.current = controller; setBusy(true); setError('');
    void task(controller.signal).then((room) => { if (!controller.signal.aborted) onOpen(room); })
      .catch((error: unknown) => { if (!controller.signal.aborted) setError(errorMessage(error)); })
      .finally(() => { if (!controller.signal.aborted) { flight.current = null; setBusy(false); } });
  };
  return createPortal(<dialog ref={dialog} className="new-chat-modal" aria-labelledby={`${id}-title`}
    onKeyDown={(event) => { if (event.key === 'Escape') event.stopPropagation(); }}
    onCancel={(event) => { event.preventDefault(); onClose(); }}>
    <header><h2 id={`${id}-title`}>New chat</h2><button className="text-button" onClick={onClose} aria-label="Close new chat">Close</button></header>
    <div className="new-chat-tabs" role="tablist" aria-label="Conversation type">
      {['Direct message', 'Rooms'].map((label, index) => <button type="button" role="tab" key={label}
        ref={(node) => { tabs.current[index] = node; }} id={`${id}-tab-${index}`} aria-controls={`${id}-panel-${index}`}
        aria-selected={tab === index} tabIndex={tab === index ? 0 : -1} disabled={busy}
        onClick={() => { setTab(index); setError(''); }} onKeyDown={(event) => {
          if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
          event.preventDefault(); const next = event.key === 'Home' ? 0 : event.key === 'End' ? 1 : 1 - tab;
          setTab(next); setError(''); tabs.current[next]?.focus();
        }}
      >{label}</button>)}
    </div>
    <div role="tabpanel" id={`${id}-panel-0`} aria-labelledby={`${id}-tab-0`} className="new-chat-content" hidden={tab !== 0}>
      {tab === 0 && <DirectMessageTab api={api} busy={busy} run={run} />}
    </div>
    <div role="tabpanel" id={`${id}-panel-1`} aria-labelledby={`${id}-tab-1`} className="new-chat-content" hidden={tab !== 1}>
      {tab === 1 && <RoomsTab api={api} rooms={rooms} busy={busy} run={run} />}
    </div>
    {busy && <p role="status" className="modal-status">Opening conversation…</p>}
    {error && <p role="alert" className="error-message">{error}</p>}
  </dialog>, document.body);
}
