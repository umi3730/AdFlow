// Local synthetic regression. Does not mutate existing user resources or call a model.
import assert from 'node:assert/strict';
import { demoCreatives, localCreativeImageURL } from '../web/lib/demo-creatives.ts';
const key = 'availability-' + Date.now();
const base = 'http://127.0.0.1:18080/v1';
async function call(method, path, body, expected = 200) {
  const response = await fetch(base + path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  assert.equal(response.status, expected, method + ' ' + path);
  return expected === 204 ? undefined : response.json();
}
const campaign = await call('POST', '/campaigns', { name: key, slotId: key, startAt: new Date().toISOString(), endAt: new Date(Date.now() + 86400000).toISOString() }, 201);
const path = '/campaigns/' + campaign.id;
const creative = await call('POST', path + '/creatives', { title: key, imageUrl: localCreativeImageURL(demoCreatives[0].id, 'http://127.0.0.1:3000'), landingUrl: 'https://example.com' }, 201);
const cp = path + '/creatives/' + creative.id;
try {
  await call('POST', cp + '/enable', undefined, 422);
  for (let i = 0; i < 2; i++) {
    assert.equal((await call('POST', cp + '/disable')).status, 'DISABLED');
    assert.equal((await call('POST', cp + '/enable')).status, 'ACTIVE');
  }
  await call('PUT', '/profiles/' + key, { tags: ['gaming_interest'], fields: { device: 'android' } }, 204);
  await call('POST', path + '/publish', { targeting: { all: [{ tag: 'gaming_interest' }] }, dailyBudgetFen: 100, impressionCostFen: 1, frequencyLimit: 3 });
  assert.equal((await call('POST', '/decisions', { requestId: key, userId: key, slotId: key })).matched, true);
  await call('POST', path + '/pause');
  await call('POST', cp + '/disable');
  await call('POST', cp + '/enable');
  assert.equal((await call('GET', path)).status, 'PAUSED');
  await call('POST', cp + '/disable');
  await call('DELETE', cp, undefined, 204);
  await call('POST', cp + '/enable', undefined, 404);
  console.log('PASS: repeated disable/enable, restored decision eligibility, parent stays paused, deleted creative cannot return.');
} finally {
  const current = await call('GET', path);
  if (current.status === 'ACTIVE') await call('POST', path + '/pause');
  await call('DELETE', path, undefined, 204);
  await call('DELETE', '/profiles/' + key, undefined, 204);
}
