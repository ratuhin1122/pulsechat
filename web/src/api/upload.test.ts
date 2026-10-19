import { expect, it, vi } from 'vitest';
import { uploadFile } from './upload';

class FakeXHR {
  upload = { onprogress: null as ((event: { loaded: number; total: number; lengthComputable: boolean }) => void) | null };
  open = vi.fn(); setRequestHeader = vi.fn(); send = vi.fn();
  abort = vi.fn(() => this.onabort?.());
  onload?: () => void; onerror?: () => void; ontimeout?: () => void; onabort?: () => void;
  responseText = ''; status = 202;
}
const file = new File(['x'.repeat(100)], 'photo.png', { type: 'image/png' });
function setup() {
  const xhr = new FakeXHR(); const controller = new AbortController(); const progress = vi.fn();
  const result = uploadFile('room/one', file, 'secret', progress, controller.signal, () => xhr as unknown as XMLHttpRequest);
  return { xhr, controller, progress, result };
}

it('sends one multipart file with bearer authentication and reports actual progress', async () => {
  const { xhr, progress, result } = setup();
  expect(xhr.open).toHaveBeenCalledWith('POST', '/rooms/room%2Fone/upload');
  expect(xhr.setRequestHeader).toHaveBeenCalledExactlyOnceWith('Authorization', 'Bearer secret');
  const body = xhr.send.mock.calls[0][0] as unknown as FormData;
  expect([...body.keys()]).toEqual(['file']); expect((body.get('file') as File).name).toBe('photo.png');
  xhr.upload.onprogress?.({ loaded: 40, total: 100, lengthComputable: true }); expect(progress).toHaveBeenCalledWith(40);
  xhr.responseText = JSON.stringify({ id: 'attachment-1', kind: 'image', status: 'pending' }); xhr.onload?.();
  expect(await result).toEqual({ id: 'attachment-1', kind: 'image', status: 'pending' });
});

it.each([401, 413, 503])('returns HTTP %i without automatically resending', async (status) => {
  const { xhr, result } = setup(); xhr.status = status; xhr.responseText = 'server error'; xhr.onload?.();
  await expect(result).rejects.toMatchObject({ status }); expect(xhr.send).toHaveBeenCalledTimes(1);
});

it('aborts an active request', async () => {
  const { result, controller, xhr } = setup(); controller.abort();
  await expect(result).rejects.toMatchObject({ name: 'AbortError' }); expect(xhr.abort).toHaveBeenCalledOnce();
});

it('does not treat malformed success as completed', async () => {
  const { result, xhr } = setup(); xhr.responseText = '{}'; xhr.onload?.();
  await expect(result).rejects.toMatchObject({ status: 502 });
});
