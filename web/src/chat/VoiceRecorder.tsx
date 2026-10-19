import { useEffect, useRef, useState } from 'react';
import { duration } from './files';

export function VoiceRecorder({ onFile }: { onFile: (file: File) => void }) {
  const [state, setState] = useState<'idle' | 'requesting' | 'recording'>('idle');
  const [seconds, setSeconds] = useState(0);
  const [error, setError] = useState('');
  const recorder = useRef<MediaRecorder | null>(null);
  const stream = useRef<MediaStream | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | undefined>(undefined);
  const generation = useRef(0);
  function release() {
    clearInterval(timer.current); stream.current?.getTracks().forEach((track) => track.stop()); stream.current = null;
  }
  function cancel() {
    generation.current++; if (recorder.current?.state === 'recording') recorder.current.stop();
    recorder.current = null; release(); setState('idle');
  }
  useEffect(() => () => {
    generation.current++; if (recorder.current?.state === 'recording') recorder.current.stop(); release();
  }, []);
  async function start() {
    setError('');
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      setError('Voice recording is unavailable in this browser. You can attach an audio file.'); return;
    }
    const mime = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus', 'audio/webm', 'audio/ogg']
      .find((type) => MediaRecorder.isTypeSupported(type));
    if (!mime) { setError('This browser cannot record WebM or Ogg audio. Attach an audio file instead.'); return; }
    const current = ++generation.current; setState('requesting');
    try {
      const input = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (current !== generation.current) { input.getTracks().forEach((track) => track.stop()); return; }
      stream.current = input;
      const recording = new MediaRecorder(input, { mimeType: mime }); recorder.current = recording;
      const chunks: Blob[] = []; let bytes = 0; let oversized = false;
      recording.ondataavailable = (event) => {
        bytes += event.data.size;
        if (bytes > 10 * 1024 * 1024) { oversized = true; if (recording.state === 'recording') recording.stop(); }
        else chunks.push(event.data);
      };
      recording.onerror = () => { if (current === generation.current) { cancel(); setError('Recording failed. Please try again.'); } };
      recording.onstop = () => {
        if (current !== generation.current) return;
        release(); recorder.current = null; setState('idle');
        if (oversized) { setError('Recording exceeds 10 MB. Please record a shorter message.'); return; }
        const blob = new Blob(chunks, { type: mime });
        if (blob.size < 100) { setError('Recording was too short. Please try again.'); return; }
        onFile(new File([blob], `voice-${Date.now()}.${mime.includes('ogg') ? 'ogg' : 'webm'}`, { type: mime }));
      };
      recording.start(1000); setSeconds(0); setState('recording');
      const began = Date.now();
      timer.current = setInterval(() => {
        const elapsed = Math.floor((Date.now() - began) / 1000); setSeconds(elapsed);
        // Leave a small margin for encoder/container padding under the server's 300s cap.
        if (elapsed >= 299 && recording.state === 'recording') recording.stop();
      }, 250);
    } catch { if (current === generation.current) { release(); setState('idle'); setError('Microphone access failed. Check your browser permissions.'); } }
  }
  return <div className="voice-recorder">
    {state === 'idle' ? <button type="button" onClick={() => void start()}>● Record voice</button>
      : <><span role="status">{state === 'requesting' ? 'Waiting for microphone…' : `Recording ${duration(seconds)} / 5:00`}</span>
        {state === 'recording' && <button type="button" onClick={() => recorder.current?.stop()}>Stop recording</button>}
        <button type="button" onClick={cancel}>Cancel recording</button></>}
    {error && <span role="alert" className="error-message">{error}</span>}
  </div>;
}
