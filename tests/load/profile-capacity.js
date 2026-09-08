import http from 'k6/http';
import execution from 'k6/execution';
import { Rate, Counter } from 'k6/metrics';

const rate = Number(__ENV.RATE);
const seconds = Number(__ENV.SECONDS);
const base = __ENV.BASE_URL;
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base || '') || !Number.isInteger(rate) || rate < 1 || rate > 16000 || !Number.isInteger(seconds) || seconds < 5 || seconds > 120)
  throw new Error('Bounded isolated local configuration required');
const errors = new Rate('profile_errors');
const valid = new Counter('valid_profiles');

export const options = {
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  scenarios: { capacity: { executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration: `${seconds}s`, preAllocatedVUs: 256, maxVUs: 512, gracefulStop: '5s' } },
  thresholds: {
    http_req_duration: ['p(95)<100'],
    profile_errors: ['rate<0.001'],
    dropped_iterations: ['count==0'],
    valid_profiles: ['count>0'],
  },
};
export default function() {
  const user = `profile-${String(execution.scenario.iterationInTest % 1000).padStart(4, '0')}`;
  const res = http.get(`${base}/v1/profiles/${user}`, { headers: { Authorization: `Bearer ${__ENV.ADFLOW_CAPACITY_TOKEN}` }, tags: { name: 'GET profile' }, timeout: '2s' });
  let profile;
  try { profile = res.json(); } catch (_) { profile = null; }
  const ok = res.status === 200 && profile?.userId === user && profile?.fields?.score === '88';
  errors.add(!ok);
  if (ok) valid.add(1);
}
export function handleSummary(data) {
  const text = ['Profile capacity probe'];
  for (const name of ['http_reqs','http_req_duration','http_req_failed','profile_errors','valid_profiles','dropped_iterations','vus'])
    if (data.metrics[name]) text.push(`${name}: ${JSON.stringify(data.metrics[name])}`);
  return { [__ENV.REPORT_PATH]: JSON.stringify(data, null, 2), stdout: text.join('\n') + '\n' };
}
