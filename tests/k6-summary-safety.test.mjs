import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

test('k6 summary excludes setup credentials and preserves measurement data', () => {
  const source = readFileSync(new URL('./load/k6-walkthrough.js', import.meta.url), 'utf8');
  const summaryFunction = source.slice(source.indexOf('export function handleSummary')).replace('export function', 'function');
  const summarize = vm.runInNewContext(`${summaryFunction}; handleSummary`, { __ENV: {} });
  const data = { setup_data: { token: 'private-test-value' }, metrics: { iterations: { values: { count: 42 } } } };
  const output = summarize(data);
  const saved = JSON.parse(output['k6-summary.json']);
  assert.equal(saved.setup_data, undefined);
  assert.deepEqual(saved.metrics, data.metrics);
  assert.equal(data.setup_data.token, 'private-test-value');
  assert.ok(!JSON.stringify(output).includes('private-test-value'));
});
