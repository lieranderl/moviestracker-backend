#!/usr/bin/env python3
"""Exercise the release workflow's actual rollback commands without cloud access."""

import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[1]
WORKLOADS = (
    "get-imdb-rating-service",
    "getmovies-latesttorrentsmovies-job",
    "getmovies-hdr10movies-job",
    "getmovies-dvmovies-job",
    "downloadimdbdata-job",
)
SNAPSHOTS = (
    "getimdbrating",
    "getmovies-latesttorrentsmovies",
    "getmovies-hdr10movies",
    "getmovies-dvmovies",
    "downloadimdbdata",
)


def rollback_script():
    workflow = (ROOT / ".github/workflows/release.yml").read_text()
    step = workflow.split("      - name: Roll back workload images on failure\n", 1)[1]
    body = step.split("        run: |\n", 1)[1]
    lines = []
    for line in body.splitlines():
        if line and not line.startswith("          "):
            break
        lines.append(line)
    return textwrap.dedent("\n".join(lines))


class ReleaseRollbackTest(unittest.TestCase):
    def run_rollback(self, failures=(), snapshots=SNAPSHOTS):
        with tempfile.TemporaryDirectory() as directory:
            workdir = Path(directory)
            (workdir / "rollback").mkdir()
            for snapshot in snapshots:
                (workdir / "rollback" / snapshot).write_text("test/image@sha256:1234\n")
            fake_gcloud = workdir / "gcloud"
            fake_gcloud.write_text(
                '#!/bin/sh\n'
                'printf "%s\\n" "$4" >> "$GCLOUD_CALLS"\n'
                'case ",$FAIL_WORKLOADS," in\n'
                '  *,"$4",*) exit 1 ;;\n'
                'esac\n'
            )
            fake_gcloud.chmod(0o755)
            calls_path = workdir / "calls"
            environment = dict(
                os.environ,
                PATH=f"{workdir}{os.pathsep}{os.environ['PATH']}",
                GCP_PROJECT="test-project",
                GCP_REGION="test-region",
                GCLOUD_CALLS=str(calls_path),
                FAIL_WORKLOADS=",".join(failures),
            )
            result = subprocess.run(
                ["bash", "-e", "-c", rollback_script()],
                cwd=workdir,
                env=environment,
                capture_output=True,
                text=True,
                check=False,
            )
            calls = calls_path.read_text().splitlines() if calls_path.exists() else []
            return result, calls

    def test_restores_every_workload(self):
        result, calls = self.run_rollback()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, list(WORKLOADS))

    def test_service_failure_does_not_skip_jobs(self):
        result, calls = self.run_rollback(failures=(WORKLOADS[0],))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, list(WORKLOADS))

    def test_multiple_failures_do_not_skip_later_jobs(self):
        result, calls = self.run_rollback(failures=WORKLOADS[1:3])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, list(WORKLOADS))

    def test_missing_snapshots_are_skipped(self):
        result, calls = self.run_rollback(snapshots=(SNAPSHOTS[-1],))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [WORKLOADS[-1]])


if __name__ == "__main__":
    unittest.main()
