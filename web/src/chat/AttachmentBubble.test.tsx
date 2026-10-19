import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AttachmentBubble } from './AttachmentBubble';
import type { Attachment } from './types';

const { file, api } = vi.hoisted(() => { const file = vi.fn(); return { file, api: { file } }; });
vi.mock('../auth/AuthProvider', () => ({ useAuth: () => ({ api }) }));
const image: Attachment = { id: 'a1', kind: 'image', original_name: 'photo.png', mime_type: 'image/webp', size_bytes: 200,
  width: 800, height: 600, blurhash: 'LEHV6nWB2yk8pyo0adR*.7kCMdnj' };

beforeEach(() => {
  let sequence = 0;
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => `blob:media-${++sequence}`) });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
  file.mockReset().mockResolvedValue(new Blob(['media']));
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  HTMLDialogElement.prototype.showModal = vi.fn(function (this: HTMLDialogElement) { this.setAttribute('open', ''); });
  HTMLDialogElement.prototype.close = vi.fn(function (this: HTMLDialogElement) { this.removeAttribute('open'); });
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it('shows a blurhash canvas, loads authenticated thumbnails, and zooms with display media', async () => {
  const view = render(<AttachmentBubble attachment={image} />);
  expect(view.container.querySelector('canvas')).toBeTruthy();
  await waitFor(() => expect(screen.getByAltText('photo.png').getAttribute('src')).toBe('blob:media-1'));
  expect(file).toHaveBeenCalledWith('a1', 'thumb', expect.any(AbortSignal));
  fireEvent.click(screen.getByRole('button', { name: 'Zoom photo.png' }));
  await waitFor(() => expect(file).toHaveBeenCalledWith('a1', 'display', expect.any(AbortSignal)));
  expect(HTMLDialogElement.prototype.showModal).toHaveBeenCalledOnce();
  await waitFor(() => expect(screen.getAllByAltText('photo.png')).toHaveLength(2));
  fireEvent.click(screen.getByRole('button', { name: 'Close image' }));
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:media-2');
  view.unmount(); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:media-1');
});

it('downloads a document on demand and displays its filename and size', async () => {
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  render(<AttachmentBubble attachment={{ ...image, kind: 'document', original_name: 'report.pdf', size_bytes: 2048 }} />);
  expect(screen.getByText('report.pdf')).toBeTruthy(); expect(screen.getByText('2 KB')).toBeTruthy(); expect(file).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Download' }));
  const link = await screen.findByRole('link', { name: 'Download' });
  expect(link.getAttribute('download')).toBe('report.pdf'); expect(link.getAttribute('href')).toBe('blob:media-1');
  expect(click).toHaveBeenCalledOnce(); expect(file).toHaveBeenCalledWith('a1', 'original', expect.any(AbortSignal));
});

it('renders playable voice with server duration and releases its blob URL', async () => {
  const view = render(<AttachmentBubble attachment={{ ...image, kind: 'voice', original_name: 'voice.ogg', duration_sec: 65 }} />);
  expect(screen.getByText('Voice message · 1:05')).toBeTruthy();
  await waitFor(() => expect(view.container.querySelector('audio')?.getAttribute('src')).toBe('blob:media-1'));
  expect(view.container.querySelector('audio')?.controls).toBe(true);
  view.unmount(); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:media-1');
});

it('offers retry after a failed authenticated media fetch', async () => {
  file.mockRejectedValueOnce(new Error('offline'));
  render(<AttachmentBubble attachment={{ ...image, kind: 'voice' }} />);
  fireEvent.click(await screen.findByRole('button', { name: 'Retry audio' }));
  await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalledOnce());
});
