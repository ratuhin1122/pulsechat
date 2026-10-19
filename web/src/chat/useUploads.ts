import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiClient } from '../api/client';
import { compressImage, fileKind, validateFile } from './files';

export interface UploadDraft {
  key: string; roomID: string; file: File; preview?: string;
  state: 'draft' | 'uploading' | 'processing' | 'error'; progress: number; id?: string; error?: string;
}

export function useUploads(api: ApiClient) {
  const [items, setItems] = useState<UploadDraft[]>([]);
  const active = useRef(new Map<string, AbortController>());
  const previews = useRef(new Map<string, string>());
  useEffect(() => () => {
    active.current.forEach((controller) => controller.abort()); active.current.clear();
    previews.current.forEach((url) => URL.revokeObjectURL(url)); previews.current.clear();
  }, []);
  const add = useCallback((roomID: string, files: File[]) => {
    const next = files.slice(0, 10).map((file): UploadDraft => {
      const key = crypto.randomUUID();
      try {
        validateFile(file);
        const kind = fileKind(file);
        const preview = kind !== 'document' ? URL.createObjectURL(file) : undefined;
        if (preview) previews.current.set(key, preview);
        return { key, roomID, file, preview, state: 'draft', progress: 0 };
      } catch (error) { return { key, roomID, file, state: 'error', progress: 0, error: (error as Error).message }; }
    });
    setItems((previous) => [...previous, ...next]);
  }, []);
  const remove = useCallback((key: string) => {
    active.current.get(key)?.abort(); active.current.delete(key);
    const url = previews.current.get(key); if (url) URL.revokeObjectURL(url);
    previews.current.delete(key); setItems((previous) => previous.filter((item) => item.key !== key));
  }, []);
  const send = useCallback(async (item: UploadDraft, compress: boolean) => {
    if (item.state !== 'draft' || active.current.has(item.key)) return;
    const controller = new AbortController(); active.current.set(item.key, controller);
    const update = (patch: Partial<UploadDraft>) => {
      if (!controller.signal.aborted) setItems((previous) => previous.map((entry) => entry.key === item.key ? { ...entry, ...patch } : entry));
    };
    update({ state: 'uploading' });
    try {
      const file = compress ? await compressImage(item.file) : item.file;
      controller.signal.throwIfAborted();
      const result = await api.upload(item.roomID, file, (progress) => update({ progress }), controller.signal);
      update({ id: result.id, state: 'processing', progress: 100 });
    } catch (error) { update({ state: 'error', error: error instanceof Error ? error.message : 'Upload failed.' }); }
    finally { active.current.delete(item.key); }
  }, [api]);
  return { items, add, remove, send };
}
