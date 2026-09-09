"""Ensure a partial provenance audit cannot be mistaken for full verification."""
import gzip
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class CapacityAuditTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.folder = Path(self.temporary.name)
        fixture = ROOT / 'docs/verification/pool-comparison/trial-01-pool-30'
        self.proof = json.loads(gzip.decompress((fixture / 'proof.json.gz').read_bytes()))
        self.proof['sourceSHA256']['cmd/api/main.go'] = '0' * 64
        for file in fixture.glob('*-k6.json'):
            shutil.copy2(file, self.folder / file.name)

    def run_audit(self, *flags):
        (self.folder / 'proof.json').write_text(json.dumps(self.proof), encoding='utf-8')
        return subprocess.run(
            [sys.executable, str(ROOT / 'tests/bench/k6-lab/verify_capacity.py'), str(self.folder), *flags],
            cwd=ROOT, capture_output=True, text=True,
        )

    def test_default_rejects_missing_source(self):
        result = self.run_audit()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('recordedSHA256', result.stderr)

    def test_measurements_only_reports_incomplete_provenance(self):
        result = self.run_audit('--measurements-only')
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertTrue(report['measurementsVerified'])
        self.assertFalse(report['verified'])
        self.assertFalse(report['sourceVerified'])
        self.assertTrue(report['sourceMismatches'])

    def test_measurements_only_still_rejects_tampered_counts(self):
        self.proof['capacityResults'][0]['completed'] += 1
        result = self.run_audit('--measurements-only')
        self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
