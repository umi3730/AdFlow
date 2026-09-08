"""Describe within-load backlog trends, separately from finite-window SLO."""
import datetime
import gzip
import json
import pathlib
import statistics
import sys

directory=pathlib.Path(sys.argv[1]);path=directory/'proof.json'
raw=path.read_bytes() if path.exists() else gzip.decompress((directory/'proof.json.gz').read_bytes())
report=json.loads(raw)
out=[]
excluded=[]
for stage in report['capacityResults']:
    if not stage['label'].startswith('confirm-'):
        continue
    if not stage.get('completionSamples') or len(stage['completionSamples'])!=stage.get('completed'):
        excluded.append({'id':stage['id'],'reason':'incomplete completion coverage; dispatch window cannot be inferred reliably'})
        continue
    start=min(s['clientStartMs'] for s in stage['completionSamples'])
    stop=max(s['clientStartMs'] for s in stage['completionSamples'])
    samples=[p for p in stage['queueTimeline'] if start+5000<=datetime.datetime.fromisoformat(p['at']).timestamp()*1000<=stop]
    if len(samples)<10:
        continue
    backlog=[p['receipts']-p['processed'] for p in samples]
    first=statistics.mean(backlog[:5]);last=statistics.mean(backlog[-5:])
    out.append({'id':stage['id'],'targetBusinessRate':stage['targetBusinessRate'],'seconds':stage['seconds'],'firstFiveMeanPendingEvents':first,'lastFiveMeanPendingEvents':last,'growthEvents':last-first,'peakPendingDuringDispatch':max(backlog),'growthExceedsOneRelayBatch':last-first>100})
print(json.dumps({'method':'Exclude first 5 seconds; compare means of first/last 5 sampled backlog counts during request dispatch. >100 events growth is a one-relay-batch warning, not proof of infinite-horizon stability.','excludedStages':excluded,'stages':out},indent=2))
