import test from 'node:test';
import assert from 'node:assert/strict';
import { groupTargetingCandidates } from '../lib/targeting-report.ts';

const candidate = (campaignId, targetingMatched = true) => ({
  campaignId,
  targetingMatched,
  hasCreative: true,
  failures: [],
});
const report = (candidates) => ({ candidates });

test('The saved winner is separated without reordering or mutating the report', () => {
  const rows = Object.freeze([
    candidate('miss', false),
    candidate('winner'),
    candidate('other'),
  ]);
  const grouped = groupTargetingCandidates(report(rows), {
    matched: true,
    campaignId: 'winner',
  });
  assert.equal(grouped.winner, rows[1]);
  assert.deepEqual(
    grouped.others.map((row) => row.campaignId),
    ['miss', 'other'],
  );
  assert.deepEqual(
    rows.map((row) => row.campaignId),
    ['miss', 'winner', 'other'],
  );
});

test('Current targeting failure does not reclassify a saved winner as a loser', () => {
  const winner = candidate('winner', false);
  const grouped = groupTargetingCandidates(
    report([winner, candidate('other')]),
    { matched: true, campaignId: 'winner' },
  );
  assert.equal(grouped.winner, winner);
  assert.equal(grouped.winner.targetingMatched, false);
});

test('A winner absent from the current candidate list is not replaced by a passing plan', () => {
  const rows = [candidate('other')];
  const grouped = groupTargetingCandidates(report(rows), {
    matched: true,
    campaignId: 'historical',
  });
  assert.equal(grouped.winner, undefined);
  assert.deepEqual(grouped.others, rows);
});

test('An unmatched decision retains every candidate for troubleshooting', () => {
  const rows = [candidate('one', false), candidate('two')];
  const grouped = groupTargetingCandidates(report(rows), {
    matched: false,
    campaignId: 'one',
  });
  assert.equal(grouped.winner, undefined);
  assert.equal(grouped.others, rows);
});
