import http from 'k6/http';
import { check } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const baseURL = __ENV.BASE_URL || 'http://localhost:18080';
const startRate = Number(__ENV.START_RATE || 100);
const spikeRate = Number(__ENV.SPIKE_RATE || 2000);
const expectedRejections = new Rate('expected_admission_rejections');
const unexpectedResponses = new Rate('unexpected_responses');
const decisionDuration = new Trend('decision_duration', true);
const jsonHeaders = { headers: { 'Content-Type': 'application/json' } };

export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-arrival-rate',
      startRate,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 500,
      stages: [
        { target: startRate, duration: '5s' },
        { target: spikeRate, duration: '3s' },
        { target: spikeRate, duration: '10s' },
        { target: startRate, duration: '5s' },
      ],
    },
  },
  thresholds: {
    unexpected_responses: ['rate<0.001'],
    decision_duration: ['p(99)<500'],
  },
};

export function setup() {
  const now = Date.now();
  const campaign = http
    .post(
      `${baseURL}/v1/campaigns`,
      JSON.stringify({
        name: `Admission Spike ${now}`,
        slotId: 'spike-test-banner',
        startAt: new Date(now - 60000).toISOString(),
        endAt: new Date(now + 3600000).toISOString(),
      }),
      jsonHeaders,
    )
    .json();
  http.post(
    `${baseURL}/v1/campaigns/${campaign.id}/creatives`,
    JSON.stringify({
      title: 'Spike Test Banner',
      imageUrl: 'https://example.com/spike.png',
      landingUrl: 'https://example.com/game',
    }),
    jsonHeaders,
  );
  http.post(
    `${baseURL}/v1/campaigns/${campaign.id}/publish`,
    JSON.stringify({
      targeting: { all: [{ tag: 'spike_test' }] },
      dailyBudgetFen: 100000000,
      impressionCostFen: 1,
      frequencyLimit: 100,
    }),
    jsonHeaders,
  );
  http.put(
    `${baseURL}/v1/profiles/spike-user`,
    JSON.stringify({ tags: ['spike_test'], fields: { device: 'test' } }),
    jsonHeaders,
  );
}

export default function () {
  const started = Date.now();
  const response = http.post(
    `${baseURL}/v1/decisions`,
    JSON.stringify({
      requestId: `spike-${__VU}-${__ITER}-${Date.now()}`,
      userId: 'spike-user',
      slotId: 'spike-test-banner',
    }),
    jsonHeaders,
  );
  decisionDuration.add(Date.now() - started);
  const rejected = response.status === 429 || response.status === 503;
  const expected = response.status === 200 || rejected;
  expectedRejections.add(rejected);
  unexpectedResponses.add(!expected);
  check(response, {
    'decision is accepted or explicitly backpressured': () => expected,
    'decision never returns an internal error': (res) => res.status !== 500,
  });
}
