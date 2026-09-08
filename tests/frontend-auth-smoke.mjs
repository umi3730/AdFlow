// Local demo credentials are seeded only in local/test. Never log access tokens.
import assert from 'node:assert/strict';
const base = 'http://127.0.0.1:18080';
const json = async (path, token, method = 'GET', body) => fetch(base + path, {
  method,
  headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) },
  body: body === undefined ? undefined : JSON.stringify(body),
});
assert.equal((await json('/v1/campaigns')).status, 401);
assert.equal((await json('/v1/auth/login', '', 'POST', { username: 'admin', password: 'incorrect-test-password' })).status, 401);
const tokens = {};
for (const role of ['viewer', 'operator', 'admin']) {
  const response = await json('/v1/auth/login', '', 'POST', { username: role, password: 'adflow-' + role });
  assert.equal(response.status, 200);
  tokens[role] = (await response.json()).accessToken;
  const me = await (await json('/v1/auth/me', tokens[role])).json();
  assert.equal(me.authEnabled, true);
  assert.equal(me.principal.role, role);
  assert.equal((await json('/v1/campaigns', tokens[role])).status, 200);
}
const runtime = await (await json('/v1/operations/mode', tokens.admin)).json();
assert.deepEqual(runtime, { eventTransport: 'sync', outboxEnabled: false });
const name = 'auth-smoke-' + Date.now();
const input = { name, slotId: 'game-home-banner', startAt: new Date().toISOString(), endAt: new Date(Date.now() + 86400000).toISOString() };
assert.equal((await json('/v1/campaigns', tokens.viewer, 'POST', input)).status, 403);
let campaign;
try {
  const create = await json('/v1/campaigns', tokens.operator, 'POST', input);
  assert.equal(create.status, 201);
  campaign = await create.json();
  const rules = { targeting: { any: [{ tag: 'gaming_interest' }] }, dailyBudgetFen: 10000, impressionCostFen: 1, frequencyLimit: 3 };
  assert.equal((await json('/v1/campaigns/' + campaign.id + '/publish', tokens.operator, 'POST', rules)).status, 403);
  assert.equal((await json('/v1/campaigns/' + campaign.id + '/publish', tokens.admin, 'POST', rules)).status, 200);
  for (const role of ['viewer', 'operator']) {
    assert.equal((await json('/v1/campaigns/' + campaign.id, tokens[role], 'DELETE')).status, 403);
    assert.equal((await json('/v1/simulations/decisions', tokens[role], 'POST', {})).status, 403);
    assert.equal((await json('/v1/operations/dead-letters/auth-smoke/replay', tokens[role], 'POST')).status, 403);
  }
  console.log(JSON.stringify({ authentication: true, roles: ['viewer', 'operator', 'admin'], missingToken: 401, insufficientRole: 403, runtime, result: 'passed' }));
} finally {
  if (campaign) {
    const current = await (await json('/v1/campaigns/' + campaign.id, tokens.admin)).json();
    if (current.status === 'ACTIVE') assert.equal((await json('/v1/campaigns/' + campaign.id + '/pause', tokens.admin, 'POST')).status, 200);
    assert.equal((await json('/v1/campaigns/' + campaign.id, tokens.admin, 'DELETE')).status, 204);
  }
}
