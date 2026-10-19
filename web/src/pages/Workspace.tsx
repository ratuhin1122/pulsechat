import { useEffect, useRef, useState } from 'react';
import type { Room } from '../api/types';
import { Brand } from '../components/Brand';
import { Icon } from '../components/Icon';
import { roomName, Sidebar } from '../rooms/Sidebar';

export function Workspace() {
  const [selected, setSelected] = useState<Room | null>(null);
  const [open, setOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { document.title = `${selected ? roomName(selected) : 'Your space'} · Pulsechat`; }, [selected]);
  useEffect(() => { if (selected) heading.current?.focus(); }, [selected]);

  return <div className="workspace" onKeyDown={(event) => {
    if (event.key === 'Escape' && open) { setOpen(false); menuButton.current?.focus(); }
  }}>
    <header className="mobile-header"><Brand />
      <button ref={menuButton} className="icon-button" aria-label={open ? 'Close rooms' : 'Open rooms'}
        aria-expanded={open} aria-controls="room-sidebar" onClick={() => setOpen(!open)}><Icon name={open ? 'close' : 'menu'} /></button>
    </header>
    <Sidebar open={open} selected={selected} onSelect={(room) => { setSelected(room); setOpen(false); }} />
    <main className="conversation-panel" id="main-content">
      <header className="conversation-header"><div className="room-heading-icon"><Icon name={selected?.type === 'group' ? 'hash' : 'rooms'} /></div>
        <div><h1 ref={heading} tabIndex={-1}>{selected ? roomName(selected) : 'Your space'}</h1>
          <p>{selected ? (selected.type === 'group' ? 'Group room' : 'Direct conversation') : 'A little closer, wherever you are.'}</p>
        </div>
        {selected && <span className="room-label">{selected.type === 'dm' ? 'DIRECT' : 'GROUP'}</span>}
      </header>
      <div className="conversation-empty"><div className="welcome-mark" aria-hidden="true"><Icon name="message" /><span /></div>
        <span className="eyebrow">ROOM FOR CONNECTION</span>
        <h2>{selected ? `You're in ${roomName(selected)}.` : 'Your people. One place.'}</h2>
        <p>{selected ? 'Make space for a good conversation.' : 'Choose a room from the sidebar to make yourself at home.'}</p>
        <div className="welcome-line" aria-hidden="true" />
      </div>
    </main>
  </div>;
}
