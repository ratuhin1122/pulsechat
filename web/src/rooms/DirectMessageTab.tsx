import { useEffect, useState } from 'react';
import type { ApiClient } from '../api/client';
import type { ChatUser } from '../api/types';
import { errorMessage } from '../api/http';
import type { OpenConversation } from './NewChatModal';

export function DirectMessageTab({ api, busy, run }: { api: ApiClient; busy: boolean; run: OpenConversation }) {
  const [query, setQuery] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState({ term: '', users: [] as ChatUser[], loading: false, error: '' });
  const term = query.trim();
  const tooLong = new TextEncoder().encode(term).length > 64;
  const valid = [...term].length >= 2 && !tooLong;
  useEffect(() => {
    if (!valid) return;
    const controller = new AbortController();
    setResult({ term, users: [], loading: true, error: '' });
    const timer = setTimeout(() => {
      void api.searchUsers(term, controller.signal).then((users) => {
        if (!controller.signal.aborted) setResult({ term, users, loading: false, error: '' });
      }).catch((error: unknown) => {
        if (!controller.signal.aborted) setResult({ term, users: [], loading: false, error: errorMessage(error) });
      });
    }, 250);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [api, term, valid, attempt]);
  const current = result.term === term ? result : { users: [], loading: true, error: '' };
  return <div>
    <label htmlFor="new-chat-user">Search users</label>
    <input id="new-chat-user" type="search" maxLength={64} autoComplete="off" placeholder="Username or display name" value={query}
      disabled={busy} onChange={(event) => setQuery(event.target.value)} />
    {!valid ? <p className="modal-note">{tooLong ? 'Please shorten your search.' : 'Enter at least two characters to find someone.'}</p>
      : current.loading ? <p role="status">Searching users…</p>
        : current.error ? <div><p role="alert">{current.error}</p><button className="text-button" onClick={() => setAttempt((value) => value + 1)}>Retry user search</button></div>
          : !current.users.length ? <p role="status">No matching users. Try a different name.</p>
            : <ul className="user-results">{current.users.map((user) => <li key={user.id}>
              <button type="button" disabled={busy} aria-label={`Start chat with ${user.username}`} onClick={() => run((signal) => api.directMessage(user.id, signal))}>
                <span><strong>{user.display_name || user.username}</strong><small>@{user.username}</small></span><span>Chat →</span>
              </button>
            </li>)}</ul>}
  </div>;
}
