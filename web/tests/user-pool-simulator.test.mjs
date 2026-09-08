import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import {
  makeSimulationProfiles,
  localSimulationTarget,
  percentile,
  startSimulation,
  validateSimulationConfig,
} from '../lib/user-pool-simulator.ts';
import { validateFieldCondition } from '../lib/profile-options.ts';
import {
  simulationMoneyFen,
  simulationBehaviorForRequest,
  validateSimulationBehavior,
} from '../lib/simulation-behavior.ts';

const fullBehavior = {
  clickPercent: 100,
  conversionPercent: 100,
  minValueFen: 990,
  maxValueFen: 19900,
};

test('Synthetic conversion amounts are bounded integer cents, varied and reproducible', () => {
  const values = new Set();
  for (let i = 0; i < 200; i++) {
    const result = simulationBehaviorForRequest(fullBehavior, 'run-' + i);
    assert.equal(result.click, true);
    assert.ok(Number.isInteger(result.valueFen));
    assert.ok(result.valueFen >= 990 && result.valueFen <= 19900);
    assert.deepEqual(
      result,
      simulationBehaviorForRequest(fullBehavior, 'run-' + i),
    );
    values.add(result.valueFen);
  }
  assert.ok(values.size > 100);
  assert.deepEqual(
    simulationBehaviorForRequest({ ...fullBehavior, clickPercent: 0 }, 'a'),
    { click: false, valueFen: null },
  );
  assert.deepEqual(
    simulationBehaviorForRequest(
      { ...fullBehavior, conversionPercent: 0 },
      'a',
    ),
    { click: true, valueFen: null },
  );
  assert.equal(
    simulationBehaviorForRequest(
      { ...fullBehavior, minValueFen: 1, maxValueFen: 1 },
      'a',
    ).valueFen,
    1,
  );
  assert.equal(simulationMoneyFen('9.90'), 990);
  assert.equal(simulationMoneyFen('0.29'), 29);
  for (const invalid of [
    '',
    '-1',
    '1.001',
    'NaN',
    'Infinity',
    '1e3',
    '10000.01',
  ])
    assert.throws(() => simulationMoneyFen(invalid));
  for (const change of [
    { clickPercent: 101 },
    { conversionPercent: -1 },
    { minValueFen: 19901 },
    { maxValueFen: 1000001 },
    { minValueFen: 1.1 },
  ])
    assert.throws(() =>
      validateSimulationBehavior({ ...fullBehavior, ...change }),
    );
  assert.throws(() =>
    validateSimulationConfig({ ...config, behavior: fullBehavior }),
  );
});

test('Matched rounds send impression, click, conversion in order and count accepted value', async () => {
  const clock = new Clock();
  const events = [];
  const behavior = { ...fullBehavior, minValueFen: 29, maxValueFen: 29 };
  const handle = startSimulation(
    { ...config, maxRounds: 1, impressions: true, behavior },
    {
      decide: async (input) => ({
        ...noAd(input),
        matched: true,
        requestId: 'server-id',
        campaignId: 'auction-winner',
        pricing: {
          mode: 'first_price',
          advertiserName: '风铃互动',
          priceFen: 5,
        },
      }),
      impression: async (decision, id) => {
        events.push(['impression', decision.requestId, id]);
      },
      click: async (decision, id) => {
        events.push(['click', decision.requestId, id]);
      },
      conversion: async (decision, id, value) => {
        events.push(['conversion', decision.requestId, id, value]);
      },
    },
    undefined,
    clock,
  );
  behavior.maxValueFen = 1000000;
  await clock.advance(1200);
  const result = await handle.done;
  assert.deepEqual(
    events.map((event) => event[0]),
    ['impression', 'click', 'conversion'],
  );
  assert.ok(events.every((event) => event[1] === 'server-id'));
  assert.equal(new Set(events.map((event) => event[2])).size, 3);
  assert.equal(events[2][3], 29);
  assert.equal(result.httpRequests, 4);
  assert.equal(result.clicksAccepted, 1);
  assert.equal(result.conversionsAccepted, 1);
  assert.equal(result.valueFenAccepted, 29);
  assert.equal(result.recent[0].campaignId, 'auction-winner');
  assert.equal(result.recent[0].advertiserName, '风铃互动');
  assert.equal(result.recent[0].priceFen, 5);
});

test('Failed earlier stages never send downstream events or count unpaid conversions', async () => {
  for (const failure of ['noAd', 'impression', 'click', 'conversion']) {
    const clock = new Clock();
    const events = [];
    const send = async (phase) => {
      events.push(phase);
      if (phase === failure) throw { status: 503 };
    };
    const handle = startSimulation(
      { ...config, maxRounds: 1, impressions: true, behavior: fullBehavior },
      {
        decide: async (input) => ({
          ...noAd(input),
          matched: failure !== 'noAd',
        }),
        impression: () => send('impression'),
        click: () => send('click'),
        conversion: () => send('conversion'),
      },
      undefined,
      clock,
    );
    await clock.advance(1200);
    const result = await handle.done;
    const expected =
      failure === 'noAd'
        ? []
        : ['impression', 'click', 'conversion'].slice(
            0,
            ['impression', 'click', 'conversion'].indexOf(failure) + 1,
          );
    assert.deepEqual(events, expected);
    assert.equal(result.valueFenAccepted, 0);
    assert.equal(result.conversionsAccepted, 0);
    if (failure !== 'noAd') assert.equal(result.errors[failure + ':503'], 1);
  }
});

test('Conversion occupies concurrency slot and cancellation preserves accepted earlier events', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    {
      ...config,
      mode: 'concurrency',
      maxRounds: 4,
      impressions: true,
      behavior: fullBehavior,
    },
    {
      decide: async (input) => ({ ...noAd(input), matched: true }),
      impression: async () => {},
      click: async () => {},
      conversion: (_, __, ___, signal) => delay(clock, 900, signal, undefined),
    },
    undefined,
    clock,
  );
  await flush();
  await clock.advance(50);
  assert.equal(handle.snapshot().started, 2);
  assert.equal(handle.snapshot().inFlight, 2);
  handle.stop();
  await flush();
  const result = await handle.done;
  assert.equal(result.clicksAccepted, 2);
  assert.equal(result.impressionsAccepted, 2);
  assert.equal(result.conversionsAccepted, 0);
  assert.equal(result.valueFenAccepted, 0);
  assert.equal(result.canceled, 2);
  assert.equal(clock.tasks.size, 0);
});
import { simulationProgress } from '../lib/simulation-presentation.ts';

test('Fixed-concurrency progress separates actual elapsed time from dispatch limits', () => {
  const config = { seconds: 30, maxRounds: 1000 };
  assert.deepEqual(
    simulationProgress(config, {
      elapsedMs: 1247,
      started: 1000,
      status: 'completed',
    }),
    {
      primary: '已发 1000 / 1000 轮',
      secondary: '实际用时 1.2 秒 · 最长派发 30 秒',
    },
  );
  assert.match(simulationProgress(config, null).secondary, /已用时 0.0 秒/);
});

test('Historical simulation result uses captured run config rather than editable next-run controls', () => {
  const source = readFileSync(
    new URL('../components/simulation-results.tsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /simulationProgress\(\s*runConfig \?\?/);
});

async function flush() {
  for (let i = 0; i < 12; i++) await Promise.resolve();
}
class Clock {
  time = 0;
  sequence = 0;
  tasks = new Map();
  now = () => this.time;
  later = (callback, delay) => {
    const id = ++this.sequence;
    this.tasks.set(id, { at: this.time + delay, callback });
    return id;
  };
  cancel = (id) => this.tasks.delete(id);
  async advance(ms) {
    const target = this.time + ms;
    let count = 0;
    while (true) {
      const next = [...this.tasks]
        .filter(([, task]) => task.at <= target)
        .sort((a, b) => a[1].at - b[1].at || a[0] - b[0])[0];
      if (!next) break;
      if (++count > 100000) throw new Error('timer loop');
      this.time = next[1].at;
      this.tasks.delete(next[0]);
      next[1].callback();
      await flush();
    }
    this.time = target;
    await flush();
  }
  async jump(ms) {
    this.time += ms;
    const due = [...this.tasks].filter(([, task]) => task.at <= this.time);
    for (const [id, task] of due) {
      if (this.tasks.delete(id)) task.callback();
    }
    await flush();
  }
}
const config = {
  runId: 'sim-test',
  userIds: ['one', 'two'],
  slotId: 'banner',
  maxRounds: 4,
  concurrency: 2,
  seconds: 1,
  timeoutMs: 1000,
  impressions: false,
};
const noAd = (input) => ({
  ...input,
  matched: false,
  reason: 'targeting_miss',
  campaignId: '',
  creativeId: '',
  reservationToken: '',
});
function delay(clock, ms, signal, value) {
  return new Promise((resolve, reject) => {
    const abort = () => {
      clock.cancel(timer);
      reject(new DOMException('Stopped', 'AbortError'));
    };
    const timer = clock.later(() => {
      signal.removeEventListener('abort', abort);
      resolve(value);
    }, ms);
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
  });
}

test('Limits reject unbounded workloads and remote targets', () => {
  for (const values of [
    { concurrency: 101 },
    { seconds: 121 },
    { maxRounds: 3001 },
    { timeoutMs: 0 },
    { userIds: [] },
    { slotId: ' ' },
  ])
    assert.throws(() => validateSimulationConfig({ ...config, ...values }));
  assert.deepEqual(
    validateSimulationConfig({ ...config, userIds: ['one', 'one'] }).userIds,
    ['one'],
  );
  assert.equal(localSimulationTarget('http://127.0.0.1:18080'), true);
  assert.equal(localSimulationTarget('http://[::1]:18080'), true);
  for (const url of [
    'https://example.com',
    'http://localhost.evil',
    'http://user:pass@localhost',
    'file:///tmp/a',
  ])
    assert.equal(localSimulationTarget(url), false);
});

test('Fixed scheduler rotates users, uses unique IDs and counts No-Ad as success', async () => {
  const clock = new Clock();
  const calls = [];
  const handle = startSimulation(
    config,
    {
      decide: async (input) => {
        calls.push(input);
        return noAd(input);
      },
      impression: async () => assert.fail('unexpected impression'),
    },
    undefined,
    clock,
  );
  await clock.advance(1200);
  const result = await handle.done;
  assert.equal(result.status, 'completed');
  assert.equal(result.started, 4);
  assert.equal(result.successful, 4);
  assert.equal(result.failed, 0);
  assert.equal(result.noAd, 4);
  assert.deepEqual(
    calls.map((call) => call.userId),
    ['one', 'two', 'one', 'two'],
  );
  assert.equal(new Set(calls.map((call) => call.requestId)).size, 4);
  assert.equal(clock.tasks.size, 0);
});

test('Manual stop aborts in-flight requests and never dispatches more', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, maxRounds: 10 },
    {
      decide: (input, signal) => delay(clock, 900, signal, noAd(input)),
      impression: async () => {},
    },
    undefined,
    clock,
  );
  await clock.advance(150);
  handle.stop();
  await flush();
  const result = await handle.done;
  assert.equal(result.status, 'stopped');
  assert.equal(result.started, 2);
  assert.equal(result.canceled, 2);
  assert.equal(result.failed, 0);
  await clock.advance(5000);
  assert.equal(handle.snapshot().started, 2);
  assert.equal(clock.tasks.size, 0);
});

test('Request deadlines count as failures, not manual cancellation', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, maxRounds: 2, timeoutMs: 100 },
    {
      decide: (input, signal) => delay(clock, 900, signal, noAd(input)),
      impression: async () => {},
    },
    undefined,
    clock,
  );
  await clock.advance(1500);
  const result = await handle.done;
  assert.equal(result.timeouts, 2);
  assert.equal(result.failed, 2);
  assert.equal(result.canceled, 0);
  assert.equal(result.errors['decision:timeout'], 2);
});

test('Impressions follow matching decisions and their failures remain distinct', async () => {
  const clock = new Clock();
  const order = [];
  const handle = startSimulation(
    { ...config, maxRounds: 2, concurrency: 1, impressions: true },
    {
      decide: async (input) => {
        order.push('decision');
        return {
          ...noAd(input),
          matched: input.userId === 'one',
          campaignId: 'campaign',
          creativeId: 'creative',
        };
      },
      impression: async () => {
        order.push('impression');
        throw { status: 503 };
      },
    },
    undefined,
    clock,
  );
  await clock.advance(1200);
  const result = await handle.done;
  assert.deepEqual(order, ['decision', 'impression', 'decision']);
  assert.equal(result.matched, 1);
  assert.equal(result.noAd, 1);
  assert.equal(result.httpRequests, 3);
  assert.equal(result.failed, 1);
  assert.equal(result.successful, 1);
  assert.equal(result.errors['impression:503'], 1);
});

test('Accepted impression is counted', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, maxRounds: 1, impressions: true },
    {
      decide: async (input) => ({ ...noAd(input), matched: true }),
      impression: async () => {},
    },
    undefined,
    clock,
  );
  await clock.advance(1200);
  const result = await handle.done;
  assert.equal(result.impressionsAccepted, 1);
  assert.equal(result.successful, 1);
  assert.equal(result.httpRequests, 2);
});

test('Cancellation during impression preserves matched count without reporting success', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, maxRounds: 1, impressions: true },
    {
      decide: async (input) => ({ ...noAd(input), matched: true }),
      impression: (_, __, signal) => delay(clock, 900, signal, undefined),
    },
    undefined,
    clock,
  );
  await flush();
  await clock.advance(10);
  handle.stop();
  await flush();
  const result = await handle.done;
  assert.equal(result.matched, 1);
  assert.equal(result.canceled, 1);
  assert.equal(result.successful, 0);
  assert.equal(result.impressionsAccepted, 0);
});

test('Reports and normalized config cannot be mutated by consumers', async () => {
  const clock = new Clock();
  const input = { ...config, userIds: ['one', 'two'] };
  const called = [];
  const handle = startSimulation(
    input,
    {
      decide: async (request) => {
        called.push(request.userId);
        return noAd(request);
      },
      impression: async () => {},
    },
    undefined,
    clock,
  );
  input.userIds[1] = 'modified';
  await clock.advance(1200);
  const report = await handle.done;
  report.reasons.targeting_miss = 999;
  report.recent[0].userId = 'modified';
  assert.equal(handle.snapshot().reasons.targeting_miss, 4);
  assert.ok(handle.snapshot().recent.every((row) => row.userId !== 'modified'));
  assert.deepEqual(called, ['one', 'two', 'one', 'two']);
});

test('Temporary pool uses standard short IDs and is generated without any persistence', () => {
  const profiles = makeSimulationProfiles(20);
  assert.equal(new Set(profiles.map((profile) => profile.userId)).size, 20);
  assert.equal(profiles[0].userId, 'user-0001');
  assert.equal(profiles[19].userId, 'user-0020');
  assert.deepEqual(profiles, makeSimulationProfiles(20));
  for (const profile of profiles)
    for (const [field, value] of Object.entries(profile.fields))
      validateFieldCondition(field, 'eq', value);
  assert.throws(() => makeSimulationProfiles(101));
});

test('UI separates temporary and saved profiles and never persists generated users', () => {
  const source = readFileSync(
    new URL('../components/user-pool-simulation.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(source, /api\.putProfile|seedSimulationProfiles/);
  assert.match(source, /api\.simulateDecision/);
  assert.match(source, /poolSource === 'saved'/);
  assert.match(source, /temporary: \{ profiles: \[\], selectedIds: \[\] \}/);
  assert.match(source, /不写入画像表/);
});

test('Station navigation keeps the simulator mounted, with global stop and bounded refresh', () => {
  const source = readFileSync(
    new URL('../components/user-pool-simulation.tsx', import.meta.url),
    'utf8',
  );
  const shell = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(source, /stop\('left_page'\)/);
  assert.match(source, /active && poolSource === 'saved' && !runner.current/);
  assert.match(source, /stop\('unmounted'\)/);
  assert.match(source, /stopSignal > 0/);
  assert.match(shell, /hidden=\{view !== 'simulation'\}/);
  assert.match(shell, /onRunningChange=\{handleSimulationRunningChange\}/);
  assert.match(shell, /setTimeout\(poll, 1000\)/);
  assert.doesNotMatch(shell, /总览与计划统计每/);
  assert.match(shell, /if \(refreshTask.current\)/);
  assert.match(shell, /停止模拟/);
  assert.match(shell, /setSimulationRefreshUntil\(Date.now\(\) \+ 15000\)/);
});

test('Percentile calculation does not mutate inputs', () => {
  const values = [30, 10, 20, 40];
  assert.equal(percentile(values, 50), 20);
  assert.equal(percentile(values, 95), 40);
  assert.equal(percentile([], 99), 0);
  assert.deepEqual(values, [30, 10, 20, 40]);
});

test('Fixed concurrency fills immediately, replenishes, rotates and stops at exact round limit', async () => {
  const clock = new Clock();
  const calls = [];
  const handle = startSimulation(
    { ...config, mode: 'concurrency', concurrency: 3, maxRounds: 8 },
    {
      decide: (input, signal) => {
        calls.push(input);
        return delay(clock, 50, signal, noAd(input));
      },
      impression: async () => {},
    },
    undefined,
    clock,
  );
  assert.equal(handle.snapshot().inFlight, 3);
  await clock.advance(52);
  assert.equal(handle.snapshot().started, 6);
  assert.equal(handle.snapshot().inFlight, 3);
  await clock.advance(1000);
  const result = await handle.done;
  assert.equal(result.started, 8);
  assert.equal(result.successful, 8);
  assert.equal(result.peakInFlight, 3);
  assert.equal(result.stopReason, 'round_limit');
  assert.equal(result.status, 'completed');
  assert.equal(new Set(calls.map((call) => call.requestId)).size, 8);
  assert.deepEqual(
    calls.map((call) => call.userId),
    ['one', 'two', 'one', 'two', 'one', 'two', 'one', 'two'],
  );
  assert.equal(clock.tasks.size, 0);
});

test('Fixed concurrency deadline drains active rounds without refilling', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, mode: 'concurrency', maxRounds: 100 },
    {
      decide: (input, signal) => delay(clock, 600, signal, noAd(input)),
      impression: async () => {},
    },
    undefined,
    clock,
  );
  await clock.advance(1000);
  assert.equal(handle.snapshot().status, 'stopping');
  assert.equal(handle.snapshot().started, 4);
  await clock.advance(1000);
  const result = await handle.done;
  assert.equal(result.stopReason, 'duration');
  assert.equal(result.successful, 4);
  assert.equal(result.canceled, 0);
  assert.equal(clock.tasks.size, 0);
});

test('Fixed concurrency manual/hidden/navigation stop cancels without replenishing', async () => {
  for (const reason of ['manual', 'hidden', 'left_page', 'unmounted']) {
    const clock = new Clock();
    const handle = startSimulation(
      { ...config, mode: 'concurrency' },
      {
        decide: (input, signal) => delay(clock, 500, signal, noAd(input)),
        impression: async () => {},
      },
      undefined,
      clock,
    );
    handle.stop(reason);
    await clock.advance(3000);
    const result = await handle.done;
    assert.equal(result.started, 2);
    assert.equal(result.canceled, 2);
    assert.equal(result.stopReason, reason);
    assert.equal(result.status, 'stopped');
    assert.equal(clock.tasks.size, 0);
  }
});

test('Fixed concurrency holds its slot through impression, handles timeouts and synchronous errors', async () => {
  for (const kind of ['impression', 'timeout', 'throw']) {
    const clock = new Clock();
    const handle = startSimulation(
      {
        ...config,
        mode: 'concurrency',
        maxRounds: 5,
        impressions: true,
        timeoutMs: 100,
      },
      {
        decide: (input, signal) => {
          if (kind === 'throw') throw new Error('offline');
          if (kind === 'timeout') return delay(clock, 900, signal, noAd(input));
          return Promise.resolve({ ...noAd(input), matched: true });
        },
        impression: (_, __, signal) => delay(clock, 50, signal, undefined),
      },
      undefined,
      clock,
    );
    await flush();
    if (kind === 'impression') {
      await clock.advance(49);
      assert.equal(handle.snapshot().started, 2);
      assert.equal(handle.snapshot().inFlight, 2);
    }
    await clock.advance(2000);
    const result = await handle.done;
    assert.equal(result.started, 5);
    assert.equal(result.successful + result.failed, 5);
    assert.equal(result.impressionsAccepted, kind === 'impression' ? 5 : 0);
    assert.equal(result.timeouts, kind === 'timeout' ? 5 : 0);
    assert.ok(result.peakInFlight <= 2);
    assert.equal(clock.tasks.size, 0);
  }
});

test('Only fixed concurrency is accepted, with a bounded workload', () => {
  assert.equal(validateSimulationConfig(config).mode, 'concurrency');
  assert.equal(
    validateSimulationConfig({ ...config, concurrency: 100 }).concurrency,
    100,
  );
  for (const maxRounds of [0, 3001, NaN, 1.5])
    assert.throws(() => validateSimulationConfig({ ...config, maxRounds }));
  for (const mode of ['rate', 'unknown'])
    assert.throws(() => validateSimulationConfig({ ...config, mode }));
});

test('100 concurrent rounds remain bounded and finish at an exact nonmultiple limit', async () => {
  const clock = new Clock();
  const handle = startSimulation(
    { ...config, concurrency: 100, maxRounds: 257, seconds: 10 },
    {
      decide: (input, signal) => delay(clock, 50, signal, noAd(input)),
      impression: async () => {},
    },
    undefined,
    clock,
  );
  assert.equal(handle.snapshot().inFlight, 100);
  await clock.advance(2000);
  const result = await handle.done;
  assert.equal(result.started, 257);
  assert.equal(result.successful, 257);
  assert.equal(result.peakInFlight, 100);
  assert.equal(result.inFlight, 0);
  assert.equal(
    result.started,
    result.successful + result.failed + result.canceled,
  );
  assert.equal(clock.tasks.size, 0);
});

test('Graceful finish drains current rounds without new work and can be interrupted', async () => {
  for (const immediate of [false, true]) {
    const clock = new Clock();
    const handle = startSimulation(
      { ...config, maxRounds: 100, concurrency: 10 },
      {
        decide: (input, signal) => delay(clock, 500, signal, noAd(input)),
        impression: async () => {},
      },
      undefined,
      clock,
    );
    handle.drain();
    assert.equal(handle.snapshot().status, 'stopping');
    assert.equal(handle.snapshot().stopReason, 'manual_drain');
    if (immediate) handle.stop();
    await clock.advance(2000);
    const result = await handle.done;
    assert.equal(result.started, 10);
    assert.equal(result.successful, immediate ? 0 : 10);
    assert.equal(result.canceled, immediate ? 10 : 0);
    assert.equal(result.status, 'stopped');
    assert.equal(clock.tasks.size, 0);
  }
});

test('Seeded pools reproduce independently sampled attributes', () => {
  const pool = makeSimulationProfiles(100, 'adflow-1');
  assert.deepEqual(pool, makeSimulationProfiles(100, 'adflow-1'));
  assert.notDeepEqual(pool, makeSimulationProfiles(100, 'other'));
  for (const device of ['android', 'ios', 'web'])
    for (const tag of ['tech_interest', 'gaming_interest'])
      assert.ok(
        pool.some((p) => p.fields.device === device && p.tags.includes(tag)),
        device + ' / ' + tag,
      );
  assert.throws(() => makeSimulationProfiles(20, ''));
  assert.throws(() => makeSimulationProfiles(20, 'a'.repeat(81)));
});

import { refreshSimulationPool } from '../lib/simulation-pool.ts';
test('Saved pool refresh preserves valid selections without changing temporary data', () => {
  const temporary = {
    profiles: makeSimulationProfiles(2),
    selectedIds: ['user-0001'],
    seed: 'adflow',
  };
  const saved = {
    profiles: makeSimulationProfiles(3),
    selectedIds: ['user-0001', 'user-0003'],
  };
  const original = structuredClone({ temporary, saved });
  const refreshed = refreshSimulationPool(saved, saved.profiles.slice(0, 2));
  assert.deepEqual(refreshed.selectedIds, ['user-0001']);
  assert.deepEqual({ temporary, saved }, original);
});
