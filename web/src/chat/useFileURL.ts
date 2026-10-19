import { useEffect, useState } from 'react';
import { useAuth } from '../auth/AuthProvider';

export function useFileURL(id: string, variant: 'thumb' | 'display' | 'original', enabled = true) {
  const { api } = useAuth();
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState({ url: '', error: '' });
  useEffect(() => {
    setResult({ url: '', error: '' });
    if (!enabled) return;
    const controller = new AbortController(); let url = '';
    void api.file(id, variant, controller.signal).then((blob) => {
      if (controller.signal.aborted) return;
      url = URL.createObjectURL(blob); setResult({ url, error: '' });
    }).catch(() => { if (!controller.signal.aborted) setResult({ url: '', error: 'Could not load this file.' }); });
    return () => { controller.abort(); if (url) URL.revokeObjectURL(url); };
  }, [api, id, variant, enabled, attempt]);
  return { ...result, retry: () => setAttempt((value) => value + 1) };
}
