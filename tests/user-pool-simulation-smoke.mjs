// Low-volume local integration test using only its own synthetic data.
import assert from 'node:assert/strict';
import { setSession } from '../web/lib/auth-session.ts';
import { api, newClientID, simulationAPIBase } from '../web/lib/api.ts';
import { demoCreatives, localCreativeImageURL } from '../web/lib/demo-creatives.ts';
import { makeSimulationProfiles, startSimulation, localSimulationTarget } from '../web/lib/user-pool-simulator.ts';

assert.ok(localSimulationTarget(simulationAPIBase), 'integration test is local-only');
try { await api.authMe(); } catch (error) {
 if (error.status !== 401) throw error;
 setSession(await api.login(process.env.ADFLOW_TEST_USERNAME ?? 'admin', process.env.ADFLOW_TEST_PASSWORD ?? 'adflow-admin'));
}
const batch = newClientID('sim-smoke');
const profiles = makeSimulationProfiles(4);
const before = await api.listProfiles({limit:100});
let campaign;
try {
  campaign = await api.createCampaign({ name: batch, slotId: batch, startAt: new Date().toISOString(), endAt: new Date(Date.now() + 86400000).toISOString() });
  await api.createCreative(campaign.id, { title: batch, description: 'simulation regression', imageUrl: localCreativeImageURL(demoCreatives[0].id, 'http://127.0.0.1:3000'), landingUrl: 'https://example.com' });
  await api.publishCampaign(campaign.id, { targeting: { any: [{ tag: 'gaming_interest' }, { tag: 'tech_interest' }] }, dailyBudgetFen: 10000, impressionCostFen: 1, frequencyLimit: 100 });
  const handle = startSimulation({ runId: batch, userIds: profiles.map(profile => profile.userId), slotId: batch, mode: 'concurrency', maxRounds: 10, concurrency: 3, seconds: 2, timeoutMs: 2000, impressions: true, behavior: { clickPercent: 100, conversionPercent: 100, minValueFen: 990, maxValueFen: 19900 } }, {
    decide: (input, signal) => api.simulateDecision({runId:batch,requestId:input.requestId,slotId:input.slotId,profile:profiles.find(profile=>profile.userId===input.userId)}, signal),
    impression: (decision, eventId, signal) => api.recordEvent({ eventId, requestId: decision.requestId, campaignId: decision.campaignId, creativeId: decision.creativeId, type: 'impression' }, signal),
    click: (decision, eventId, signal) => api.recordEvent({ eventId, requestId: decision.requestId, campaignId: decision.campaignId, creativeId: decision.creativeId, type: 'click' }, signal),
    conversion: (decision, eventId, valueFen, signal) => api.recordEvent({ eventId, requestId: decision.requestId, campaignId: decision.campaignId, creativeId: decision.creativeId, type: 'conversion', valueFen }, signal),
  });
  const result = await handle.done;
  assert.equal(result.status, 'completed');
  assert.ok(result.started >= 2 && result.started <= 10);
  assert.equal(result.started, 10);
  assert.equal(result.failed, 0); assert.equal(result.canceled, 0);
  assert.equal(result.matched, result.started); assert.equal(result.impressionsAccepted, result.matched);
  assert.ok(result.peakInFlight <= 3);
  assert.equal(result.clicksAccepted, result.started);
  assert.equal(result.conversionsAccepted, result.started);
  assert.equal(result.httpRequests, result.started * 4);
  assert.ok(result.valueFenAccepted >= 990 * result.started && result.valueFenAccepted <= 19900 * result.started);
  // In Kafka mode HTTP acceptance precedes consumer-side aggregation.
  let metric = await api.metrics(campaign.id);
  const metricDeadline = Date.now() + 15000;
  while (metric.conversions !== result.conversionsAccepted && Date.now() < metricDeadline) {
    await new Promise((resolve) => setTimeout(resolve, 250));
    metric = await api.metrics(campaign.id);
  }
  assert.equal(metric.conversions, result.conversionsAccepted);
  assert.equal(metric.valueFen, result.valueFenAccepted);
  const after = await api.listProfiles({limit:100});
  assert.deepEqual(after,before,'temporary profiles changed the saved catalog');
  console.log(JSON.stringify({status:result.status,started:result.started,matched:result.matched,failed:result.failed,peakInFlight:result.peakInFlight,httpRequests:result.httpRequests,impressions:result.impressionsAccepted,clicks:result.clicksAccepted,conversions:result.conversionsAccepted,catalogUnchanged:true}));
} finally {
  if (campaign) {
    const current = await api.getCampaign(campaign.id);
    if (current.status === 'ACTIVE') await api.pauseCampaign(campaign.id);
    await api.deleteCampaign(campaign.id);
  }
}
