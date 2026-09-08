// Local-only synthetic API regression; no model API calls.
import assert from 'node:assert/strict';
const base = 'http://127.0.0.1:18080/v1';
const key = 'audience-check-' + Date.now();
async function call(method, path, body, expected = 200) {
  const response = await fetch(base + path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  assert.equal(response.status, expected, method + ' ' + path);
  return expected === 204 ? undefined : response.json();
}
for (const fields of [{ age: '18.5' }, { age: '121' }, { member_level: 'vip100' }, { channel: 'unknown' }]) await call('PUT', '/profiles/' + key, { fields }, 422);
await call('PUT', '/profiles/' + key, { tags: ['tech_interest'], fields: { device: 'android', age: '25', score: '88', member_level: 'gold', channel: 'referral' } }, 204);
const campaign = await call('POST', '/campaigns', { name: key, slotId: key, startAt: new Date().toISOString(), endAt: new Date(Date.now() + 86400000).toISOString() }, 201);
const path = '/campaigns/' + campaign.id;
const costs = { dailyBudgetFen: 100, impressionCostFen: 1, frequencyLimit: 3 };
try {
  await call('POST', path + '/publish', { ...costs, targeting: { all: [{ field: 'member_level', op: 'gte', value: 'gold' }] } }, 422);
  await call('POST', path + '/creatives', { title: 'Audience test', imageUrl: 'https://example.com/a.png', landingUrl: 'https://example.com' }, 201);
  await call('POST', path + '/publish', { ...costs, targeting: { all: [{ tag: 'tech_interest' }, { field: 'age', op: 'gte', value: '18' }, { field: 'age', op: 'lte', value: '35' }, { field: 'member_level', op: 'in', value: 'silver,gold' }, { field: 'channel', op: 'eq', value: 'referral' }] } });
  const result = await call('POST', '/decisions', { requestId: key, userId: key, slotId: key });
  assert.equal(result.matched, true);
  console.log('PASS: age bounds, dictionary validation, enum ordering rejection, and real decision match.');
} finally {
  const current = await call('GET', path);
  if (current.status === 'ACTIVE') await call('POST', path + '/pause');
  await call('DELETE', path, undefined, 204);
  await call('DELETE', '/profiles/' + key, undefined, 204);
}
