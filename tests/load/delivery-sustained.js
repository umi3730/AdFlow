import http from 'k6/http';
import exec from 'k6/execution';
import { check, fail, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const baseURL = (__ENV.BASE_URL || 'http://localhost:18080').replace(/\/$/, '');
const campaignCount = positiveInteger('CAMPAIGN_COUNT', 20);
const profileCount = positiveInteger('PROFILE_COUNT', 1000);
const targetRate = positiveInteger('TARGET_RATE', 200);
const preAllocatedVUs = positiveInteger('PRE_ALLOCATED_VUS', 100);
const maxVUs = positiveInteger('MAX_VUS', 500);
const warmupDuration = __ENV.WARMUP_DURATION || '30s';
const steadyDuration = __ENV.STEADY_DURATION || '2m';
const cooldownDuration = __ENV.COOLDOWN_DURATION || '15s';
const sendImpressions = (__ENV.SEND_IMPRESSIONS || 'true').toLowerCase() !== 'false';
const seedData = (__ENV.SEED_DATA || 'true').toLowerCase() !== 'false';
const thinkTimeMs = nonNegativeInteger('THINK_TIME_MS', 0);
const p95TargetMs = positiveInteger('P95_TARGET_MS', 100);
const p99TargetMs = positiveInteger('P99_TARGET_MS', 200);
const maxErrorRate = positiveNumber('MAX_ERROR_RATE', 0.01);

const decisionDuration = new Trend('decision_duration', true);
const eventDuration = new Trend('event_duration', true);
const fullPathDuration = new Trend('full_path_duration', true);
const decisionErrors = new Rate('decision_errors');
const eventErrors = new Rate('event_errors');
const fullPathErrors = new Rate('full_path_errors');
const matchedDecisions = new Counter('matched_decisions');
const acceptedImpressions = new Counter('accepted_impressions');

export const options = {
  setupTimeout: __ENV.SETUP_TIMEOUT || '5m',
  scenarios: {
    sustained_delivery: {
      executor: 'ramping-arrival-rate',
      startRate: Math.max(1, Math.floor(targetRate / 10)),
      timeUnit: '1s',
      preAllocatedVUs,
      maxVUs,
      stages: [
        { target: targetRate, duration: warmupDuration },
        { target: targetRate, duration: steadyDuration },
        { target: 0, duration: cooldownDuration },
      ],
      gracefulStop: '10s',
    },
  },
  thresholds: {
    decision_duration: [`p(95)<${p95TargetMs}`, `p(99)<${p99TargetMs}`],
    decision_errors: [`rate<${maxErrorRate}`],
    event_errors: [`rate<${maxErrorRate}`],
    full_path_errors: [`rate<${maxErrorRate}`],
    dropped_iterations: ['count==0'],
  },
};

const jsonHeaders = { headers: { 'Content-Type': 'application/json' } };

export function setup() {
  const live = http.get(`${baseURL}/livez`, { tags: { name: 'GET /livez [setup]' } });
  requireStatus(live, [200], 'liveness check');

  const runID = (__ENV.RUN_ID || `${Date.now()}`).replace(/[^a-zA-Z0-9_-]/g, '-');
  const slotID = `load-slot-${runID}`.slice(0, 64);
  const segments = [];
  for (let index = 0; index < campaignCount; index += 1) {
    const segment = `load-segment-${runID}-${index}`.slice(0, 64);
    segments.push(segment);
    if (seedData) {
      seedCampaign(runID, slotID, segment, index);
    }
  }

  const userIDs = [];
  for (let index = 0; index < profileCount; index += 1) {
    const userID = `load-user-${runID}-${index}`;
    if (seedData) {
      const response = http.put(
        `${baseURL}/v1/profiles/${userID}`,
        JSON.stringify({
          tags: [segments[index % segments.length]],
          fields: { device: index % 2 === 0 ? 'ios' : 'android', cohort: `${index % 10}` },
        }),
        { ...jsonHeaders, tags: { name: 'PUT /v1/profiles/:id [setup]' } },
      );
      requireStatus(response, [204], `create profile ${index}`);
    }
    userIDs.push(userID);
  }
  return { runID, slotID, userIDs };
}

export default function (data) {
  const started = Date.now();
  const iteration = exec.scenario.iterationInTest;
  const userID = data.userIDs[iteration % data.userIDs.length];
  const requestID = `load-${data.runID}-${exec.vu.idInTest}-${iteration}-${Date.now()}`;
  const decision = http.post(
    `${baseURL}/v1/decisions`,
    JSON.stringify({ requestId: requestID, userId: userID, slotId: data.slotID }),
    { ...jsonHeaders, tags: { name: 'POST /v1/decisions' } },
  );
  decisionDuration.add(decision.timings.duration);

  let decisionBody;
  try {
    decisionBody = decision.json();
  } catch (_) {
    decisionBody = null;
  }
  const decisionOK = check(decision, {
    'decision returns 200': (response) => response.status === 200,
    'decision matches a campaign': () => decisionBody?.matched === true,
    'decision contains delivery identifiers': () => Boolean(decisionBody?.campaignId && decisionBody?.creativeId),
  });
  decisionErrors.add(!decisionOK);
  if (!decisionOK) {
    fullPathErrors.add(true);
    return;
  }
  matchedDecisions.add(1);

  if (sendImpressions) {
    const event = http.post(
      `${baseURL}/v1/events`,
      JSON.stringify({
        eventId: `impression-${requestID}`,
        requestId: requestID,
        campaignId: decisionBody.campaignId,
        creativeId: decisionBody.creativeId,
        type: 'impression',
      }),
      { ...jsonHeaders, tags: { name: 'POST /v1/events' } },
    );
    eventDuration.add(event.timings.duration);
    const eventOK = check(event, {
      'impression is accepted': (response) => response.status === 201 || response.status === 202,
    });
    eventErrors.add(!eventOK);
    fullPathErrors.add(!eventOK);
    if (eventOK) {
      acceptedImpressions.add(1);
    }
  } else {
    fullPathErrors.add(false);
  }

  fullPathDuration.add(Date.now() - started);
  if (thinkTimeMs > 0) {
    sleep(thinkTimeMs / 1000);
  }
}

function seedCampaign(runID, slotID, segment, index) {
  const now = Date.now();
  const campaignResponse = http.post(
    `${baseURL}/v1/campaigns`,
    JSON.stringify({
      name: `Sustained Load ${runID} ${index}`.slice(0, 128),
      slotId: slotID,
      startAt: new Date(now - 60000).toISOString(),
      endAt: new Date(now + 4 * 60 * 60 * 1000).toISOString(),
    }),
    { ...jsonHeaders, tags: { name: 'POST /v1/campaigns [setup]' } },
  );
  requireStatus(campaignResponse, [201], `create campaign ${index}`);
  const campaign = campaignResponse.json();

  const creativeResponse = http.post(
    `${baseURL}/v1/campaigns/${campaign.id}/creatives`,
    JSON.stringify({
      title: `Load Creative ${index}`,
      description: 'Synthetic full-path performance test creative',
      imageUrl: 'https://example.com/load.png',
      landingUrl: 'https://example.com/game',
    }),
    { ...jsonHeaders, tags: { name: 'POST /v1/campaigns/:id/creatives [setup]' } },
  );
  requireStatus(creativeResponse, [201], `create creative ${index}`);

  const publishResponse = http.post(
    `${baseURL}/v1/campaigns/${campaign.id}/publish`,
    JSON.stringify({
      targeting: { all: [{ tag: segment }] },
      dailyBudgetFen: 1000000000000,
      impressionCostFen: 1,
      frequencyLimit: 100,
    }),
    { ...jsonHeaders, tags: { name: 'POST /v1/campaigns/:id/publish [setup]' } },
  );
  requireStatus(publishResponse, [200], `publish campaign ${index}`);
}

function requireStatus(response, expected, operation) {
  if (!expected.includes(response.status)) {
    fail(`${operation} failed: status=${response.status} body=${response.body}`);
  }
}

function positiveInteger(name, fallback) {
  const value = Number(__ENV[name] || fallback);
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return value;
}

function nonNegativeInteger(name, fallback) {
  const value = Number(__ENV[name] || fallback);
  if (!Number.isInteger(value) || value < 0) {
    throw new Error(`${name} must be a non-negative integer`);
  }
  return value;
}

function positiveNumber(name, fallback) {
  const value = Number(__ENV[name] || fallback);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be positive`);
  }
  return value;
}
