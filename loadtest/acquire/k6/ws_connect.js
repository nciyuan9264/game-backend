import ws from 'k6/ws';
import { check } from 'k6';

export const options = {
  vus: Number(__ENV.VUS || 30),
  duration: __ENV.DURATION || '5m',
};

const wsBase = (__ENV.ACQUIRE_WS_BASE || 'ws://127.0.0.1:8000').replace(/\/$/, '');
const rooms = (__ENV.ROOMS || '').split(',').map((v) => v.trim()).filter(Boolean);
const prefix = __ENV.PREFIX || 'lt-k6';

export default function () {
  if (rooms.length === 0) {
    throw new Error('ROOMS env is required, comma separated room IDs');
  }

  const roomID = rooms[(__VU - 1) % rooms.length];
  const userID = `${prefix}-vu-${__VU}`;
  const url = `${wsBase}/ws?roomID=${encodeURIComponent(roomID)}&userID=${encodeURIComponent(userID)}`;

  const response = ws.connect(url, {}, (socket) => {
    socket.on('message', () => {});
    socket.on('error', (error) => {
      console.error(`ws error room=${roomID} user=${userID}: ${error.error()}`);
    });
    socket.setTimeout(() => {
      socket.close();
    }, 1000 * Number(__ENV.SOCKET_SECONDS || 120));
  });

  check(response, {
    'ws status is 101': (r) => r && r.status === 101,
  });
}
