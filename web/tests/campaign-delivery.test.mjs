import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
import { runInNewContext } from 'node:vm';
import {
  campaignDisplayStatus,
  formatCampaignDate,
  nextCampaignClockDelay,
} from '../lib/campaign-delivery.ts';
import {
  filterCampaigns,
  allCampaignFilters,
} from '../lib/campaign-filters.ts';

const start = Date.parse('2026-09-07T00:00:00Z');
const end = start + 60_000;
const campaign = {
  id: 'period-test',
  name: '日期测试',
  slotId: 'game-home-banner',
  status: 'ACTIVE',
  startAt: new Date(start).toISOString(),
  endAt: new Date(end).toISOString(),
};

test('Published campaigns need a usable creative and preserve lifecycle and time boundaries', () => {
  const missing = { ...campaign, activeCreativeCount: 0 };
  assert.equal(campaignDisplayStatus(missing, start), 'NEEDS_CREATIVE');
  assert.equal(
    campaignDisplayStatus({ ...missing, activeCreativeCount: 1 }, start),
    'ACTIVE',
  );
  assert.equal(
    campaignDisplayStatus({ ...missing, activeCreativeCount: null }, start),
    'CHECKING',
  );
  assert.equal(campaignDisplayStatus(missing, start - 1), 'SCHEDULED');
  assert.equal(campaignDisplayStatus(missing, end), 'ENDED');
  assert.equal(
    campaignDisplayStatus({ ...missing, status: 'PAUSED' }, start),
    'PAUSED',
  );
  assert.equal(missing.status, 'ACTIVE');
  assert.deepEqual(
    filterCampaigns([missing], '', allCampaignFilters, 'NEEDS_CREATIVE', start),
    [missing],
  );
});

test('Published campaign uses an inclusive start and exclusive end without changing its lifecycle', () => {
  const original = structuredClone(campaign);
  assert.equal(campaignDisplayStatus(campaign, start - 1), 'SCHEDULED');
  assert.equal(campaignDisplayStatus(campaign, start), 'ACTIVE');
  assert.equal(campaignDisplayStatus(campaign, end - 1), 'ACTIVE');
  assert.equal(campaignDisplayStatus(campaign, end), 'ENDED');
  assert.equal(campaignDisplayStatus(campaign, end + 1), 'ENDED');
  assert.deepEqual(campaign, original);
});

test('Draft and paused states stay distinct from a live published campaign', () => {
  assert.equal(
    campaignDisplayStatus({ ...campaign, status: 'DRAFT' }, end + 1),
    'DRAFT',
  );
  assert.equal(
    campaignDisplayStatus({ ...campaign, status: 'PAUSED' }, start - 1),
    'PAUSED',
  );
  assert.equal(
    campaignDisplayStatus({ ...campaign, status: 'PAUSED' }, start),
    'PAUSED',
  );
  assert.equal(
    campaignDisplayStatus({ ...campaign, status: 'PAUSED' }, end),
    'ENDED',
  );
  assert.equal(
    campaignDisplayStatus({ ...campaign, status: 'ENDED' }, start - 1),
    'ENDED',
  );
});

test('Invalid dates and an uninitialized browser clock are never displayed as delivering', () => {
  for (const period of [
    { startAt: '', endAt: campaign.endAt },
    { startAt: 'bad', endAt: campaign.endAt },
    { startAt: campaign.endAt, endAt: campaign.startAt },
    { startAt: campaign.startAt, endAt: campaign.startAt },
  ])
    assert.equal(
      campaignDisplayStatus({ ...campaign, ...period }, start),
      'INVALID_PERIOD',
    );
  assert.equal(campaignDisplayStatus(campaign, null), 'CHECKING');
  assert.equal(campaignDisplayStatus(campaign, NaN), 'CHECKING');
});

test('Status filters and Chinese status search use the same current delivery state', () => {
  const rows = [
    campaign,
    {
      ...campaign,
      id: 'later',
      startAt: new Date(end).toISOString(),
      endAt: new Date(end + 60000).toISOString(),
    },
    {
      ...campaign,
      id: 'old',
      startAt: new Date(start - 120000).toISOString(),
      endAt: campaign.startAt,
    },
  ];
  const original = structuredClone(rows);
  assert.equal(rows.filter((row) => row.status === 'ACTIVE').length, 3);
  assert.equal(
    rows.filter((row) => campaignDisplayStatus(row, start) === 'ACTIVE').length,
    1,
  );
  assert.deepEqual(
    filterCampaigns(rows, '', allCampaignFilters, 'ACTIVE', start).map(
      (c) => c.id,
    ),
    ['period-test'],
  );
  assert.deepEqual(
    filterCampaigns(rows, '', allCampaignFilters, 'SCHEDULED', start).map(
      (c) => c.id,
    ),
    ['later'],
  );
  assert.deepEqual(
    filterCampaigns(rows, '', allCampaignFilters, 'ENDED', start).map(
      (c) => c.id,
    ),
    ['old'],
  );
  assert.deepEqual(
    filterCampaigns(
      rows,
      '已结束',
      allCampaignFilters,
      allCampaignFilters,
      start,
    ).map((c) => c.id),
    ['old'],
  );
  assert.deepEqual(
    filterCampaigns(rows, '', allCampaignFilters, 'ACTIVE', end).map(
      (c) => c.id,
    ),
    ['later'],
  );
  assert.deepEqual(rows, original);
});

test('Clock wakes at the next boundary and remains bounded when no boundary is near', () => {
  assert.equal(nextCampaignClockDelay([campaign], start - 200), 200);
  assert.equal(nextCampaignClockDelay([campaign], end - 1), 1);
  assert.equal(nextCampaignClockDelay([campaign], end), 60000);
  assert.equal(nextCampaignClockDelay([], start), 60000);
  assert.equal(
    nextCampaignClockDelay([{ ...campaign, status: 'DRAFT' }], end - 1),
    60000,
  );
  assert.equal(
    nextCampaignClockDelay([{ ...campaign, status: 'PAUSED' }], end - 10),
    10,
  );
});

test('Period display uses explicit Beijing time including midnight', () => {
  assert.equal(formatCampaignDate('2026-09-07T16:00:00Z'), '2026-09-08 00:00');
  assert.equal(
    formatCampaignDate('2026-09-08T00:00:00+08:00'),
    '2026-09-08 00:00',
  );
  assert.equal(formatCampaignDate('bad'), '时间无效');
});

test('Shared campaign clock advances automatically, refreshes on return, and cleans up', () => {
  let clock = start - 20;
  const updates = [],
    effects = [],
    timers = new Map(),
    focusHandlers = new Map(),
    visibilityHandlers = new Map();
  let sequence = 0;
  const source = readFileSync(
    new URL('../hooks/use-campaign-clock.ts', import.meta.url),
    'utf8',
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
  const exports = {};
  const document = {
    hidden: false,
    addEventListener: (name, fn) => visibilityHandlers.set(name, fn),
    removeEventListener: (name) => visibilityHandlers.delete(name),
  };
  runInNewContext(compiled, {
    require: (name) => {
      if (name === 'react')
        return {
          useState: (value) => [value, (next) => updates.push(next)],
          useEffect: (effect) => effects.push(effect),
        };
      if (name === '@/lib/campaign-delivery') return { nextCampaignClockDelay };
      throw new Error('unexpected import ' + name);
    },
    exports,
    window: {
      addEventListener: (name, fn) => focusHandlers.set(name, fn),
      removeEventListener: (name) => focusHandlers.delete(name),
    },
    document,
    setTimeout: (callback, delay) => {
      const id = ++sequence;
      timers.set(id, { callback, delay });
      return id;
    },
    clearTimeout: (id) => timers.delete(id),
    Date: { now: () => clock },
  });
  assert.equal(exports.useCampaignClock([campaign]), null);
  const cleanup = effects[0]();
  assert.equal(campaignDisplayStatus(campaign, updates.at(-1)), 'SCHEDULED');
  assert.equal([...timers.values()][0].delay, 20);
  clock = start;
  [...timers.values()][0].callback();
  assert.equal(campaignDisplayStatus(campaign, updates.at(-1)), 'ACTIVE');
  assert.equal(timers.size, 1);
  clock = end;
  visibilityHandlers.get('visibilitychange')();
  assert.equal(campaignDisplayStatus(campaign, updates.at(-1)), 'ENDED');
  focusHandlers.get('focus')();
  assert.equal(timers.size, 1);
  cleanup();
  assert.equal(timers.size, 0);
  assert.equal(focusHandlers.size + visibilityHandlers.size, 0);
});
