import { afterEach, describe, expect, it, onTestFinished, vi } from 'vitest';
import { ChatSocket } from './ChatSocket';
import { sampleMessage, TestSocket } from './testSocket';
import { compareMessages, mergeMessages, parseEvent } from './types';

function setup() {
  vi.useFakeTimers(); TestSocket.instances = [];
  const session = { getAccessToken: vi.fn().mockResolvedValue('first-token'), refresh: vi.fn().mockResolvedValue('fresh-token') };
  const handlers = { status: vi.fn(), event: vi.fn(), connected: vi.fn() };
  const client = new ChatSocket(session, handlers, {
    create: (url) => new TestSocket(url) as unknown as WebSocket,
    random: () => 0.5, origin: 'https://chat.test',
  });
  onTestFinished(() => client.stop());
  return { client, session, handlers };
}
const tick = () => vi.advanceTimersByTimeAsync(0);
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); });

describe('socket lifecycle', () => {
  it('authenticates using WSS, sends text, and pings every 25 seconds', async () => {
    const { client, handlers } = setup(); client.start(); await tick();
    const socket = TestSocket.instances[0];
    expect(new URL(socket.url).protocol).toBe('wss:');
    expect(new URL(socket.url).searchParams.get('access_token')).toBe('first-token');
    expect(client.send('room-1', 'hello')).toBe(false);
    socket.open(); expect(handlers.status).toHaveBeenLastCalledWith('connected');
    expect(client.send('room-1', 'hello')).toBe(true);
    expect(JSON.parse(socket.sent[0])).toEqual({ type: 'message', room_id: 'room-1', body: 'hello' });
    await vi.advanceTimersByTimeAsync(24_999); expect(socket.sent).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1); expect(JSON.parse(socket.sent[1])).toEqual({ type: 'ping' });
    socket.receive({ type: 'pong' });
    await vi.advanceTimersByTimeAsync(10_000); expect(socket.readyState).toBe(1);
    socket.bufferedAmount = 65_537; expect(client.send('room-1', 'no queue')).toBe(false);
  });

  it('closes a silent peer and reconnects after the pong deadline', async () => {
    const { client, handlers } = setup(); client.start(); await tick();
    const socket = TestSocket.instances[0]; socket.open();
    await vi.advanceTimersByTimeAsync(35_000);
    expect(socket.readyState).toBe(3); expect(handlers.status).toHaveBeenLastCalledWith('reconnecting');
    await vi.advanceTimersByTimeAsync(750); expect(TestSocket.instances).toHaveLength(2);
  });

  it('allows active replay traffic while the application pong is queued', async () => {
    const { client } = setup(); client.start(); await tick();
    const socket = TestSocket.instances[0]; socket.open();
    await vi.advanceTimersByTimeAsync(34_000);
    socket.receive({ ...sampleMessage(), type: 'message' });
    await vi.advanceTimersByTimeAsync(2000); expect(socket.readyState).toBe(1);
    socket.receive({ type: 'pong' });
    await vi.advanceTimersByTimeAsync(10_000); expect(socket.readyState).toBe(1);
  });

  it('ignores malformed frames without letting them poison the replay cursor', async () => {
    const { client, handlers } = setup(); client.start(); await tick();
    const socket = TestSocket.instances[0]; socket.open();
    socket.receive('{bad JSON');
    socket.receive({ ...sampleMessage(), type: 'message', id: '' });
    socket.receive({ type: 'presence', user_id: 9, online: true });
    expect(handlers.event).not.toHaveBeenCalled();
  });

  it('uses exponential backoff with jitter and caps the delay', async () => {
    const { client } = setup(); client.start(); await tick();
    for (const [index, delay] of [750, 1500, 3000, 6000, 12000, 22500, 22500].entries()) {
      const current = TestSocket.instances.at(-1)!;
      current.open(); current.drop();
      await vi.advanceTimersByTimeAsync(delay - 1); expect(TestSocket.instances).toHaveLength(index + 1);
      await vi.advanceTimersByTimeAsync(1); expect(TestSocket.instances).toHaveLength(index + 2);
    }
  });

  it('keeps a monotonic replay cursor and refreshes an expired socket token', async () => {
    const { client, session } = setup(); client.start(); await tick();
    const socket = TestSocket.instances[0]; socket.open();
    const latest = sampleMessage('00000000-0000-4000-8000-000000000002', '2026-10-19T12:00:00.000002Z');
    socket.receive({ ...latest, type: 'message', message_type: 'text' });
    socket.receive({ ...sampleMessage(), type: 'message' });
    socket.receive({ type: 'attachment_status', attachment_id: 'not-a-message', status: 'ready' });
    socket.drop(1008, 'access token expired'); await vi.advanceTimersByTimeAsync(750);
    const url = new URL(TestSocket.instances[1].url);
    expect(url.searchParams.get('last_message_id')).toBe(latest.id);
    expect(url.searchParams.get('access_token')).toBe('fresh-token');
    expect(session.refresh).toHaveBeenCalledWith('first-token');
  });

  it('seeds a history cursor only once and stops all reconnect work on cleanup', async () => {
    const { client } = setup();
    client.seedCursor(sampleMessage()); client.seedCursor(sampleMessage('later-history'));
    client.start(); await tick();
    expect(new URL(TestSocket.instances[0].url).searchParams.get('last_message_id')).toBe(sampleMessage().id);
    TestSocket.instances[0].drop(); client.stop();
    await vi.advanceTimersByTimeAsync(120_000); expect(TestSocket.instances).toHaveLength(1);
  });

  it('pauses offline and immediately resumes when the network returns', async () => {
    const { client, handlers } = setup(); client.start(false); await tick();
    expect(TestSocket.instances).toHaveLength(0); expect(handlers.status).toHaveBeenLastCalledWith('offline');
    client.setOnline(true); await tick(); TestSocket.instances[0].open();
    client.setOnline(false); await vi.advanceTimersByTimeAsync(60_000); expect(TestSocket.instances).toHaveLength(1);
    client.setOnline(true); await tick(); expect(TestSocket.instances).toHaveLength(2);
  });

  it('replays from a history seed that arrives after the first reconnection', async () => {
    const { client } = setup(); client.start(); await tick();
    TestSocket.instances[0].open(); TestSocket.instances[0].drop();
    await vi.advanceTimersByTimeAsync(750); TestSocket.instances[1].open();
    client.seedCursor(sampleMessage());
    await vi.advanceTimersByTimeAsync(1500);
    expect(new URL(TestSocket.instances[2].url).searchParams.get('last_message_id')).toBe(sampleMessage().id);
  });

  it('does not open a socket when authentication resolves after cleanup', async () => {
    const { client, session } = setup();
    let release!: (value: string) => void;
    session.getAccessToken.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    client.start(); client.stop(); release('late-token'); await tick();
    expect(TestSocket.instances).toHaveLength(0);
  });
});

it('deduplicates replay/history overlap and orders PostgreSQL microsecond timestamps', () => {
  const first = sampleMessage();
  const second = sampleMessage('00000000-0000-4000-8000-000000000002', '2026-10-19T12:00:00.000002Z');
  expect(compareMessages(first, second)).toBeLessThan(0);
  expect(mergeMessages([second, first], [first])).toEqual([first, second]);
});

it('refreshes before every reconnect even when the old token has not expired', async () => {
  const { client, session } = setup(); client.start(); await tick();
  TestSocket.instances[0].open(); TestSocket.instances[0].drop();
  await vi.advanceTimersByTimeAsync(750);
  expect(session.refresh).toHaveBeenCalledTimes(1);
  expect(new URL(TestSocket.instances[1].url).searchParams.get('access_token')).toBe('fresh-token');
  TestSocket.instances[1].open(); client.setOnline(false); client.setOnline(true); await tick();
  expect(session.refresh).toHaveBeenCalledTimes(2);
});

it('does not reconnect until refresh succeeds and cancels a pending refresh on stop', async () => {
  const { client, session } = setup(); client.start(); await tick();
  TestSocket.instances[0].open(); TestSocket.instances[0].drop();
  let resolve!: (token: string) => void;
  session.refresh.mockImplementation(() => new Promise((done) => { resolve = done; }));
  await vi.advanceTimersByTimeAsync(750); expect(TestSocket.instances).toHaveLength(1);
  client.stop(); resolve('late'); await tick(); expect(TestSocket.instances).toHaveLength(1);
});

it.each([
  [{ type: 'read', room_id: 'room-1', user_id: 'other', message_id: 'm1' }, true],
  [{ type: 'read', room_id: '', user_id: 'other', message_id: 'm1' }, false],
  [{ type: 'read', room_id: 'room-1', user_id: 4, message_id: 'm1' }, false],
])('validates read event fields: %j', (event, valid) => {
  expect(parseEvent(JSON.stringify(event)) !== null).toBe(valid);
});
