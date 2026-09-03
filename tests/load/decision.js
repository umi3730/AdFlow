import http from 'k6/http';
import { check } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const baseURL = __ENV.BASE_URL || 'http://localhost:18080';
const decisionDuration = new Trend('decision_duration', true);
const decisionErrors = new Rate('decision_errors');

export const options = {
  stages: [
    { duration: '10s', target: 20 },
    { duration: '20s', target: 50 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    decision_duration: ['p(95)<80', 'p(99)<150'],
    decision_errors: ['rate<0.01'],
  },
};

const jsonHeaders = { headers: { 'Content-Type': 'application/json' } };

export function setup() {
  const now = Date.now();
  const campaign = http.post(`${baseURL}/v1/campaigns`, JSON.stringify({
    name: `Load Test ${now}`,
    slotId: 'load-test-banner',
    startAt: new Date(now - 60000).toISOString(),
    endAt: new Date(now + 3600000).toISOString(),
  }), jsonHeaders).json();

  http.post(`${baseURL}/v1/campaigns/${campaign.id}/creatives`, JSON.stringify({
    title: 'Load Test Banner',
    description: 'Synthetic performance test creative',
    imageUrl: 'https://example.com/banner.png',
    landingUrl: 'https://example.com/game',
  }), jsonHeaders);

  http.post(`${baseURL}/v1/campaigns/${campaign.id}/publish`, JSON.stringify({
    targeting: { all: [{ tag: 'load_test' }] },
    dailyBudgetFen: 100000000,
    impressionCostFen: 1,
    frequencyLimit: 100,
  }), jsonHeaders);
  return { campaignId: campaign.id };
}

export default function () {
  const userId = `load-user-${__VU}-${__ITER}`;
  http.put(`${baseURL}/v1/profiles/${userId}`, JSON.stringify({ tags: ['load_test'], fields: { device: 'test' } }), jsonHeaders);

  const started = Date.now();
  const response = http.post(`${baseURL}/v1/decisions`, JSON.stringify({
    requestId: `load-request-${__VU}-${__ITER}-${Date.now()}`,
    userId,
    slotId: 'load-test-banner',
  }), jsonHeaders);
  decisionDuration.add(Date.now() - started);
  const valid = check(response, {
    'decision status is 200': (res) => res.status === 200,
    'decision matched': (res) => res.json('matched') === true,
  });
  decisionErrors.add(!valid);
}
