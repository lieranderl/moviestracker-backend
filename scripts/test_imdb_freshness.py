#!/usr/bin/env python3
"""Check the deployed freshness expression with an installed promtool, offline."""

import argparse
from pathlib import Path
import re
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--promtool", default="promtool")
    arguments = parser.parse_args()
    source = (ROOT / "infra/prod/monitoring.tf").read_text().split(
        'resource "google_monitoring_alert_policy" "imdb_stale"', 1
    )[1]
    query = re.search(r"query\s*= <<-EOT\n(.*?)\n\s*EOT", source, re.S).group(1)
    query = query.replace(
        "${google_logging_metric.imdb_download_success.name}",
        "moviestracker/imdb_download_success",
    ).replace("${var.project_id}", "test-project")
    duration = re.search(r'duration\s*=\s*"(\d+)s"', source).group(1)
    evaluation_interval = re.search(
        r'evaluation_interval\s*=\s*"(\d+)s"', source
    ).group(1)
    # Promtool checks PromQL semantics, but not Cloud Monitoring's platform
    # limit. This policy has one range selector and no offset: its lookback and
    # retest window together must not exceed 25 hours for a log-based metric.
    ranges = re.findall(r"\[([0-9hms]+)\]", query)
    if len(ranges) != 1 or re.search(r"\boffset\b", query):
        raise ValueError("freshness window validation requires one range and no offset")
    parts = re.findall(r"(\d+)([hms])", ranges[0])
    if "".join(value + unit for value, unit in parts) != ranges[0]:
        raise ValueError("unsupported freshness range duration")
    lookback_seconds = sum(int(value) * {"h": 3600, "m": 60, "s": 1}[unit] for value, unit in parts)
    if lookback_seconds + int(duration) > 25 * 3600:
        raise ValueError("Cloud Monitoring lookback plus retest window exceeds 25 hours")
    series = (
        '{__name__="logging.googleapis.com/user/moviestracker/imdb_download_success", '
        'monitored_resource="cloud_run_job", project_id="test-project", '
        'job_name="downloadimdbdata-job"}'
    )
    rule = (
        "groups:\n  - name: imdb-freshness\n"
        f"    interval: {evaluation_interval}s\n"
        "    rules:\n      - alert: ImdbStale\n        expr: |\n"
        + "\n".join("          " + line.strip() for line in query.splitlines())
        + f"\n        for: {duration}s\n"
    )
    cases = (
        ("isolated-success-24-hours-ago", "1 _x1599", "24h", False),
        ("isolated-success-just-now", "_x1439 1", "24h", False),
        ("success-surrounded-by-zero-intervals", "0+0x1439 1 0+0x59", "25h", False),
        ("grace-period-before-firing", "1 _x1599", "24h59m", False),
        ("stale-after-grace-period", "1 _x1599", "25h1m", True),
        ("only-zero-intervals", "0+0x59", "16m", True),
        ("entirely-absent-series", None, "16m", True),
    )
    tests = "rule_files:\n  - rules.yml\nevaluation_interval: 1m\ntests:\n"
    for name, values, when, firing in cases:
        tests += f"  - name: {name}\n    interval: 1m\n"
        if values is not None:
            tests += (
                f"    input_series:\n      - series: '{series}'\n"
                f"        values: '{values}'\n"
            )
        tests += f"    alert_rule_test:\n      - eval_time: {when}\n        alertname: ImdbStale\n"
        tests += (
            "        exp_alerts:\n          - exp_labels: {}\n            exp_annotations: {}\n"
            if firing
            else "        exp_alerts: []\n"
        )
    with tempfile.TemporaryDirectory() as directory:
        workdir = Path(directory)
        (workdir / "rules.yml").write_text(rule)
        (workdir / "tests.yml").write_text(tests)
        subprocess.run(
            [arguments.promtool, "test", "rules", "tests.yml"],
            cwd=workdir,
            check=True,
        )
    print(f"IMDb freshness: {len(cases)} offline PromQL cases passed")


if __name__ == "__main__":
    main()
