import http from 'k6/http';
import execution from 'k6/execution';
import { Counter, Rate, Trend } from 'k6/metrics';
const rate=Number(__ENV.RATE), seconds=Number(__ENV.SECONDS);
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(__ENV.BASE_URL||'')||!Number.isInteger(rate)||rate<1||rate>800||!Number.isInteger(seconds)||seconds<5||seconds>120)throw new Error('Invalid isolated workload');
const rejected=new Counter('backpressure_rejections'),matchedTime=new Trend('matched_decision_ms',true);
const errors=new Rate('business_errors'), completed=new Counter('completed_rounds'), matched=new Counter('matched_decisions'), accepted=new Counter('accepted_events'), impressions=new Counter('accepted_impressions'), noAd=new Counter('no_ad');
const clicks=new Counter('accepted_clicks'),conversions=new Counter('accepted_conversions');
const decisionTime=new Trend('decision_ms',true), acceptedTime=new Trend('accepted_round_ms',true);
export const options={summaryTrendStats:['avg','med','p(95)','p(99)','max'],scenarios:{load:{executor:'constant-arrival-rate',rate,timeUnit:'1s',duration:`${seconds}s`,preAllocatedVUs:128,maxVUs:512,gracefulStop:'10s'}},thresholds:{matched_decision_ms:['p(95)<300'],decision_ms:['p(95)<300'],accepted_round_ms:['p(95)<1000'],business_errors:['rate<0.001'],dropped_iterations:['count==0'],completed_rounds:['count>0']}};
export default function(){
 const i=execution.scenario.iterationInTest,start=Date.now(),requestId=`kc-${__ENV.STAGE}_${start}_${i}`;
 const params=(name)=>({headers:{'Content-Type':'application/json',Authorization:`Bearer ${__ENV.ADFLOW_CAPACITY_TOKEN}`},timeout:'3s',tags:{name}});
 const res=http.post(`${__ENV.BASE_URL}/v1/decisions`,JSON.stringify({requestId,userId:`chain-profile-${String(i%1000).padStart(4,'0')}`,slotId:__ENV.SLOT}),params('POST decisions'));
 decisionTime.add(res.timings.duration);
 let d;try{d=res.json()}catch(_){d=null}
 if(res.status===503&&d?.error?.code==='decision_backpressure'&&res.headers['Retry-After']==='1'){rejected.add(1);errors.add(false);return}
 if(res.status!==200||!d?.matched||d.requestId!==requestId||d.campaignId!==__ENV.WINNER||d.pricing?.priceFen!==5||d.pricing?.mode!=='first_price'||d.pricing?.advertisers!==3||!d.creativeId){if(res.status===200&&!d?.matched)noAd.add(1);errors.add(true);return}
 rejected.add(0);matchedTime.add(res.timings.duration);matched.add(1);
 for(const type of ['impression','click','conversion']){
  const e=http.post(`${__ENV.BASE_URL}/v1/events`,JSON.stringify({eventId:`${type}-${requestId}`,requestId,campaignId:d.campaignId,creativeId:d.creativeId,type,...(type==='conversion'?{valueFen:500}:{})}),params(`POST ${type}`));
  if(e.status!==202){errors.add(true);return} accepted.add(1);if(type==='impression')impressions.add(1);if(type==='click')clicks.add(1);if(type==='conversion')conversions.add(1);
 }
 completed.add(1);errors.add(false);acceptedTime.add(Date.now()-start);
}
export function handleSummary(data){const lines=['Intentional overload test: explicit backlog rejections counted separately; NOT capacity acceptance'];for(const n of ['backpressure_rejections','matched_decision_ms','iterations','http_reqs','decision_ms','accepted_round_ms','business_errors','matched_decisions','completed_rounds','accepted_events','accepted_impressions','accepted_clicks','accepted_conversions','no_ad','dropped_iterations'])if(data.metrics[n])lines.push(`${n}: ${JSON.stringify(data.metrics[n])}`);return{[__ENV.REPORT_PATH]:JSON.stringify(data,null,2),stdout:lines.join('\n')+'\n'}}
