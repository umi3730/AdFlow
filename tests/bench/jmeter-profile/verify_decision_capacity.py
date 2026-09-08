"""Verify decision load classification and persisted reservation evidence."""
import hashlib
import json
import math
import pathlib
import sys

directory=pathlib.Path(sys.argv[1])
data=json.loads((directory/'results.json').read_text(encoding='utf-8'))
assert not data.get('error'),data.get('error')
assert data['cleanup'] and all(v is True for v in data['cleanup'].values())
for name,digest in data['sourceSHA256'].items():
    assert hashlib.sha256(pathlib.Path(name).read_bytes()).hexdigest()==digest,name
stages=[]
totals={'requests':0,'matched':0,'noAd':0,'httpErrors':0,'unexpectedAuction':0,'dropped':0}
for stage in data['results']:
    path=directory/(stage['id']+'-k6.json')
    k6=json.loads(path.read_text(encoding='utf-8'))
    def metric(name,key):
        return k6['metrics'].get(name,{}).get('values',{}).get(key,0)
    for name,key in [('requests','http_reqs'),('matched','matched_decisions'),('noAd','no_ad'),('httpErrors','decision_http_errors'),('unexpectedAuction','unexpected_auction'),('dropped','dropped_iterations')]:
        assert stage[name]==metric(key,'count')
        totals[name]+=stage[name]
    proof=stage['proof']
    assert proof['reservedFen']==proof['reservationEntries']*5, 'all held reservations must use the winning 5-fen price'
    consistent=(proof['storedMatchedIncludingWarmup']>=stage['matched']+3 and proof['wrongStoredWinnerOrPrice']==0 and proof['wrongStoredUser']==0 and proof['spentFen']==0 and proof['reservedFen']==proof['reservationAmountSumFen'] and proof['reservedFen']<=data['budgetFen'] and proof['maxUserFrequency']<=100 and proof['eventReceipts']==0)
    assert proof['consistent']==consistent
    thresholds=all(t['ok'] for m in k6['metrics'].values() for t in m.get('thresholds',{}).values())
    p95=metric('http_req_duration','p(95)')
    error_rate=metric('decision_errors','rate')
    assert math.isclose(stage['httpMetrics']['p(95)'],p95)
    passed=thresholds and p95<300 and error_rate<.001 and stage['dropped']==0 and stage['requests']>=stage['offeredQPS']*stage['seconds']*.99 and stage['matched']>=stage['requests']*.999 and stage['noAd']==0 and stage['unexpectedAuction']==0 and consistent
    assert passed==stage['passed']
    points=stage['resources']
    stages.append({'id':stage['id'],'offeredQPS':stage['offeredQPS'],'seconds':stage['seconds'],'actualQPS':stage['actualQPS'],'p95Ms':p95,'p99Ms':metric('http_req_duration','p(99)'), 'matched':stage['matched'],'requests':stage['requests'],'errors':error_rate,'noAd':stage['noAd'],'dropped':stage['dropped'],'passed':passed,
      'sampledPeakInFlight':max((p.get('adflow_decision_in_flight',0) for p in points),default=0),
      'sampledPeakGoroutines':max((p.get('go_goroutines',0) for p in points),default=0),
      'sampledPeakHostCPUPercent':max((p.get('hostBusyPercent',0) for p in points),default=0),
      'maxUserFrequency':proof['maxUserFrequency'],'reservedFen':proof['reservedFen'],
      'rawSHA256':hashlib.sha256(path.read_bytes()).hexdigest()})
confirmed=data.get('confirmedOfferedQPS')
if confirmed:
    assert data['confirmationsPassed']==3
    assert all(s['passed'] and s['offeredQPS']==confirmed and s['seconds']==data['confirmSeconds'] and s['label'].startswith('confirm-') for s in data['results'][-3:])
print(json.dumps({'verified':True,'confirmedOfferedQPS':confirmed,'totals':totals,'stages':stages,'rawSHA256':hashlib.sha256((directory/'results.json').read_bytes()).hexdigest()},indent=2))
