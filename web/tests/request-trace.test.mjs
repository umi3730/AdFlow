import test from 'node:test';
import assert from 'node:assert/strict';
import {
  eventCompletion,
  settlementSummary,
  simulationTraceTarget,
  traceAdvice,
} from '../lib/request-trace.ts';

test('Published event is not presented as counted without its processed record', () => {
  assert.equal(eventCompletion({ status: 'PUBLISHED' }), '尚无计量记录');
  assert.equal(
    eventCompletion({
      status: 'PROCESSING',
      processedAt: '2026-09-07T00:00:00Z',
    }),
    '已计入统计',
  );
});
test('Missing historical settlement proof is not presented as waiting for an exposure', () => {
  assert.match(
    settlementSummary({
      events: [{ type: 'impression', processedAt: '2026-09-07T00:00:00Z' }],
    }),
    /状态未保存/,
  );
});
test('No-ad and expired unexposed decisions have distinct settlement explanations', () => {
  assert.match(
    settlementSummary({ decision: { matched: false }, events: [] }),
    /无需/,
  );
  assert.match(
    settlementSummary({
      decision: { matched: true, expiresAt: '2026-09-07T00:00:00Z' },
      observedAt: '2026-09-07T00:01:00Z',
      events: [],
    }),
    /有效期已过/,
  );
});
test('Temporary simulation failures retain their run and user identity for lookup', () => {
  assert.deepEqual(
    simulationTraceTarget('client-1', 'user-0001', 'run-1', true),
    {
      requestId: 'client-1',
      simulationRunId: 'run-1',
      simulationUserId: 'user-0001',
    },
  );
  assert.deepEqual(
    simulationTraceTarget('tmp:server', 'user-0001', 'run-1', true),
    { requestId: 'tmp:server' },
  );
  assert.deepEqual(
    simulationTraceTarget('saved-request', 'user-0001', 'run-1', false),
    { requestId: 'saved-request' },
  );
});
test('Reconciliation requires investigation instead of automatic recharge or replay', () => {
  assert.match(traceAdvice('RECONCILE', 'kafka'), /不要直接重新扣费/);
  assert.match(traceAdvice('DEAD_LETTERED', 'kafka'), /管理员/);
});
