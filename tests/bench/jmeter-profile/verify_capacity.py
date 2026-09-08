"""Cross-check capacity decisions against k6 summaries, not just exit codes."""
import hashlib
import json
import math
import pathlib
import sys

directory = pathlib.Path(sys.argv[1])
report = json.loads((directory/'results.json').read_text(encoding='utf-8'))
assert not report.get('error'), report.get('error')
assert report['cleanup'] and all(v is True for v in report['cleanup'].values())
for name, digest in report['sourceSHA256'].items():
    content=pathlib.Path(name).read_bytes()
    if hashlib.sha256(content).hexdigest()!=digest and name=='tests/bench/jmeter-profile/main.go':
        content=(directory/'tested-main.go.txt').read_bytes()
    assert hashlib.sha256(content).hexdigest() == digest, name
results = []
total_requests = 0
for stage in report['results']:
    path = directory/(stage['id']+'-k6.json')
    data = json.loads(path.read_text(encoding='utf-8'))
    def value(name, key):
        return data['metrics'].get(name, {}).get('values', {}).get(key, 0)
    count = value('http_reqs','count')
    valid = value('valid_profiles','count')
    p95 = value('http_req_duration','p(95)')
    errors = value('profile_errors','rate')
    dropped = value('dropped_iterations','count')
    thresholds = all(t['ok'] for m in data['metrics'].values() for t in m.get('thresholds', {}).values())
    passed = thresholds and p95 < 100 and errors < .001 and dropped == 0 and count >= stage['offeredQPS']*stage['seconds']*.99 and valid >= count*.999
    assert passed == stage['passed']
    assert stage['requests'] == count and stage['validResponses'] == valid
    assert math.isclose(stage['httpMetrics']['p(95)'], p95)
    assert stage['resources']
    totals = {name: [p[name]['cpuPercentOneCore'] for p in stage['resources'] if name in p] for name in ['api','injector','redis']}
    results.append({'id': stage['id'], 'offeredQPS':stage['offeredQPS'], 'actualHTTPQPS':value('http_reqs','rate'), 'requests':count, 'p95Ms':p95, 'p99Ms':value('http_req_duration','p(99)'), 'errorRate':errors, 'dropped':dropped,'passed':passed,
      'cpuAverageOneCorePercent':{name:sum(v)/len(v) for name,v in totals.items() if v},
      'sampledPeakGoroutines':max(p.get('go_goroutines',0) for p in stage['resources']),
      'sampledPeakHostCPUPercent':max(p.get('hostBusyPercent',0) for p in stage['resources']),
      'tableFetches':stage['tableFetches'], 'k6SHA256':hashlib.sha256(path.read_bytes()).hexdigest()})
    total_requests += count
confirmed = report.get('confirmedOfferedQPS')
if confirmed:
    final = report['results'][-3:]
    assert all(s['passed'] and s['offeredQPS'] == confirmed and s['seconds'] == report['confirmSeconds'] and s['label'].startswith('confirm-') for s in final)
    assert report['confirmationsPassed'] == 3
print(json.dumps({'verified': True,'confirmedOfferedQPS':confirmed,'totalHTTPRequests':total_requests,'rawSHA256':hashlib.sha256((directory/'results.json').read_bytes()).hexdigest(),'stages':results}, indent=2))
