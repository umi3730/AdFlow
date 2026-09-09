import test from 'node:test';
import assert from 'node:assert/strict';
import {
  beijingDate,
  reportDateRange,
  reportPreset,
  formatReportBucket,
  reportQuery,
  deliveryChartData,
  deliveryDetailRows,
} from '../lib/delivery-report.ts';

test('report dates use Beijing boundaries and inclusive end day', () => {
  const filter = reportDateRange(
    '2026-09-09',
    '2026-09-09',
    'hour',
    'c&other=bad',
  );
  assert.equal(filter.from, '2026-09-08T16:00:00.000Z');
  assert.equal(filter.to, '2026-09-09T16:00:00.000Z');
  assert.equal(
    new URLSearchParams(reportQuery(filter)).get('campaignId'),
    'c&other=bad',
  );
  assert.equal(beijingDate(new Date('2026-09-08T17:00:00Z')), '2026-09-09');
  assert.equal(formatReportBucket(filter.from, 'hour'), '09-09 00:00');
  assert.deepEqual(reportPreset(7, new Date('2026-09-08T17:00:00Z')), {
    start: '2026-09-03',
    end: '2026-09-09',
  });
});
test('reject invalid and overly broad date ranges', () => {
  for (const [from, to] of [
    ['2026-02-30', '2026-03-01'],
    ['', ''],
    ['2026-09-10', '2026-09-09'],
    ['2026-08-01', '2026-09-09'],
  ])
    assert.throws(() => reportDateRange(from, to, 'day'));
});

test('chart preserves historical zero hours, excludes future hours and gaps undefined rates', () => {
  const point = {
    impressions: 0,
    clicks: 0,
    conversions: 0,
    spendFen: 0,
    valueFen: 0,
    ctr: 0,
    cvr: 0,
  };
  const report = {
    granularity: 'hour',
    generatedAt: '2026-09-09T03:20:00Z',
    series: [
      { ...point, bucket: '2026-09-09T09:00:00+08:00' },
      {
        ...point,
        bucket: '2026-09-09T10:00:00+08:00',
        impressions: 2,
        clicks: 1,
        ctr: 0.5,
        spendFen: 5,
      },
      { ...point, bucket: '2026-09-09T11:00:00+08:00' },
      { ...point, bucket: '2026-09-09T12:00:00+08:00' },
    ],
  };
  const chart = deliveryChartData(report);
  assert.equal(chart.length, 3);
  assert.equal(chart[0].impressions, 0);
  assert.equal(chart[0].ctr, null);
  assert.equal(chart[0].cvr, null);
  assert.equal(chart[1].ctr, 50);
  assert.equal(chart[1].spend, 0.05);
  assert.equal(
    report.series.length,
    4,
    'report and export data must remain unchanged',
  );
});

test('detail defaults keep all kinds of activity, newest first, without changing report data', () => {
  const empty = {
    impressions: 0,
    clicks: 0,
    conversions: 0,
    spendFen: 0,
    valueFen: 0,
    unpricedImpressions: 0,
  };
  const series = [
    { ...empty, bucket: '2026-09-09T00:00:00+08:00' },
    { ...empty, bucket: '2026-09-09T01:00:00+08:00', clicks: 1 },
    { ...empty, bucket: '2026-09-09T02:00:00+08:00', spendFen: 5 },
    { ...empty, bucket: '2026-09-09T03:00:00+08:00', valueFen: 500 },
    { ...empty, bucket: '2026-09-09T04:00:00+08:00', unpricedImpressions: 1 },
  ];
  const before = structuredClone(series);
  assert.deepEqual(
    deliveryDetailRows(series).map((p) => p.bucket),
    series
      .slice(1)
      .reverse()
      .map((p) => p.bucket),
  );
  assert.equal(deliveryDetailRows(series, true).length, 5);
  assert.deepEqual(deliveryDetailRows([series[0]]), []);
  assert.deepEqual(series, before);
});
