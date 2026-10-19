import { useEffect, useRef, useState } from 'react';
import { decode } from 'blurhash';
import { useFileURL } from './useFileURL';
import { duration, fileSize } from './files';
import type { Attachment } from './types';

function Placeholder({ hash }: { hash?: string }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    if (!hash) return;
    try {
      const pixels = decode(hash, 32, 32);
      canvas.current?.getContext('2d')?.putImageData(new ImageData(new Uint8ClampedArray(pixels), 32, 32), 0, 0);
    } catch { /* Invalid or unsupported hashes retain the neutral placeholder. */ }
  }, [hash]);
  return <canvas ref={canvas} width={32} height={32} aria-hidden="true" className="blurhash-placeholder" />;
}

function Zoom({ attachment, close }: { attachment: Attachment; close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const file = useFileURL(attachment.id, 'display');
  useEffect(() => {
    const node = dialog.current; const previous = document.activeElement as HTMLElement | null;
    node?.showModal();
    return () => { node?.close(); previous?.focus(); };
  }, []);
  return <dialog ref={dialog} className="image-zoom" aria-label={attachment.original_name} onCancel={close}
    onClick={(event) => { if (event.target === event.currentTarget) close(); }}>
    <button type="button" autoFocus onClick={close}>Close image</button>
    {file.url ? <img src={file.url} alt={attachment.original_name} /> : <p role="status">{file.error || 'Loading image…'}</p>}
    {file.error && <button onClick={file.retry}>Try again</button>}
  </dialog>;
}

function ImageBubble({ attachment }: { attachment: Attachment }) {
  const file = useFileURL(attachment.id, 'thumb');
  const [zoom, setZoom] = useState(false);
  const [loaded, setLoaded] = useState(false);
  return <div className="image-bubble">
    <button type="button" className="image-preview" aria-label={`Zoom ${attachment.original_name}`} onClick={() => setZoom(true)}
      style={{ aspectRatio: attachment.width && attachment.height ? `${attachment.width} / ${attachment.height}` : '4 / 3' }}>
      <Placeholder hash={attachment.blurhash} />
      {file.url && <img src={file.url} alt={attachment.original_name} style={{ opacity: loaded ? 1 : 0 }} onLoad={() => setLoaded(true)} />}
    </button>
    {file.error && <button type="button" onClick={file.retry}>Retry image</button>}
    {zoom && <Zoom attachment={attachment} close={() => setZoom(false)} />}
  </div>;
}

function DocumentBubble({ attachment }: { attachment: Attachment }) {
  const [requested, setRequested] = useState(false);
  const downloaded = useRef('');
  const file = useFileURL(attachment.id, 'original', requested);
  useEffect(() => {
    if (!file.url || downloaded.current === file.url) return;
    downloaded.current = file.url;
    const link = document.createElement('a'); link.href = file.url; link.download = attachment.original_name;
    document.body.append(link); link.click(); link.remove();
  }, [file.url, attachment.original_name]);
  return <div className="document-card">
    <svg viewBox="0 0 24 24" width="30" height="30" fill="none" stroke="currentColor" aria-hidden="true"><path d="M5 2h9l5 5v15H5z M14 2v6h5 M8 13h8 M8 17h8" /></svg>
    <div><strong>{attachment.original_name}</strong><span>{fileSize(attachment.size_bytes)}</span></div>
    {file.url ? <a href={file.url} download={attachment.original_name}>Download</a>
      : <button type="button" disabled={requested && !file.error} onClick={() => { setRequested(true); if (file.error) file.retry(); }}>{file.error ? 'Retry download' : requested ? 'Loading…' : 'Download'}</button>}
    {file.error && <span role="alert">{file.error}</span>}
  </div>;
}

function VoiceBubble({ attachment }: { attachment: Attachment }) {
  const file = useFileURL(attachment.id, 'original');
  const [seconds, setSeconds] = useState(attachment.duration_sec ?? 0);
  return <div className="voice-bubble"><span>Voice message · {duration(seconds)}</span>
    {file.url ? <audio controls preload="metadata" src={file.url} aria-label={`Voice message ${attachment.original_name}`}
      onLoadedMetadata={(event) => { if (!attachment.duration_sec) setSeconds(event.currentTarget.duration); }} />
      : <span role="status">{file.error || 'Loading audio…'}</span>}
    {file.error && <button type="button" onClick={file.retry}>Retry audio</button>}
  </div>;
}

export function AttachmentBubble({ attachment }: { attachment: Attachment }) {
  return attachment.kind === 'image' ? <ImageBubble attachment={attachment} />
    : attachment.kind === 'voice' ? <VoiceBubble attachment={attachment} /> : <DocumentBubble attachment={attachment} />;
}
