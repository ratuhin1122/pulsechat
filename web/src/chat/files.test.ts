import { afterEach, expect, it, vi } from 'vitest';
import { compressImage, duration, fileKind, fileSize, validateFile } from './files';
import { parseEvent } from './types';

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });
it.each([
  ['photo.JPG', 'image'], ['animation.gif', 'image'], ['report.pdf', 'document'], ['archive.zip', 'document'],
  ['memo.webm', 'voice'], ['memo.mp3', 'voice'],
])('classifies %s as %s', (name, expected) => expect(fileKind(new File([], name))).toBe(expected));

it.each(['script.js', 'app.exe', 'notes.html', 'voice.m4a'])('rejects unsupported %s', (name) => {
  expect(() => validateFile(new File(['x'.repeat(100)], name))).toThrow('not supported');
});

it.each([['photo.png', 15], ['report.pdf', 25], ['voice.ogg', 10]])('enforces limits for %s', (name, mb) => {
  const file = new File(['x'.repeat(100)], name);
  expect(() => validateFile(file)).not.toThrow();
  Object.defineProperty(file, 'size', { configurable: true, value: mb as number * 1024 * 1024 + 1 });
  expect(() => validateFile(file)).toThrow('smaller');
  expect(() => validateFile(new File(['tiny'], name))).toThrow('100 bytes');
});

it('formats sizes and durations', () => {
  expect(fileSize(2048)).toBe('2 KB'); expect(fileSize(1572864)).toBe('1.5 MB');
  expect(duration(125.5)).toBe('2:05'); expect(duration(Infinity)).toBe('0:00');
});

it('preserves GIFs and falls back when image compression fails', async () => {
  const bitmap = vi.fn().mockRejectedValue(new Error('decode failed')); vi.stubGlobal('createImageBitmap', bitmap);
  const gif = new File(['gif'], 'animation.gif'); expect(await compressImage(gif)).toBe(gif); expect(bitmap).not.toHaveBeenCalled();
  const png = new File(['png'], 'photo.png'); expect(await compressImage(png)).toBe(png);
});

it('resizes optional compression to 1600px and releases the bitmap', async () => {
  const close = vi.fn(); const drawImage = vi.fn();
  vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ width: 3200, height: 2400, close }));
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (this: HTMLCanvasElement, callback) {
    expect(this.width).toBe(1600); expect(this.height).toBe(1200);
    callback(new Blob(['x'.repeat(150)], { type: 'image/webp' }));
  });
  const result = await compressImage(new File(['x'.repeat(300)], 'photo.png'));
  expect(result.name).toBe('photo.webp'); expect(result.size).toBe(150); expect(drawImage).toHaveBeenCalledOnce(); expect(close).toHaveBeenCalledOnce();
});

it.each(['ready', 'rejected'])('parses attachment status %s independently of message replay', (status) => {
  const event = { type: 'attachment_status', attachment_id: 'attachment-1', status };
  expect(parseEvent(JSON.stringify(event))).toEqual(event);
  expect(parseEvent(JSON.stringify({ ...event, status: 'pending' }))).toBeNull();
});
