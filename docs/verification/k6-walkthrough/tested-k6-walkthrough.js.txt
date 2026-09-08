import http from 'k6/http';
import exec from 'k6/execution';
import { check, fail } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const base = __ENV.BASE_URL;
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base || '')) throw new Error('Use the isolated loopback lab runner');
const seconds = Number(__ENV.SECONDS || 20);
const rate = Number(__ENV.RATE || 30);
if (!Number.isInteger(seconds) || seconds < 5 || seconds > 120 || !Number.isInteger(rate) || rate < 10 || rate > 100)
  throw new Error('SECONDS must be 5..120 and RATE 10..100');

const decisionMs = new Trend('decision_ms', true);
const acceptedMs = new Trend('business_accepted_ms', true);
const errors = new Rate('business_errors');
const matched = new Counter('matched_decisions');
const eventCount = new Counter('accepted_events');
const completed = new Counter('accepted_rounds');

const arrival = (rate, duration, startTime, vus) => ({
  executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration,
  startTime, preAllocatedVUs: vus, maxVUs: vus * 2, gracefulStop: '10s',
});

export const options = {
  setupTimeout: '3m',
  summaryTrendStats: ['avg', 'min', 'med', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    smoke: arrival(1, '5s', '0s', 2),
    baseline: arrival(10, `${seconds}s`, '7s', 10),
    moderate: arrival(rate, `${seconds}s`, `${9 + seconds}s`, 30),
  },
  thresholds: {
    checks: ['rate==1'],
    business_errors: ['rate==0'],
    dropped_iterations: ['count==0'],
    'decision_ms{phase:baseline}': ['p(95)<300', 'p(99)<1000'],
    'decision_ms{phase:moderate}': ['p(95)<300', 'p(99)<1000'],
    'business_accepted_ms{phase:baseline}': ['p(95)<1000'],
    'business_accepted_ms{phase:moderate}': ['p(95)<1000'],
    // Explicit submetrics keep per-phase counts visible in the JSON summary.
    'accepted_rounds{phase:smoke}': ['count>0'],
    'accepted_rounds{phase:baseline}': ['count>0'],
    'accepted_rounds{phase:moderate}': ['count>0'],
  },
};

function params(token, name, phase = 'setup') {
  return { headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    tags: { name, phase }, timeout: '5s' };
}
function requireCode(response, code, label) {
  if (response.status !== code) fail(`${label}: expected ${code}, got ${response.status}`);
}
export function setup() {
  const login = http.post(`${base}/v1/auth/login`, JSON.stringify({ username: 'admin', password: 'adflow-admin' }), params('', 'login'));
  requireCode(login, 200, 'login');
  const token = login.json('accessToken');
  const runId = `k6-${Date.now()}`;
  const slot = `${runId}-slot`;
  let winner;
  for (const bid of [2, 3, 5]) {
    const created = http.post(`${base}/v1/campaigns`, JSON.stringify({ name: `${runId} bid ${bid}`, slotId: slot,
      startAt: new Date(Date.now() - 60000).toISOString(), endAt: new Date(Date.now() + 3600000).toISOString() }), params(token, 'create campaign'));
    requireCode(created, 201, 'campaign');
    const id = created.json('id');
    const creative = http.post(`${base}/v1/campaigns/${id}/creatives`, JSON.stringify({ title: 'k6 sample', description: 'Isolated load fixture',
      imageUrl: 'https://example.com/image.png', landingUrl: 'https://example.com/' }), params(token, 'create creative'));
    requireCode(creative, 201, 'creative');
    const publish = http.post(`${base}/v1/campaigns/${id}/publish`, JSON.stringify({ targeting: { all: [{ tag: runId }] },
      dailyBudgetFen: 10000000, impressionCostFen: bid, frequencyLimit: 100,
      auction: { advertiserId: `bidder-${bid}`, advertiserName: `Bidder ${bid}`, bidFen: bid } }), params(token, 'publish campaign'));
    requireCode(publish, 200, 'publish');
    if (bid === 5) winner = id;
  }
  const users = [];
  for (let i = 0; i < 200; i++) {
    const user = `${runId}-user-${i}`;
    requireCode(http.put(`${base}/v1/profiles/${user}`, JSON.stringify({ tags: [runId], fields: { device: 'android', age: '25', score: '88' } }), params(token, 'create profile')), 204, 'profile');
    requireCode(http.get(`${base}/v1/profiles/${user}`, params(token, 'warm profile')), 200, 'profile warmup');
    users.push(user);
  }
  return { token, runId, slot, winner, users };
}

export default function(data) {
  const phase = exec.scenario.name;
  const tags = { phase };
  const i = exec.scenario.iterationInTest;
  const requestId = `${data.runId}-${phase}-${i}`;
  const started = Date.now();
  const response = http.post(`${base}/v1/decisions`, JSON.stringify({ requestId, userId: data.users[i % data.users.length], slotId: data.slot }), params(data.token, 'POST decisions', phase));
  decisionMs.add(response.timings.duration, tags);
  let decision;
  try { decision = response.json(); } catch (_) { decision = null; }
  const ok = check(response, {
    'decision HTTP 200': r => r.status === 200,
    'highest bidder selected at 5 fen': () => decision?.matched === true && decision?.campaignId === data.winner && decision?.pricing?.mode === 'first_price' && decision?.pricing?.priceFen === 5,
  }, tags);
  if (!ok) { errors.add(true, tags); return; }
  matched.add(1, tags);
  for (const type of ['impression', 'click', 'conversion']) {
    const event = http.post(`${base}/v1/events`, JSON.stringify({ eventId: `${type}-${requestId}`, requestId,
      campaignId: decision.campaignId, creativeId: decision.creativeId, type, ...(type === 'conversion' ? { valueFen: 500 } : {}) }), params(data.token, `POST events ${type}`, phase));
    const accepted = check(event, { 'new event HTTP 202': r => r.status === 202 }, tags);
    if (!accepted) { errors.add(true, tags); return; }
    eventCount.add(1, tags);
  }
  errors.add(false, tags);
  completed.add(1, tags);
  acceptedMs.add(Date.now() - started, tags);
  // No sleep: the arrival-rate executor controls when a new round starts.
}

export function handleSummary(data) {
  const names = ['iterations', 'http_reqs', 'checks', 'business_errors', 'dropped_iterations', 'matched_decisions', 'accepted_events',
    'accepted_rounds{phase:smoke}', 'accepted_rounds{phase:baseline}', 'accepted_rounds{phase:moderate}',
    'decision_ms{phase:baseline}', 'decision_ms{phase:moderate}',
    'business_accepted_ms{phase:baseline}', 'business_accepted_ms{phase:moderate}'];
  const lines = ['AdFlow k6 walkthrough (HTTP acceptance only; async proof follows)'];
  for (const name of names) if (data.metrics[name]) lines.push(`${name}: ${JSON.stringify(data.metrics[name])}`);
  return { [__ENV.REPORT_PATH || 'k6-summary.json']: JSON.stringify(data, null, 2), stdout: lines.join('\n') + '\n' };
}
