import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MessageList } from './MessageList';
import { sampleMessage } from './testSocket';

afterEach(cleanup);

it('anchors the visible message when a live append overlaps an older-history prepend', () => {
  const older = sampleMessage('older', '2026-10-19T11:59:00Z');
  const current = sampleMessage();
  const later = sampleMessage('later', '2026-10-19T12:01:00Z');
  const props = { messages: [current], loading: false, initialized: true, more: true, error: '', online: {},
    loadOlder: vi.fn().mockResolvedValue(undefined), retry: vi.fn().mockResolvedValue(undefined) };
  const { rerender } = render(<MessageList {...props} />);
  const log = screen.getByRole('log');
  const article = log.querySelector('article')!;
  let messageTop = 80;
  let height = 1000;
  vi.spyOn(log, 'getBoundingClientRect').mockImplementation(() => ({ top: 100, bottom: 300 } as DOMRect));
  vi.spyOn(article, 'getBoundingClientRect').mockImplementation(() => ({ top: messageTop, bottom: messageTop + 60 } as DOMRect));
  Object.defineProperty(log, 'scrollHeight', { get: () => height });
  Object.defineProperty(log, 'clientHeight', { value: 200 });
  log.scrollTop = 20;
  fireEvent.scroll(log);
  expect(props.loadOlder).toHaveBeenCalledTimes(1);
  rerender(<MessageList {...props} loading />);
  height = 1200;
  rerender(<MessageList {...props} messages={[current, later]} loading />);
  messageTop += 140;
  height = 1340;
  rerender(<MessageList {...props} messages={[older, current, later]} />);
  // Only the prepended 140px shifts the anchor, not the 200px live append.
  expect(log.scrollTop).toBe(160);
});
