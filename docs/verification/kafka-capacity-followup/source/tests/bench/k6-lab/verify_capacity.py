"""Audit Kafka capacity, keeping incomplete timing samples distinguishable."""
import gzip
import hashlib
import json
import math
import pathlib
import sys

directory=pathlib.Path(sys.argv[1])
path=directory/'proof.json'
raw=path.read_bytes() if path.exists() else gzip.decompress((directory/'proof.json.gz').read_bytes())
data=json.loads(raw)
if data.get('error'):
    assert not data.get('confirmedBusinessRoundsPerSecond'), 'an errored run cannot claim confirmed capacity'
assert all(v is True for v in data['cleanup'].values()),data['cleanup']
for name,digest in data['sourceSHA256'].items():
    content=pathlib.Path(name).read_bytes()
    if hashlib.sha256(content).hexdigest()!=digest and name=='tests/bench/k6-lab/kafka_capacity.go':
        content=(directory/'tested-kafka-capacity.go.txt').read_bytes()
    if hashlib.sha256(content).hexdigest()!=digest and name=='tests/bench/k6-lab/main.go':
        content=(directory/'tested-main.go.txt').read_bytes()
    assert hashlib.sha256(content).hexdigest()==digest,name
stages=[]
incomplete=[]
total_loops=total_events=0
for stage in data['capacityResults']:
    if 'passed' not in stage:
        assert data.get('error'), 'incomplete stage requires an explicit run error'
        incomplete.append({'id':stage['id'],'rate':stage['targetBusinessRate'],'seconds':stage['seconds'],'status':'aborted; no complete measurement','runError':data['error']})
        continue
    k6=json.loads((directory/(stage['id']+'-k6.json')).read_text(encoding='utf-8'))
    def v(name,key):
        return k6['metrics'].get(name,{}).get('values',{}).get(key,0)
    for key,metric in [('iterations','iterations'),('matched','matched_decisions'),('completed','completed_rounds'),('acceptedEvents','accepted_events'),('acceptedImpressions','accepted_impressions'),('acceptedClicks','accepted_clicks'),('acceptedConversions','accepted_conversions'),('dropped','dropped_iterations'),('noAd','no_ad')]:
        assert stage[key]==v(metric,'count'),(stage['id'],key)
    samples=stage['completionSamples']
    assert len({s['requestId'] for s in samples})==len(samples)
    for s in samples:
        assert s['committedObservedLatencyMs']==s['committedObservedAtMs']-s['clientStartMs']
        assert s['clientStartMs']==int(s['requestId'].split('_')[-2])
        assert s['committedObservedAtMs']>=s['lastProcessedRecordAtMs']
    lat=sorted(s['committedObservedLatencyMs'] for s in samples)
    assert len(lat)==stage['committedObserved']['count']
    if lat:
        for p,key in [(50,'medianMs'),(95,'p95Ms'),(99,'p99Ms')]:
            assert stage['committedObserved'][key]==lat[math.ceil(len(lat)*p/100)-1]
    coverage=len(lat)==stage['completed']
    proof=stage['proof'];snap=proof['snapshot'];warm=proof['warmupRounds']
    accounting=(snap['decisions']==stage['matched'] and snap['receipts']==snap['published']==snap['processed']==stage['acceptedEvents'] and snap['settled']==stage['acceptedImpressions'] and snap['reconcile']==0 and proof['impressions']==stage['acceptedImpressions']+warm and proof['clicks']==stage['acceptedClicks']+warm and proof['conversions']==stage['acceptedConversions']+warm and proof['conversionValueFen']==(stage['acceptedConversions']+warm)*500 and proof['spentFen']==(stage['acceptedImpressions']+warm)*5 and proof['reservedFen']==0 and proof['duplicatesIdempotent'] and proof['brokerLagZero'])
    assert proof['consistent']==(accounting and coverage and stage['iterations']>0)
    broker=[line.split() for line in proof['brokerGroupDescribe'].splitlines() if len(line.split())>=6 and line.split()[0].startswith('adflow-k6lab-')]
    assert len(broker)==3 and all(row[3]==row[4] and row[5]=='0' for row in broker)
    thresholds=all(t['ok'] for m in k6['metrics'].values() for t in m.get('thresholds',{}).values())
    passed=(thresholds and v('decision_ms','p(95)')<300 and v('accepted_round_ms','p(95)')<1000 and stage['committedObserved']['p95Ms']<5000 and v('business_errors','rate')<.001 and stage['noAd']==0 and stage['dropped']==0 and stage['iterations']>=stage['targetBusinessRate']*stage['seconds']*.99 and stage['drainAfterLoadSeconds']<=10 and proof['consistent'])
    assert stage['passed']==passed
    stages.append({'id':stage['id'],'rate':stage['targetBusinessRate'],'seconds':stage['seconds'],'completed':stage['completed'],'events':stage['acceptedEvents'],'decisionP95Ms':v('decision_ms','p(95)'),'acceptanceP95Ms':v('accepted_round_ms','p(95)'), 'committedObservedP95Ms':stage['committedObserved']['p95Ms'] if coverage else None,'observationCoverage':len(lat)/stage['completed'] if stage['completed'] else 0,'drainSeconds':stage['drainAfterLoadSeconds'],'accountingPassed':accounting,'passed':passed})
    total_loops+=stage['iterations'];total_events+=stage['acceptedEvents']
confirmed=data.get('confirmedBusinessRoundsPerSecond')
last=data['capacityResults'][-3:] if confirmed else []
if confirmed:
    assert data['confirmationsPassed']==3
    assert all(s['passed'] and s['targetBusinessRate']==confirmed and s['seconds']==data['confirmationSeconds'] and s['committedObserved']['count']==s['completed'] for s in last)
lat=sorted(s['committedObservedLatencyMs'] for stage in last for s in stage['completionSamples'])
print(json.dumps({'verified':True,'incompleteStages':incomplete,'confirmedBusinessRoundsPerSecond':confirmed,'measuredLoopsAllStages':total_loops,'acceptedEventsAllStages':total_events,'confirmationLoops':sum(s['iterations'] for s in last),'confirmationEvents':sum(s['acceptedEvents'] for s in last),'confirmedCombinedP95Ms':lat[math.ceil(len(lat)*.95)-1] if confirmed else None,'stages':stages,'rawSHA256':hashlib.sha256(raw).hexdigest()},indent=2))
