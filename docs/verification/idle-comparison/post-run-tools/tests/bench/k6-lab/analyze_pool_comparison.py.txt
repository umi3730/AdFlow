"""Recompute pool comparisons from raw k6 summaries and cumulative DBStats."""
import gzip
import hashlib
import json
from pathlib import Path
import statistics
import subprocess
import sys
import argparse

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('directory')
parser.add_argument('--measurements-only', action='store_true', help='Preserve source audit failures while auditing raw measurements.')
args=parser.parse_args()
directory=Path(args.directory)
design=json.loads((directory/'design.json').read_text(encoding='utf-8-sig'))
dimension=design.get('dimension','open')
assert dimension in ['open','idle']
expected_order=[10,20,20,10,10,20] if dimension=='idle' else [30,60,100,60,100,30,100,30,60]
assert design.get('finishedAt') and len(design['trials'])==len(expected_order), 'comparison is incomplete'
assert design['order']==expected_order
if dimension=='idle':assert design['fixedPool']['maxOpen']==30
rows=[]
source_audits=[]
for trial in design['trials']:
    folder=directory/trial['name']
    raw=(folder/'proof.json').read_bytes() if (folder/'proof.json').exists() else gzip.decompress((folder/'proof.json.gz').read_bytes())
    report=json.loads(raw)
    assert report['poolMaxOpen']==trial['pool']
    if dimension=='idle':
        assert report['clockObservation']['stable'], 'wall-clock-affected results cannot establish an idle-pool comparison'
        assert trial['pool']==30 and report['poolMaxIdle']==trial['idle']
        assert report['poolFixedSettings']['maxIdle']==trial['idle']
    assert report['poolFixedSettings']['maxLifetime']=='3m' and report['poolFixedSettings']['maxIdleTime']=='1m'
    assert report['apiBinarySHA256']==design['apiSHA256']
    assert all(report['cleanup'].values()), 'temporary resources remain'
    command=[sys.executable,'tests/bench/k6-lab/verify_capacity.py',str(folder)]
    if args.measurements_only:command.append('--measurements-only')
    audited=subprocess.run(command,capture_output=True,text=True,check=True)
    audit=json.loads(audited.stdout)
    assert audit['measurementsVerified'] and (args.measurements_only or audit['verified'])
    source_audits.append({'trial':trial['name'],'sourceVerified':audit['sourceVerified'],'sourceMismatches':audit['sourceMismatches']})
    (folder/'verification.json').write_text(audited.stdout,encoding='utf-8')
    for stage in report['capacityResults']:
        if 'poolObservation' not in stage:
            assert report.get('error'), 'missing observation in a successful run'
            continue
        observation=stage['poolObservation']
        before=observation['before']['instance'];after=observation['after']['instance']
        samples=observation['samples']
        for sample in [observation['before'],*samples]:
            instance=sample['instance']
            assert instance['pid']==before['pid'] and instance['startedAt']==before['startedAt']
            assert instance['apiAddress']==report['environment']['API'].removeprefix('http://')
            assert instance['pool']['MaxOpenConnections']==trial['pool']
        waits=after['pool']['WaitCount']-before['pool']['WaitCount']
        wait_seconds=(after['pool']['WaitDuration']-before['pool']['WaitDuration'])/1e9
        assert waits==observation['waitCountDelta']>=0
        assert abs(wait_seconds-observation['waitDurationSeconds'])<1e-8
        assert observation['sampledPeakInUse']==max(s['instance']['pool']['InUse'] for s in samples)
        k6=json.loads((folder/(stage['id']+'-k6.json')).read_text(encoding='utf-8'))
        def metric(name,key):return k6['metrics'].get(name,{}).get('values',{}).get(key,0)
        row={'trial':trial['name'],'round':trial['round'],'pool':trial['pool'],'idle':trial.get('idle',10),'phase':stage['label'],
             'seconds':stage['seconds'],'attempts':stage['iterations'],'admitted':stage['completed'],
             'rejected':stage['backpressureRejected'],'admittedPerSecond':stage['completed']/stage['seconds'],
             'matchedDecisionP95Ms':metric('matched_decision_ms','p(95)'),
             'acceptanceP95Ms':metric('accepted_round_ms','p(95)'),
             'committedP95Ms':stage['committedObserved']['p95Ms'] if stage['committedObserved']['count']==stage['completed'] else None,
             'waitCount':waits,'waitSeconds':wait_seconds,'sampledPeakInUse':observation['sampledPeakInUse'],
             'maxIdleClosedDelta':after['pool']['MaxIdleClosed']-before['pool']['MaxIdleClosed'],
             'maxLifetimeClosedDelta':after['pool']['MaxLifetimeClosed']-before['pool']['MaxLifetimeClosed'],
             'httpRequests':metric('http_reqs','count'),'unexpectedErrorRate':stage['businessErrors'],
             'drainSeconds':stage['drainAfterLoadSeconds'],'passed':stage['passed']}
        if stage['label']=='overload':
            profile=folder/observation['cpuProfile'];assert profile.stat().st_size>0
            row['cpuProfileSHA256']=hashlib.sha256(profile.read_bytes()).hexdigest()
        rows.append(row)
groups=[]
for phase in ['baseline','overload','recovery']:
    for limit in ([10,20] if dimension=='idle' else [30,60,100]):
        selected=[r for r in rows if r['phase']==phase and r['idle' if dimension=='idle' else 'pool']==limit]
        group={'phase':phase,'pool':30 if dimension=='idle' else limit,'idle':limit if dimension=='idle' else 10,'measuredTrials':len(selected),'allPassed':len(selected)==3 and all(r['passed'] for r in selected)}
        if selected:
            for field in ['admittedPerSecond','matchedDecisionP95Ms','acceptanceP95Ms','committedP95Ms','waitCount','waitSeconds','maxIdleClosedDelta']:
                values=[r[field] for r in selected if r[field] is not None]
                group[field]={'median':statistics.median(values),'min':min(values),'max':max(values)} if values else None
            group['sampledPeakInUse']=max(r['sampledPeakInUse'] for r in selected)
        groups.append(group)
output={'verified':all(a['sourceVerified'] for a in source_audits),'measurementsVerified':True,'sourceAudits':source_audits,'dimension':dimension,'design':'three ordered pairs; fresh resources; max-open fixed at 30; only max-idle changes; 20s CPU profile per overload' if dimension=='idle' else 'three cyclic orders; fresh resources per trial; only max-open changes; 20s CPU profile in every overload phase',
        'rows':rows,'groups':groups,'allTrialsPassed':all(t['passed'] for t in design['trials']),
        'notes':['Rejected attempts are not accepted throughput.','DB wait counts describe connection acquisitions, including background work, not unique HTTP requests.','Peak occupancy is sampled, not an exact maximum.','Three trials on shared local hardware do not establish production capacity or statistical significance.']}
(directory/'analysis.json').write_text(json.dumps(output,indent=2),encoding='utf-8')
print(json.dumps({'allTrialsPassed':output['allTrialsPassed'],'groups':[g for g in groups if g['phase']=='overload']},indent=2))
