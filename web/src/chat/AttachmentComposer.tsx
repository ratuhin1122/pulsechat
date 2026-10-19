import { useRef, useState } from 'react';
import { acceptedFiles, fileKind, fileSize } from './files';
import { VoiceRecorder } from './VoiceRecorder';
import type { useUploads } from './useUploads';

export function AttachmentComposer({ roomID, uploads, statuses, ready }: {
  roomID: string; uploads: ReturnType<typeof useUploads>; statuses: Record<string, 'ready' | 'rejected'>; ready: boolean;
}) {
  const picker = useRef<HTMLInputElement>(null);
  const [compress, setCompress] = useState(false);
  return <div className="attachment-composer">
    <div className="attachment-tools">
      <input ref={picker} className="sr-only" type="file" multiple accept={acceptedFiles} aria-label="Choose attachments"
        onChange={(event) => { uploads.add(roomID, [...event.target.files ?? []]); event.target.value = ''; }} />
      <button type="button" onClick={() => picker.current?.click()}>＋ Attach files</button>
      <VoiceRecorder onFile={(file) => uploads.add(roomID, [file])} />
      <label className="compression-option"><input type="checkbox" checked={compress} onChange={(event) => setCompress(event.target.checked)} /> Compress images</label>
    </div>
    <div className="upload-drafts" aria-label="Attachments">
      {uploads.items.filter((item) => item.roomID === roomID).map((item) => {
        const status = item.id ? statuses[item.id] : undefined;
        return <div className="upload-draft" key={item.key}>
          {item.preview && (fileKind(item.file) === 'image' ? <img src={item.preview} alt={`Preview of ${item.file.name}`} />
            : <audio controls src={item.preview} aria-label={`Preview of ${item.file.name}`} />)}
          <div className="upload-description"><strong>{item.file.name}</strong><span>{fileSize(item.file.size)}</span>
            {item.state === 'uploading' && <><progress max={100} value={item.progress} aria-label={`Uploading ${item.file.name}`} /><span>{item.progress}% uploaded</span></>}
            <span role="status">{status === 'ready' ? 'Ready · sent' : status === 'rejected' ? 'Rejected by file validation'
              : item.state === 'processing' ? 'Processing… Waiting for server validation.' : item.state === 'uploading' ? 'Uploading…' : ''}</span>
            {item.error && <span role="alert" className="error-message">{item.error}</span>}
          </div>
          {item.state === 'draft' && <button type="button" disabled={!ready} onClick={() => void uploads.send(item, compress)}>Send attachment</button>}
          {item.state !== 'uploading' && <button type="button" aria-label={`Dismiss ${item.file.name}`} onClick={() => uploads.remove(item.key)}>×</button>}
        </div>;
      })}
    </div>
    <p className="attachment-hint">Drop files or paste images here · Images 15 MB · Documents 25 MB · Voice 10 MB / 5 min</p>
  </div>;
}
