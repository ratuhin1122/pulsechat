import { useLayoutEffect, useRef } from 'react';
import { PresenceDot } from './PresenceDot';
import { compareMessages } from './types';
import type { Message } from './types';
import { AttachmentBubble } from './AttachmentBubble';

interface Props {
  userID?: string; reads?: Record<string, string[]>;
  messages: Message[]; loading: boolean; initialized: boolean; more: boolean; error: string;
  online: Record<string, boolean | undefined>; loadOlder: () => Promise<void>; retry: () => Promise<void>;
}

export function MessageList({ userID, reads = {}, messages, loading, initialized, more, error, online, loadOlder, retry }: Props) {
  const isRead = (message: Message) => Object.entries(reads).some(([user, ids]) => user !== userID
    && messages.some((marker) => ids.includes(marker.id) && compareMessages(marker, message) >= 0));
  const viewport = useRef<HTMLDivElement>(null);
  const bottom = useRef(true);
  const initial = useRef(true);
  const anchor = useRef<{ height: number; top: number; id?: string; offset?: number } | null>(null);
  function captureAnchor(node: HTMLDivElement) {
    const top = node.getBoundingClientRect().top;
    const first = [...node.querySelectorAll<HTMLElement>('[data-message-id]')].find((item) => item.getBoundingClientRect().bottom > top);
    return { height: node.scrollHeight, top: node.scrollTop, id: first?.dataset.messageId, offset: first ? first.getBoundingClientRect().top - top : undefined };
  }
  function older(retrying = false) {
    const node = viewport.current;
    if (loading || !node || !retrying && !more) return;
    anchor.current = initialized ? captureAnchor(node) : null;
    void (retrying ? retry() : loadOlder());
  }
  useLayoutEffect(() => {
    const node = viewport.current;
    if (!node) return;
    if (anchor.current) {
      if (!loading) {
        const saved = anchor.current;
        const item = saved.id ? [...node.querySelectorAll<HTMLElement>('[data-message-id]')].find((item) => item.dataset.messageId === saved.id) : undefined;
        node.scrollTop = item && saved.offset !== undefined
          ? node.scrollTop + item.getBoundingClientRect().top - node.getBoundingClientRect().top - saved.offset
          : saved.top + node.scrollHeight - saved.height;
        anchor.current = null;
      }
    } else if (initial.current || bottom.current) node.scrollTop = node.scrollHeight;
    if (initialized) initial.current = false;
  }, [messages, loading, initialized]);

  return <div ref={viewport} className="message-viewport" tabIndex={0} role="log" aria-label="Messages" aria-live="polite" aria-relevant="additions"
    onScroll={() => {
      const node = viewport.current;
      if (!node) return;
      bottom.current = node.scrollHeight - node.scrollTop - node.clientHeight < 80;
      if (anchor.current && loading) anchor.current = captureAnchor(node);
      if (node.scrollTop < 60 && more && !loading && !error) older();
    }}>
    <div className="history-control">
      {loading ? <span role="status">Loading messages…</span>
        : error ? <><p role="alert">{error}</p><button className="text-button" onClick={() => older(true)}>Try again</button></>
          : more ? <button className="text-button" onClick={() => older()}>Load earlier messages</button>
            : initialized && messages.length > 0 ? <span>Beginning of this conversation</span> : null}
    </div>
    {!loading && !error && !messages.length && <div className="empty-messages"><h2>Start the conversation.</h2><p>A simple hello is a good place to begin.</p></div>}
    {messages.map((message) => <article className={`chat-message ${message.sender_id === userID ? 'is-own' : 'is-other'}`} key={message.id} data-message-id={message.id}>
      <div className="message-avatar" aria-hidden="true">{message.sender_id.slice(0, 2).toUpperCase()}</div>
      <div className="message-content"><header>
        <span className="message-author" title={message.sender_id}>User {message.sender_id.slice(0, 8)}</span>
        <PresenceDot online={online[message.sender_id]} />
        <time dateTime={message.created_at} title={new Date(message.created_at).toLocaleString()}>
          {new Date(message.created_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}
        </time>
        {message.sender_id === userID && <span className="read-receipt" aria-label={isRead(message) ? 'Read' : 'Sent'}>{isRead(message) ? '✓✓' : '✓'}</span>}
      </header>
      {message.body && <p className="message-body">{message.body}</p>}
      {message.attachments?.map((attachment) => <AttachmentBubble key={attachment.id} attachment={attachment} />)}
      {message.type !== 'text' && !message.attachments?.length && <p className="attachment-summary">{message.type} attachment</p>}
      </div>
    </article>)}
  </div>;
}
