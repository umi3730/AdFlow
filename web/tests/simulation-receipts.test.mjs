import test from 'node:test';
import assert from 'node:assert/strict';
import { receiptCounts } from '../lib/simulation-receipts.ts';

test('Receipt confirmation uses exact accepted IDs and durable processing proof', () => {
  const trace = {
    events: [
      { eventId: 'one', status: 'PUBLISHED' },
      {
        eventId: 'two',
        status: 'PROCESSED',
        processedAt: '2026-09-10T01:00:00Z',
      },
      {
        eventId: 'unrelated',
        status: 'PROCESSED',
        processedAt: '2026-09-10T01:00:00Z',
      },
      { eventId: 'three', status: 'DEAD_LETTERED' },
    ],
  };
  assert.deepEqual(receiptCounts(['one', 'two', 'three', 'missing'], trace), {
    processed: 1,
    attention: 1,
    pending: 2,
  });
  assert.deepEqual(receiptCounts(['one'], { ...trace, truncated: true }), {
    processed: 0,
    attention: 0,
    pending: 1,
  });
});

test('Reconciliation is actionable while missing proof never counts as success', () => {
  assert.deepEqual(
    receiptCounts(['one'], { events: [], settlement: { status: 'RECONCILE' } }),
    { processed: 0, attention: 1, pending: 0 },
  );
  assert.deepEqual(
    receiptCounts(['one'], {
      events: [{ eventId: 'one', status: 'PROCESSED' }],
    }),
    { processed: 0, attention: 0, pending: 1 },
  );
});
