import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || '5m',
};

const httpBase = (__ENV.ACQUIRE_HTTP_BASE || 'http://127.0.0.1:8000').replace(/\/$/, '');
const roomID = __ENV.ROOM_ID || '';
const cookie = __ENV.COOKIE || '';

function params() {
  if (!cookie) return {};
  return {
    headers: {
      Cookie: cookie,
    },
  };
}

export default function () {
  const listResp = http.get(`${httpBase}/room/list`, params());
  check(listResp, {
    'room list status is 200 or auth-blocked': (r) => r.status === 200 || r.status === 401,
  });

  if (roomID) {
    const statusResp = http.get(`${httpBase}/room/game_status?room_id=${encodeURIComponent(roomID)}`, params());
    check(statusResp, {
      'game status status is 200 or auth-blocked': (r) => r.status === 200 || r.status === 401,
    });
  }

  const rankingResp = http.get(`${httpBase}/ranking/leaderboard`);
  check(rankingResp, {
    'leaderboard status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
