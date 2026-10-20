import { render, screen, cleanup } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MessageList } from './MessageList';
afterEach(cleanup);
it('distinguishes own left messages from other right messages', () => {
  const base = { room_id: 'room', type: 'text' as const, created_at: '2026-10-21T00:00:00Z' };
  render(<MessageList userID="self" messages={[{ ...base, id: 'own', sender_id: 'self', body: 'My message' }, { ...base, id: 'other', sender_id: 'peer', body: 'Their message' }]}
    loading={false} initialized more={false} error="" online={{}} loadOlder={vi.fn()} retry={vi.fn()} />);
  expect(screen.getByText('My message').closest('article')?.classList.contains('is-own')).toBe(true);
  expect(screen.getByText('Their message').closest('article')?.classList.contains('is-other')).toBe(true);
});
