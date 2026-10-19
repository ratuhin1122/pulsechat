import { ApiError } from './http';

export interface UploadResult { id: string; kind: string; status: 'pending' }

export function uploadFile(roomID: string, file: File, token: string, progress: (percent: number) => void,
  signal: AbortSignal, create = () => new XMLHttpRequest()): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted();
    const xhr = create();
    const abort = () => xhr.abort();
    const finish = () => signal.removeEventListener('abort', abort);
    xhr.open('POST', `/rooms/${encodeURIComponent(roomID)}/upload`);
    xhr.setRequestHeader('Authorization', `Bearer ${token}`);
    xhr.withCredentials = true;
    xhr.timeout = 120_000;
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) progress(Math.min(100, Math.round(event.loaded / event.total * 100)));
    };
    xhr.onload = () => {
      finish();
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new ApiError(xhr.status, xhr.status === 503
          ? 'Upload may be saved, but processing is unavailable. Do not resend automatically.'
          : xhr.responseText.trim() || 'Upload failed.')); return;
      }
      try {
        const result = JSON.parse(xhr.responseText) as UploadResult;
        if (typeof result.id !== 'string' || !result.id || result.status !== 'pending') throw new Error();
        resolve(result);
      } catch { reject(new ApiError(502, 'Upload response was invalid. Check the conversation before resending.')); }
    };
    xhr.onerror = xhr.ontimeout = () => { finish(); reject(new ApiError(0, 'Upload interrupted. Check the conversation before resending.')); };
    xhr.onabort = () => { finish(); reject(new DOMException('Upload cancelled', 'AbortError')); };
    signal.addEventListener('abort', abort, { once: true });
    const body = new FormData(); body.append('file', file); xhr.send(body);
  });
}
