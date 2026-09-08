"""Independently audit raw paired measurements and percentile calculations."""
import collections
import hashlib
import json
import math
import pathlib
import statistics
import sys

path = pathlib.Path(sys.argv[1])
report = json.loads(path.read_text(encoding="utf-8"))
checks = []
for source, digest in report["sourceSHA256"].items():
    assert hashlib.sha256(pathlib.Path(source).read_bytes()).hexdigest() == digest, source
for dataset in report["datasets"]:
    assert dataset["schemaDropped"]
    assert sum(dataset["statusCounts"].values()) == dataset["rows"]
    expected_count = report["rounds"] * report["samplesPerOperationPerRound"]
    assert len(dataset["measurements"]) == expected_count * 4
    for operation in ["select", "update"]:
        pairs = {}
        for mode in ["invisible", "visible"]:
            samples = [m for m in dataset["measurements"] if m["operation"] == operation and m["indexMode"] == mode]
            assert len(samples) == expected_count
            pairs[mode] = collections.Counter((m["round"], m["request"]) for m in samples)
            assert all(m["verifiedRows"] == 3 for m in samples)
            assert all(m["rowsSent"] == 3 if operation == "select" else m["rowsAffected"] == 3 for m in samples)
            summary = next(s for s in dataset["summary"] if s["operation"] == operation and s["indexMode"] == mode)
            for metric in ["clientStatementMs", "serverStatementMs", "rowsExamined"]:
                values = sorted(m[metric] for m in samples)
                actual = {"median": statistics.median(values), "p95": values[math.ceil(len(values) * .95)-1], "min": min(values), "max": max(values)}
                for key in actual:
                    assert math.isclose(actual[key], summary[metric][key], rel_tol=1e-12), (metric, key)
        assert pairs["invisible"] == pairs["visible"], "unpaired request samples"
    for mode in ["invisible", "visible"]:
        plan = dataset["plans"][mode]
        indexes = {row["Key_name"]: row["Visible"] for row in plan["indexes"]}
        assert indexes["idx_outbox_request_status"] == ("YES" if mode == "visible" else "NO")
        assert indexes["idx_event_outbox_claim"] == "YES"
    checks.append({"rows": dataset["rows"], "pairedRequests": True, "rowsVerified": True, "percentilesRecomputed": True, "indexVisibilityVerified": True, "samples": len(dataset["measurements"]), "schemaDropped": True})
print(json.dumps({"sourceHashesMatch": True, "rawSHA256": hashlib.sha256(path.read_bytes()).hexdigest(), "datasets": checks}, indent=2))
