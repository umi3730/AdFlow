"""Verify committed end-to-end outcomes and recompute pooled quantiles."""
import collections
import hashlib
import json
import math
import pathlib
import statistics
import sys

def stats(values):
    values = sorted(values)
    return {"n": len(values), "median": statistics.median(values), "p95": values[math.ceil(len(values)*.95)-1], "p99": values[math.ceil(len(values)*.99)-1], "max": max(values)}

path = pathlib.Path(sys.argv[1])
data = json.loads(path.read_text(encoding="utf-8"))
assert not data.get("error"), data.get("error")
for source, digest in data["sourceSHA256"].items():
    assert hashlib.sha256(pathlib.Path(source).read_bytes()).hexdigest() == digest, source
assert data["cleanup"] and all(value is True for value in data["cleanup"].values()), data["cleanup"]
assert len(data["results"]) == data["pairedRounds"] * 4
pooled = collections.defaultdict(list)
totals = collections.Counter()
for run in data["results"]:
    assert not run["failures"], (run["runId"], run["failures"])
    expected = data["requestsPerArm"] + 3
    samples = run["samples"]
    assert len(samples) == expected
    assert len({s["requestId"] for s in samples}) == expected
    for sample in samples:
        assert not sample.get("error"), sample
        assert sample["httpStatuses"] == [200, 202, 202, 202]
        assert sample.get("lastProcessedAt") and sample.get("committedObservedAt")
        assert sample["committedObservedMs"] >= sample["allEventsProcessedMs"]
        assert len(sample["events"]) == 3
        totals["allFlows"] += 1
        if not sample["warmup"]:
            pooled[(run["scenario"], run["indexMode"])].append(sample)
            totals["measuredFlows"] += 1
    proof = run["proof"]
    assert proof["duplicateRequestAndEventsIdempotent"]
    assert proof["kafkaThreePartitionsZeroLag"]
    counts = proof["durableCounts"]
    assert counts["settled"] == expected
    assert counts["receipts"] == counts["published"] == counts["processed"] == expected * 3
    metrics = proof["metrics"]
    assert metrics["impressions"] == metrics["clicks"] == metrics["conversions"] == expected
    assert metrics["valueFen"] == expected * 500
    assert proof["redis"]["spentFen"] == expected * 5
    assert proof["redis"]["reservedFen"] == 0
    assert proof["redis"]["allUsersFrequencyOne"]
    assert proof["runOutboxStatuses"] == [{"count": expected * 3, "status": "PUBLISHED"}]
    measured = [s for s in samples if not s["warmup"]]
    assert len(measured) == data["requestsPerArm"]
    for name in ["decisionMs", "allEventsAcceptedMs", "allEventsProcessedMs", "committedObservedMs"]:
        recomputed = stats([s[name] for s in measured])
        for key, value in recomputed.items():
            assert math.isclose(value, run["summary"][name][key], rel_tol=1e-12)
    totals["settledImpressions"] += counts["settled"]
    totals["processedEvents"] += counts["processed"]
    totals["spentFen"] += proof["redis"]["spentFen"]

summary = []
for (scenario, mode), samples in pooled.items():
    assert len(samples) == data["pairedRounds"] * data["requestsPerArm"]
    row = {"scenario": scenario, "indexMode": mode}
    for name in ["decisionMs", "allEventsAcceptedMs", "allEventsProcessedMs", "committedObservedMs"]:
        row[name] = stats([s[name] for s in samples])
    summary.append(row)
print(json.dumps({"verified": True, "rawSHA256": hashlib.sha256(path.read_bytes()).hexdigest(), "totals": dict(totals), "pooled": summary}, indent=2))
