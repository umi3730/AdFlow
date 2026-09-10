import test from 'node:test';
import assert from 'node:assert/strict';
import {
  consoleURL,
  parseConsoleLocation,
  reportLocation,
} from '../lib/console-location.ts';
import { reportDateRange } from '../lib/delivery-report.ts';

test('Console links preserve unrelated parameters and round-trip applied filters', () => {
  const filter = reportDateRange(
    '2026-09-09',
    '2026-09-10',
    'hour',
    '计划 & 1',
  );
  const next = consoleURL('http://localhost/?keep=1#evidence', {
    view: 'reports',
    q: '中文 & 广告',
    ...reportLocation(filter),
  });
  const url = new URL(next, 'http://localhost');
  const parsed = parseConsoleLocation(url.search);
  assert.equal(url.searchParams.get('keep'), '1');
  assert.equal(url.hash, '#evidence');
  assert.equal(parsed.view, 'reports');
  assert.equal(parsed.query, '中文 & 广告');
  assert.deepEqual(parsed.reportFilter, filter);
  const cleared = new URL(
    consoleURL(url.href, { view: 'dashboard', q: '', slot: '__all__' }),
    url,
  );
  assert.equal(cleared.searchParams.has('view'), false);
  assert.equal(cleared.searchParams.has('q'), false);
  assert.equal(cleared.searchParams.has('slot'), false);
  assert.deepEqual(parseConsoleLocation(cleared.search).reportFilter, filter);
});

test('Unknown views and invalid report links fall back without issuing invalid queries', () => {
  assert.equal(parseConsoleLocation('?view=unknown').view, 'dashboard');
  for (const query of [
    'reportStart=2026-02-30&reportEnd=2026-03-02&reportGrain=day',
    'reportStart=2026-09-10&reportEnd=2026-09-09&reportGrain=hour',
    'reportStart=2026-01-01&reportEnd=2026-09-09&reportGrain=day',
    'reportStart=2026-09-09&reportEnd=2026-09-10&reportGrain=unknown',
  ])
    assert.equal(parseConsoleLocation('?' + query).reportFilter, undefined);
});
