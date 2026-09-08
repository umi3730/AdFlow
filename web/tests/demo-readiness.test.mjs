import test from 'node:test';
import assert from 'node:assert/strict';
import { checkDemoReadiness } from '../lib/demo-readiness.ts';
const now = Date.parse('2026-09-07T10:00:00Z');
const campaign = (id) => ({
  id,
  name: id,
  status: 'ACTIVE',
  slotId: 'game-home-banner',
  activeVersion: {},
  startAt: '2026-09-07T00:00:00Z',
  endAt: '2026-09-08T00:00:00Z',
});
const creatives = async () => ({ items: [{ status: 'ACTIVE' }] });
test('Demo preflight rejects missing plans without creating replacement data', async () => {
  await assert.rejects(
    checkDemoReadiness(
      'rules',
      async () => {
        throw { status: 404 };
      },
      creatives,
      now,
    ),
    /已删除/,
  );
});
test('Demo preflight rejects unavailable creatives and expired delivery periods', async () => {
  await assert.rejects(
    checkDemoReadiness(
      'rules',
      async (id) => campaign(id),
      async () => ({ items: [{ status: 'DISABLED' }] }),
      now,
    ),
    /没有启用的素材/,
  );
  await assert.rejects(
    checkDemoReadiness(
      'rules',
      async (id) => ({ ...campaign(id), endAt: '2026-09-07T09:00:00Z' }),
      creatives,
      now,
    ),
    /不在投放期/,
  );
});
test('One unavailable auction participant does not prevent trying eligible plans', async () => {
  const result = await checkDemoReadiness(
    'auction',
    async (id) => ({
      ...campaign(id),
      status: id === 'demo-auction-2' ? 'PAUSED' : 'ACTIVE',
    }),
    creatives,
    now,
  );
  assert.match(result, /demo-auction-2.*暂停/);
});
test('Dependency failure is not misreported as deleted demo data', async () => {
  const cause = new Error('API unavailable');
  await assert.rejects(
    checkDemoReadiness(
      'rules',
      async () => {
        throw cause;
      },
      creatives,
      now,
    ),
    (error) => error === cause,
  );
});
