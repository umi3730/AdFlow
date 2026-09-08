"""Recompute JMeter quantiles from raw JTL and cross-check source/cache proof."""
import collections
import csv
import hashlib
import gzip
import io
import json
import math
import pathlib
import sys

directory = pathlib.Path(sys.argv[1])
result = json.loads((directory / 'results.json').read_text(encoding='utf-8'))
assert not result.get('error'), result.get('error')
assert all(v is True for v in result['cleanup'].values()), result['cleanup']
assert len(result['results']) == result['pairedRounds'] * 2
for source, sha in result['sourceSHA256'].items():
    content = pathlib.Path(source).read_bytes()
    if hashlib.sha256(content).hexdigest() != sha and source == 'tests/bench/jmeter-profile/main.go':
        content = (directory / 'tested-runner.go.txt').read_bytes()
    assert hashlib.sha256(content).hexdigest() == sha, source
groups = collections.defaultdict(lambda: {'latencies': [], 'seconds': 0, 'allSamples': 0, 'tableFetches': 0, 'sourceDigestCount': 0, 'cacheHits': 0})
files = {}
for arm in result['results']:
    path = directory / (arm['id'] + '.jtl')
    raw = path.read_bytes() if path.exists() else gzip.decompress(path.with_suffix('.jtl.gz').read_bytes())
    files[path.name] = hashlib.sha256(raw).hexdigest()
    with io.StringIO(raw.decode('utf-8'), newline='') as f:
        rows = list(csv.DictReader(f))
    assert rows and all(r['success'] == 'true' and r['responseCode'] == '200' for r in rows)
    measured = [r for r in rows if r['label'] == 'GET profile']
    assert len(rows) - len(measured) == result['threads'] * 20
    latencies = sorted(int(r['elapsed']) for r in measured)
    n = len(measured)
    seconds = (max(int(r['timeStamp'])+int(r['elapsed']) for r in measured) - min(int(r['timeStamp']) for r in measured)) / 1000
    assert n == arm['jmeter']['measuredSamples']
    for percent in [50, 95, 99]:
        assert arm['jmeter'][f'p{percent}Ms'] == latencies[math.ceil(n*percent/100)-1]
    assert math.isclose(arm['jmeter']['requestsPerSecond'], n/seconds)
    assert arm['verificationPassed']
    if arm['mode'] == 'mysql':
        assert arm['tableFetches'] == len(rows)
    else:
        assert arm['tableFetches'] == arm['sourceSelects'] == 0
        assert arm['cacheCounterDeltas']['hit'] == len(rows)
        assert arm['cacheCounterDeltas'].get('miss', 0) == arm['cacheCounterDeltas'].get('fill', 0) == 0
    group = groups[arm['mode']]
    group['latencies'].extend(latencies)
    group['seconds'] += seconds
    group['allSamples'] += len(rows)
    group['tableFetches'] += arm['tableFetches']
    group['sourceDigestCount'] += arm['sourceSelects']
    group['cacheHits'] += arm['cacheCounterDeltas'].get('hit', 0)
pooled = []
for mode, g in groups.items():
    latencies = sorted(g.pop('latencies'))
    n = len(latencies)
    pooled.append({'mode': mode, 'measuredSamples': n, 'p95Ms': latencies[math.ceil(n*.95)-1], 'p99Ms': latencies[math.ceil(n*.99)-1], 'meanMs': sum(latencies)/n, 'requestsPerSecond': n/g['seconds'], **g})
print(json.dumps({'verified': True, 'rawSHA256': hashlib.sha256((directory/'results.json').read_bytes()).hexdigest(), 'jtlSHA256': files, 'pooled': pooled}, indent=2))
