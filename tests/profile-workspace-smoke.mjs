// Opt-in local HTTP smoke test. Creates isolated synthetic records; no Agent calls.
// Run: node tests/profile-workspace-smoke.mjs
import assert from 'node:assert/strict';

const base = 'http://127.0.0.1:18080/v1';
const prefix = `profile-smoke-${Date.now()}`;
const userId = `${prefix}-user`;
const slotId = `${prefix}-slot`;
async function request(path, method = 'GET', body) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  assert.ok(response.ok, `${method} ${path}: ${response.status} ${await (!response.ok ? response.text() : Promise.resolve(''))}`);
  return response.status === 204 ? undefined : response.json();
}

await request(`/profiles/${userId}`, 'PUT', {
  tags: ['anime'], fields: { device: 'android', score: '88', region: 'shanghai' },
});
const saved = await request(`/profiles/${userId}`);
assert.equal(saved.fields.region, 'shanghai');
const page = await request(`/profiles?q=${prefix}&tag=anime&device=android&limit=1`);
assert.equal(page.total, 1);
assert.equal(page.items[0].userId, userId);
assert.equal((await request(`/profiles?q=${prefix}&offset=1`)).items.length, 0);

const campaign = await request('/campaigns', 'POST', {
  name: `Profile smoke ${prefix}`, slotId,
  startAt: new Date(Date.now() - 60_000).toISOString(),
  endAt: new Date(Date.now() + 86_400_000).toISOString(),
});
await request(`/campaigns/${campaign.id}/creatives`, 'POST', {
  title: 'Smoke creative', imageUrl: 'https://example.com/smoke.png', landingUrl: 'https://example.com',
});
await request(`/campaigns/${campaign.id}/publish`, 'POST', {
  targeting: { all: [{ tag: 'strategy_game' }, { field: 'platform', op: 'eq', value: 'android' }], any: [], none: [] },
  dailyBudgetFen: 1, impressionCostFen: 1, frequencyLimit: 1,
});
const diagnosticBody = { userId, slotId };
const failed = await request('/decisions', 'POST', { ...diagnosticBody, requestId: `${prefix}-miss` });
assert.equal(failed.reason, 'targeting_miss');
assert.equal(failed.expiresAt, null);
const report = await request('/decisions/explain', 'POST', diagnosticBody);
assert.equal(report.scope, 'current_targeting_only');
assert.deepEqual(report.candidates[0].failures.map(f => f.code).sort(), ['missing_field', 'missing_tag']);

await request(`/profiles/${userId}`, 'PUT', {
  tags: [...saved.tags, 'strategy_game'], fields: { ...saved.fields, platform: 'android' },
});
for (let i = 0; i < 3; i++) {
  const matchedReport = await request('/decisions/explain', 'POST', diagnosticBody);
  assert.equal(matchedReport.candidates[0].targetingMatched, true);
  assert.equal(matchedReport.candidates[0].failures.length, 0);
}
const matched = await request('/decisions', 'POST', { ...diagnosticBody, requestId: `${prefix}-match` });
assert.equal(matched.matched, true, 'Diagnostics must not consume the single budget/frequency allowance');
assert.equal((await request(`/profiles/${userId}`)).fields.region, 'shanghai');
await request(`/campaigns/${campaign.id}/pause`, 'POST');
console.log(JSON.stringify({ passed: true, userId, campaignId: campaign.id, campaignPaused: true,
  checks: ['profile write/read/filter/pagination', 'missing field/tag', 'null expiry', 'extra-field preservation', 'diagnostics without reservation'] }));
