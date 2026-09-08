import assert from 'node:assert/strict';
import test from 'node:test';
import {
  makePlanSimulationProfiles,
  matchesSampleTargeting,
} from '../lib/plan-simulation-profiles.ts';
import { validateFieldCondition } from '../lib/profile-options.ts';
import { makeSimulationProfiles } from '../lib/user-pool-simulator.ts';

const campaign = (targeting) => ({
  id: 'test-plan',
  name: '测试计划',
  slotId: 'game-home-banner',
  status: 'ACTIVE',
  revision: 2,
  startAt: '2026-01-01T00:00:00Z',
  endAt: '2099-01-01T00:00:00Z',
  activeVersion: {
    number: 3,
    targeting,
    dailyBudgetFen: 10000,
    impressionCostFen: 1,
    frequencyLimit: 100,
    publishedAt: '2026-01-01T00:00:00Z',
  },
});
const field = (name, op, value) => ({ field: name, op, value });

test('Plan samples include exact custom/demo tags and split odd pools reproducibly', () => {
  const plan = campaign({
    all: [{ tag: 'auction_demo' }, { tag: '中文自定义' }],
    none: [{ tag: 'excluded' }],
  });
  const before = structuredClone(plan);
  const result = makePlanSimulationProfiles(9, 'seed-a', plan, 'mixed');
  assert.deepEqual(
    result,
    makePlanSimulationProfiles(9, 'seed-a', plan, 'mixed'),
  );
  assert.deepEqual(plan, before);
  assert.equal(
    Object.values(result.context.expected).filter(Boolean).length,
    5,
  );
  for (const [i, profile] of result.profiles.entries()) {
    const actual =
      profile.tags.includes('auction_demo') &&
      profile.tags.includes('中文自定义') &&
      !profile.tags.includes('excluded');
    assert.equal(actual, i % 2 === 0);
    assert.equal(result.context.expected[profile.userId], actual);
  }
  assert.equal(result.context.version, 3);
  assert.equal(result.context.slotId, plan.slotId);
  result.context.targeting.all[0].tag = 'changed';
  assert.deepEqual(plan, before);
  assert.notDeepEqual(
    result.profiles,
    makePlanSimulationProfiles(9, 'seed-b', plan, 'mixed').profiles,
  );
});

test('Intersect numeric ranges and dictionaries; choose feasible any branch despite excluded alternatives', () => {
  const plan = campaign({
    all: [
      field('age', 'gte', '18'),
      field('age', 'lte', '35'),
      field('device', 'eq', 'android'),
      field('score', 'gte', '70.25'),
    ],
    any: [
      field('device', 'eq', 'ios'),
      { tag: 'forbidden' },
      { tag: 'eligible' },
    ],
    none: [{ tag: 'forbidden' }, field('score', 'gte', '70.26')],
  });
  const result = makePlanSimulationProfiles(100, 'ranges', plan, 'matched');
  for (const profile of result.profiles) {
    assert.ok(
      Number(profile.fields.age) >= 18 && Number(profile.fields.age) <= 35,
    );
    assert.equal(profile.fields.device, 'android');
    assert.ok(
      Number(profile.fields.score) >= 70.25 &&
        Number(profile.fields.score) < 70.26,
    );
    assert.ok(profile.tags.includes('eligible'));
    assert.ok(!profile.tags.includes('forbidden'));
    for (const [name, value] of Object.entries(profile.fields))
      validateFieldCondition(name, 'eq', value);
  }
});

test('Numeric in compares numbers, custom text remains literal and exclusions intersect correctly', () => {
  const plan = campaign({
    all: [
      field('age', 'in', '018, 020, 035'),
      field('age', 'lte', '20'),
      field('custom', 'in', '001, 01'),
    ],
    none: [field('age', 'eq', '18'), field('custom', 'eq', '01')],
  });
  for (const profile of makePlanSimulationProfiles(20, 'in', plan, 'matched')
    .profiles) {
    assert.equal(Number(profile.fields.age), 20);
    assert.equal(profile.fields.custom, '001');
  }
});

test('Counterexamples violate all, any and none, with meaningful valid field values', () => {
  const plans = [
    campaign({ all: [field('age', 'gte', '18'), field('age', 'lte', '35')] }),
    campaign({ any: [{ tag: 'tech_interest' }, { tag: 'gaming_interest' }] }),
    campaign({ none: [field('device', 'eq', 'ios')] }),
  ];
  plans.forEach((plan, index) => {
    for (const profile of makePlanSimulationProfiles(
      30,
      'negative',
      plan,
      'unmatched',
    ).profiles) {
      if (index === 0)
        assert.ok(
          Number(profile.fields.age) < 18 || Number(profile.fields.age) > 35,
        );
      if (index === 1)
        assert.ok(
          !profile.tags.includes('tech_interest') &&
            !profile.tags.includes('gaming_interest'),
        );
      if (index === 2) assert.equal(profile.fields.device, 'ios');
    }
  });
});

test('Unconstrained plans and contradictions report impossible sample classes explicitly', () => {
  assert.throws(
    () => makePlanSimulationProfiles(2, 'seed', campaign({}), 'mixed'),
    /没有可违反/,
  );
  assert.equal(
    makePlanSimulationProfiles(1, 'seed', campaign({}), 'mixed').profiles
      .length,
    1,
  );
  const impossible = [
    { all: [{ tag: 'same' }], none: [{ tag: 'same' }] },
    { all: [field('age', 'gte', '50'), field('age', 'lte', '20')] },
    { any: [{ tag: 'same' }], none: [{ tag: 'same' }] },
  ];
  for (const rule of impossible) {
    assert.throws(
      () => makePlanSimulationProfiles(4, 'seed', campaign(rule), 'matched'),
      /互相矛盾/,
    );
    assert.equal(
      makePlanSimulationProfiles(4, 'seed', campaign(rule), 'unmatched')
        .profiles.length,
      4,
    );
  }
});

test('Missing field can form a valid negative sample when every present numeric value matches', () => {
  const plan = campaign({ all: [field('age', 'gte', '0')] });
  const profile = makePlanSimulationProfiles(1, 'seed', plan, 'unmatched')
    .profiles[0];
  assert.equal(Object.hasOwn(profile.fields, 'age'), false);
  assert.equal(
    matchesSampleTargeting(profile, plan.activeVersion.targeting),
    false,
  );
});

test('Small decimal intervals retain plain decimal strings accepted by the backend', () => {
  const plan = campaign({
    all: [field('score', 'gte', '0.000000001')],
    none: [field('score', 'gte', '0.000000002')],
  });
  for (const profile of makePlanSimulationProfiles(20, 'tiny', plan, 'matched')
    .profiles) {
    assert.ok(
      Number(profile.fields.score) >= 0.000000001 &&
        Number(profile.fields.score) < 0.000000002,
    );
    validateFieldCondition('score', 'eq', profile.fields.score);
  }
});

test('Reject invalid rules, unconfigured plans and out-of-range generation inputs', () => {
  const plan = campaign({ all: [{ tag: 'demo' }] });
  assert.throws(
    () =>
      makePlanSimulationProfiles(
        2,
        'seed',
        { ...plan, activeVersion: undefined },
        'mixed',
      ),
    /尚未发布/,
  );
  for (const count of [0, 101, 1.5])
    assert.throws(() =>
      makePlanSimulationProfiles(count, 'seed', plan, 'mixed'),
    );
  assert.throws(() => makePlanSimulationProfiles(2, '', plan, 'mixed'));
  assert.throws(() => makePlanSimulationProfiles(2, 'seed', plan, 'bad'));
  for (const condition of [
    field('device', 'gte', 'android'),
    field('age', 'eq', '121'),
    field('score', 'eq', '-1'),
    field('age', 'eq', '1.2'),
  ])
    assert.throws(() =>
      makePlanSimulationProfiles(
        2,
        'seed',
        campaign({ all: [condition] }),
        'matched',
      ),
    );
});

test('Unknown field names are own properties and cannot mutate object prototypes', () => {
  const plan = campaign({
    all: [
      field('__proto__', 'eq', 'safe'),
      field('constructor', 'eq', 'literal'),
    ],
  });
  const profile = makePlanSimulationProfiles(1, 'seed', plan, 'matched')
    .profiles[0];
  assert.equal(Object.getPrototypeOf(profile.fields), Object.prototype);
  assert.equal(profile.fields.__proto__, 'safe');
  assert.equal(profile.fields.constructor, 'literal');
});

test('Random generation retains its original seeded behavior without special demo tags', () => {
  const profiles = makeSimulationProfiles(100, 'adflow-1');
  assert.deepEqual(profiles, makeSimulationProfiles(100, 'adflow-1'));
  assert.ok(
    profiles.every(
      (profile) =>
        !profile.tags.includes('auction_demo') &&
        !profile.tags.includes('adflow_demo'),
    ),
  );
});
