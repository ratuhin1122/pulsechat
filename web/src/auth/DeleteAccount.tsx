import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from './AuthProvider';
import { errorMessage } from '../api/http';

export function DeleteAccount() {
  const { session } = useAuth();
  const [open, setOpen] = useState(false);
  return <><button type="button" className="text-button danger-text" onClick={() => setOpen(true)}>Delete account</button>
    {open && <AccountDialog remove={(password) => session.deleteAccount(password)} onClose={() => setOpen(false)} />}</>;
}

function AccountDialog({ remove, onClose }: { remove: (password: string) => Promise<void>; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const flight = useRef(false);
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const node = dialog.current; node?.showModal();
    return () => { node?.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  return createPortal(<dialog ref={dialog} className="new-chat-modal" aria-labelledby="delete-account-title"
    onKeyDown={(event) => { if (event.key === 'Escape') event.stopPropagation(); }}
    onCancel={(event) => { event.preventDefault(); if (!busy) onClose(); }}>
    <h2 id="delete-account-title">Delete your account?</h2>
    <p className="modal-note">This permanently deletes your account, your sent messages, and your uploaded images, documents, and voice recordings for everyone. This cannot be undone.</p>
    <form onSubmit={(event) => {
      event.preventDefault(); if (flight.current) return; flight.current = true; setBusy(true); setError('');
      void remove(password).catch((error: unknown) => { setError(errorMessage(error)); })
        .finally(() => { flight.current = false; setBusy(false); });
    }}>
      <label htmlFor="delete-account-password">Confirm your password</label>
      <input id="delete-account-password" type="password" autoComplete="current-password" required maxLength={1024} value={password} disabled={busy} onChange={(event) => setPassword(event.target.value)} />
      {error && <p className="error-message" role="alert">{error}</p>}
      <div className="deletion-actions"><button type="button" className="text-button" disabled={busy} onClick={onClose}>Cancel</button>
        <button type="submit" className="danger-button" disabled={busy || !password}>{busy ? 'Deleting account…' : 'Permanently delete account'}</button></div>
    </form>
  </dialog>, document.body);
}
