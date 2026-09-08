// Opt-in, dedicated local instance with fresh auction demo data.
import assert from 'node:assert/strict';
import { writeFileSync, mkdirSync } from 'node:fs';
assert.equal(process.env.ADFLOW_AUCTION_TEST, '1', 'Use a dedicated test instance and set ADFLOW_AUCTION_TEST=1');
process.env.NEXT_PUBLIC_ADFLOW_API_URL ??= 'http://127.0.0.1:18081';
const base = new URL(process.env.NEXT_PUBLIC_ADFLOW_API_URL);
assert.ok(['localhost','127.0.0.1','[::1]'].includes(base.hostname));
const { api, newClientID } = await import('../web/lib/api.ts');
const { setSession } = await import('../web/lib/auth-session.ts');
setSession(await api.login('admin','adflow-admin'));
let publishedCheck;
try {
  const now=Date.now();
  publishedCheck=await api.createCampaign({name:'竞价发布接口回归',slotId:'auction-api-check',startAt:new Date(now).toISOString(),endAt:new Date(now+86400000).toISOString()});
  publishedCheck=await api.publishCampaign(publishedCheck.id,{targeting:{all:[{tag:'auction_demo'}]},dailyBudgetFen:100,impressionCostFen:99,frequencyLimit:3,auction:{advertiserId:'STUDIO-CHECK',advertiserName:'发布测试公司',bidFen:7}});
  assert.equal(publishedCheck.activeVersion.auction.advertiserId,'studio-check');
  assert.equal(publishedCheck.activeVersion.auction.bidFen,7);
  assert.equal(publishedCheck.activeVersion.impressionCostFen,7);
} finally {
  if(publishedCheck){const current=await api.getCampaign(publishedCheck.id);if(current.status==='ACTIVE')await api.pauseCampaign(current.id);await api.deleteCampaign(current.id);}
}
const high = await api.getCampaign('demo-auction-2');
assert.equal(high.status,'ACTIVE');
assert.equal(high.activeVersion.auction.bidFen,5);
const before=await api.metrics(high.id);
const request={requestId:newClientID('auction-check'),userId:'demo-auction-user',slotId:'game-home-banner'};
const first=await api.decide(request);
assert.equal(first.campaignId,high.id);assert.equal(first.pricing.mode,'first_price');assert.equal(first.pricing.priceFen,5);assert.equal(first.pricing.advertisers,3);
assert.deepEqual(await api.decide(request),first);
const event={eventId:newClientID('auction-impression'),requestId:first.requestId,campaignId:first.campaignId,creativeId:first.creativeId,type:'impression'};
await api.recordEvent(event);await api.recordEvent(event);
const after=await api.metrics(high.id);
assert.equal(after.impressions-before.impressions,1);
let paused=false, fallback;
try {
  await api.pauseCampaign(high.id);paused=true;
  // The configured candidate TTL is five seconds by default.
  await new Promise(resolve=>setTimeout(resolve,5200));
  fallback=await api.decide({...request,requestId:newClientID('auction-fallback')});
  assert.equal(fallback.campaignId,'demo-auction-3');assert.equal(fallback.pricing.priceFen,3);
  assert.deepEqual(await api.decide(request),first,'historical decision was re-auctioned');
} finally { if(paused)await api.resumeCampaign(high.id); }
const report={at:new Date().toISOString(),environment:base.origin,kind:'local-functional-verification',publishAPIValidated:true,bidsFen:[2,5,3],winner:first.campaignId,pricing:first.pricing,retryIdentical:true,duplicateImpressionDelta:after.impressions-before.impressions,afterPausingTopBid:{winner:fallback.campaignId,priceFen:fallback.pricing.priceFen},topBidRestored:true};
mkdirSync('docs/verification/auction',{recursive:true});
writeFileSync('docs/verification/auction/http-smoke.json',JSON.stringify(report,null,2)+'\n');
console.log(JSON.stringify(report));
