import http from 'k6/http';
import execution from 'k6/execution';
import { Rate, Counter } from 'k6/metrics';

const rate = Number(__ENV.RATE), seconds = Number(__ENV.SECONDS);
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(__ENV.BASE_URL || '') || !Number.isInteger(rate) || rate < 1 || rate > 2000 || !Number.isInteger(seconds) || seconds < 5 || seconds > 120)
  throw new Error('Invalid isolated decision workload');
const errors = new Rate('decision_errors');
const matched = new Counter('matched_decisions');
const noAd = new Counter('no_ad');
const badAuction = new Counter('unexpected_auction');
const httpErrors = new Counter('decision_http_errors');
export const options = {
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  scenarios: { decision: { executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration: `${seconds}s`, preAllocatedVUs: 256, maxVUs: 512, gracefulStop: '5s' } },
  thresholds: {
    http_req_duration: ['p(95)<300'], decision_errors: ['rate<0.001'],
    dropped_iterations: ['count==0'], matched_decisions: ['count>0'],
  },
};
export default function() {
  const index = execution.scenario.iterationInTest;
  const userId = `profile-${String(index % 1000).padStart(4, '0')}`;
  const requestId = `dc-${__ENV.STAGE}-${index}`;
  const response = http.post(`${__ENV.BASE_URL}/v1/decisions`, JSON.stringify({ requestId, userId, slotId: 'decision-capacity-slot' }), {
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${__ENV.ADFLOW_CAPACITY_TOKEN}` }, timeout: '2s', tags: { name: 'POST decisions' },
  });
  let result;
  try { result = response.json(); } catch (_) { result = null; }
  if (response.status !== 200) { httpErrors.add(1, { status: String(response.status) }); errors.add(true); return; }
  if (!result?.matched) { noAd.add(1, { reason: result?.reason || 'invalid_body' }); errors.add(true); return; }
  if (result.requestId !== requestId || result.campaignId !== __ENV.WINNER || !result.creativeId || result.pricing?.mode !== 'first_price' || result.pricing?.priceFen !== 5 || result.pricing?.advertisers !== 3) {
    badAuction.add(1); errors.add(true); return;
  }
  matched.add(1); errors.add(false);
}
export function handleSummary(data) {
  const lines = ['Decision capacity: unique requests, real matched auction, no event calls'];
  for (const name of ['http_reqs','http_req_duration','decision_errors','decision_http_errors','matched_decisions','no_ad','unexpected_auction','dropped_iterations','vus'])
    if (data.metrics[name]) lines.push(`${name}: ${JSON.stringify(data.metrics[name])}`);
  return { [__ENV.REPORT_PATH]: JSON.stringify(data,null,2), stdout: lines.join('\n')+'\n' };
}
