// Opt-in local check; leaves one clearly named, unpublished draft for inspection.
import assert from 'node:assert/strict';
import { newAgentCampaignInput } from '../web/lib/agent-campaign.ts';

const base = 'http://127.0.0.1:18080/v1';
const input = newAgentCampaignInput(`Agent 行内创建验收 ${Date.now()}`, 'game-home-banner');
const response = await fetch(`${base}/campaigns`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
});
assert.equal(response.status, 201);
const created = await response.json();
assert.equal(created.status, 'DRAFT');
assert.equal(created.activeVersion, undefined, 'Creating a plan must not publish generated rules');
const readResponse = await fetch(`${base}/campaigns/${encodeURIComponent(created.id)}`);
assert.equal(readResponse.status, 200);
const stored = await readResponse.json();
assert.equal(stored.id, created.id);
assert.equal(stored.status, 'DRAFT');
assert.equal(stored.name, input.name);
assert.equal(stored.slotId, input.slotId);
assert.equal(Date.parse(stored.endAt) - Date.parse(stored.startAt), 7 * 86400000);
console.log(JSON.stringify({ passed: true, campaignId: stored.id, name: stored.name, status: stored.status, published: false }));
