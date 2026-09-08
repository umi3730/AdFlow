// Only creates/deletes its own synthetic resources. No model API calls.
import assert from 'node:assert/strict';
const base = 'http://127.0.0.1:18080/v1';
async function call(method, path, body, expected = 200) {
  const response = await fetch(base + path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  assert.equal(response.status, expected, method + ' ' + path);
  return expected === 204 ? undefined : response.json();
}
const preflight = await fetch(base + '/campaigns/example', { method: 'OPTIONS', headers: { Origin: 'http://127.0.0.1:3000', 'Access-Control-Request-Method': 'DELETE' } });
assert.equal(preflight.status, 204);
assert.match(preflight.headers.get('Access-Control-Allow-Methods'), /DELETE/);
const id = 'delete-smoke-' + Date.now();
const campaign = await call('POST', '/campaigns', { name: id, slotId: id, startAt: new Date().toISOString(), endAt: new Date(Date.now() + 86400000).toISOString() }, 201);
const path = '/campaigns/' + campaign.id;
const creative = await call('POST', path + '/creatives', { title: 'Delete smoke creative', imageUrl: 'https://example.com/a.png', landingUrl: 'https://example.com' }, 201);
const cp = path + '/creatives/' + creative.id;
await call('DELETE', cp, undefined, 409);
await call('POST', cp + '/disable');
await call('DELETE', cp, undefined, 204);
assert.equal((await call('GET', path + '/creatives')).items.length, 0);
await call('POST', path + '/publish', { targeting: { all: [{ tag: 'anime' }] }, dailyBudgetFen: 100, impressionCostFen: 1, frequencyLimit: 1 });
await call('DELETE', path, undefined, 409);
await call('POST', path + '/pause');
await call('DELETE', path, undefined, 204);
await call('GET', path, undefined, 404);
await call('PUT', '/profiles/' + id, { tags: ['anime', 'new_user'], fields: { age: '25', device: 'android' } }, 204);
await call('DELETE', '/profiles/' + id, undefined, 204);
await call('DELETE', '/profiles/' + id, undefined, 204);
await call('GET', '/profiles/' + id, undefined, 404);
console.log(JSON.stringify({ passed: true, ownCampaignSoftDeleted: campaign.id, ownCreativeSoftDeleted: creative.id, ownProfileRemoved: id, corsDelete: true }));
