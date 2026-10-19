import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { useUploads } from './useUploads';
import { AttachmentComposer } from './AttachmentComposer';
import { VoiceRecorder } from './VoiceRecorder';
import type { ApiClient } from '../api/client';

beforeEach(() => {
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const image = () => new File(['x'.repeat(200)], 'photo.png', { type: 'image/png' });

it('previews a picked image, reports upload progress, waits for ready, and revokes its URL', async () => {
  let complete!: (value: { id: string; kind: string; status: 'pending' }) => void;
  const upload = vi.fn((_room: string, _file: File, progress: (value: number) => void) => {
    progress(45); return new Promise<{ id: string; kind: string; status: 'pending' }>((resolve) => { complete = resolve; });
  });
  const api = { upload } as unknown as ApiClient;
  function Harness({ statuses = {} }: { statuses?: Record<string, 'ready' | 'rejected'> }) {
    const uploads = useUploads(api);
    return <AttachmentComposer roomID="room-1" uploads={uploads} statuses={statuses} ready />;
  }
  const view = render(<Harness />);
  fireEvent.change(screen.getByLabelText('Choose attachments'), { target: { files: [image()] } });
  expect(screen.getByAltText('Preview of photo.png').getAttribute('src')).toBe('blob:preview');
  fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));
  expect((screen.getByRole('progressbar') as HTMLProgressElement).value).toBe(45);
  await act(async () => complete({ id: 'a1', kind: 'image', status: 'pending' }));
  expect(screen.getByText(/Processing…/)).toBeTruthy(); expect(screen.queryByText('Ready · sent')).toBeNull();
  view.rerender(<Harness statuses={{ a1: 'ready' }} />); expect(screen.getByText('Ready · sent')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Dismiss photo.png' })); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview');
});

it('retains uploads across room changes and aborts/releases resources on unmount', async () => {
  let signal!: AbortSignal;
  const upload = vi.fn((_room: string, _file: File, _progress: unknown, value: AbortSignal) => {
    signal = value; return new Promise(() => {});
  });
  const { result, unmount } = renderHook(() => useUploads({ upload } as unknown as ApiClient));
  act(() => result.current.add('room-1', [image()]));
  const first = result.current.items[0]; act(() => { void result.current.send(first, false); });
  await waitFor(() => expect(upload).toHaveBeenCalledOnce());
  act(() => result.current.add('room-2', [image()])); expect(result.current.items.map((item) => item.roomID)).toEqual(['room-1', 'room-2']);
  unmount(); expect(signal.aborted).toBe(true); expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2);
});

it('shows rejected files without offering another automatic upload', () => {
  const api = { upload: vi.fn() } as unknown as ApiClient;
  function Harness() { const uploads = useUploads(api); return <AttachmentComposer roomID="room" uploads={uploads} statuses={{}} ready />; }
  render(<Harness />); fireEvent.change(screen.getByLabelText('Choose attachments'), { target: { files: [new File(['abc'], 'app.exe')] } });
  expect(screen.getByRole('alert').textContent).toContain('not supported'); expect(screen.queryByText('Send attachment')).toBeNull();
});

class Recorder {
  static isTypeSupported = (type: string) => type.startsWith('audio/webm');
  static latest: Recorder;
  state = 'inactive'; ondataavailable?: (event: { data: Blob }) => void; onstop?: () => void; onerror?: () => void;
  constructor() { Recorder.latest = this; }
  start() { this.state = 'recording'; }
  stop() { this.state = 'inactive'; this.ondataavailable?.({ data: new Blob(['x'.repeat(200)]) }); this.onstop?.(); }
}

it('records a supported voice file and stops microphone tracks', async () => {
  const stop = vi.fn(); const input = { getTracks: () => [{ stop }] };
  vi.stubGlobal('MediaRecorder', Recorder);
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: vi.fn().mockResolvedValue(input) } });
  const onFile = vi.fn(); render(<VoiceRecorder onFile={onFile} />);
  fireEvent.click(screen.getByRole('button', { name: /Record voice/ }));
  await screen.findByRole('button', { name: 'Stop recording' });
  fireEvent.click(screen.getByRole('button', { name: 'Stop recording' }));
  expect(onFile).toHaveBeenCalledOnce(); expect(onFile.mock.calls[0][0].name).toMatch(/\.webm$/); expect(stop).toHaveBeenCalledOnce();
});

it('releases microphone permission results arriving after unmount', async () => {
  let grant!: (value: unknown) => void;
  const stop = vi.fn(); vi.stubGlobal('MediaRecorder', Recorder);
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: () => new Promise((resolve) => { grant = resolve; }) } });
  const onFile = vi.fn(); const view = render(<VoiceRecorder onFile={onFile} />);
  fireEvent.click(screen.getByRole('button', { name: /Record voice/ })); view.unmount();
  await act(async () => grant({ getTracks: () => [{ stop }] }));
  expect(stop).toHaveBeenCalledOnce(); expect(onFile).not.toHaveBeenCalled();
});
