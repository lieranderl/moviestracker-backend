# Security and code logic review — 2026-10-30

Scope: all three Go workloads, catalog and IMDb logic, cloud adapters, Docker,
GitHub workflows, and declared infrastructure. Static review and local checks;
no live GCP inspection, deployment, infrastructure plan/apply, or state access.

No confirmed critical/high security defect, authentication bypass, or data race
was identified. All seven medium-priority findings and the cleanup item have
been fixed locally, with regression coverage. Deployment and the production
infrastructure plan/apply remain outside this task.

## Original findings

The findings and source line references below describe the pre-fix code.

### 1. P2 — Malformed IMDb snapshots bypass the minimum-data safeguard

[internal/imdb/index.go:119](internal/imdb/index.go#L119) counts every overflow
row, even when it overwrites the same map entry. Compact duplicate IDs also
inflate the row count. Both publishing and refresh validation trust that count.
A malformed upstream export can therefore replace a good index with one that
returns 404 for every valid title.

Offline reproduction, passing one million `"\t\t\n"` rows to
`ParseRatingsWithStats` and then `CheckMinRows`:

```text
parse_error=<nil> stats_rows=1000000 usable_index_entries=1 floor_error=<nil>
```

Remediation: validate distinct, usable title IDs and reject unusable empty IDs;
retain the required overflow fallback for legitimate values outside the compact
representation. Add duplicate-ID and malformed-snapshot regressions.

### 2. P2 — Third-party response sizes are not consistently bounded

[internal/catalog/rutor.go:53](internal/catalog/rutor.go#L53) feeds an unbounded
response into a DOM parser, with three pages fetched concurrently in a 512 MiB
job. [internal/catalog/tmdb.go:90](internal/catalog/tmdb.go#L90) likewise decodes
unbounded JSON. Large or unexpectedly compressed upstream responses can exhaust
memory and terminate a whole run instead of degrading one source/item.

IMDb bounds the compressed download to 64 MiB, but
[internal/imdb/download.go:44](internal/imdb/download.go#L44) and the streamed
GCS refresh do not bound the total decompressed input or parsed entries. A highly
compressible archive can exceed the index memory budget before validation ends.
These are upstream/operator trust boundaries, not public request-body inputs.

Remediation: enforce explicit limits on decompressed HTML/JSON/ratings bytes and
index growth, reject oversized input, and preserve streaming. Measure any added
validation against the existing 40 MiB index budget.

### 3. P2 — A due refresh can fail a lookup despite an available index

[internal/imdb/refresh.go:129](internal/imdb/refresh.go#L129) synchronously runs
the storage check on the winning lookup request without its own deadline.
[cmd/getimdbrating/main.go:81](cmd/getimdbrating/main.go#L81) sets a 15-second
write timeout, which does not cancel the storage operation. A storage stall can
consume the response window while the old index remains valid.

Remediation: derive a bounded refresh context from the caller with enough time
left to serve the existing index; test a source that blocks until cancellation.
Keep request-path refresh and coalescing under the existing CPU/scale settings.

### 4. P2 — Tracker challenge/error pages count as successful runs

[internal/catalog/rutor.go:62](internal/catalog/rutor.go#L62) accepts arbitrary
HTTP 200 HTML. If no torrent rows exist, parsing returns an empty successful
batch. All sources can return challenge pages while the runner logs completion
with zero failures and zero parsing rejections, masking stale catalog data.

Offline reproduction with `<html><body>Checking your browser...</body></html>`:

```text
error=<nil> fetched=0 rejected=0 torrents=0
```

Remediation: distinguish an expected tracker page or legitimate empty-results
marker from an unexpected document, failing only the affected source.

### 5. P2 — Older catalog writes can regress the frontend ordering timestamp

[internal/platform/firestore/repository.go:38](internal/platform/firestore/repository.go#L38)
unconditionally replaces documents. Deduplication preserves the newest
`lasttimefound` only within the current batch. A degraded scrape that loses the
newest release, or an older overlapping job finishing last, can overwrite a
newer persisted timestamp and move a film backwards in frontend ordering.

Remediation: preserve the maximum persisted timestamp with a concurrency-safe
transaction or equivalent precondition strategy. Test out-of-order writes using
the emulator; account for the additional read/transaction cost.

### 6. P2 — Rollback skips remaining workloads after its first failure

[.github/workflows/release.yml:187](.github/workflows/release.yml#L187) runs under
GitHub's default fail-fast shell. If the first image restoration fails, the
remaining catalog/downloader restorations are skipped, leaving a mixed release.

Local shell reproduction:

```sh
bash -e -c 'test ! -s /etc/hosts || { echo attempting-imdb; false; }; echo attempting-catalog'
```

Output: `attempting-imdb`, exit 1; the catalog restoration is never attempted.
Remediation: attempt each restoration, collect failures, then exit nonzero.

### 7. P2 — IMDb freshness alert expires before the next scheduled download

[infra/prod/monitoring.tf:163](infra/prod/monitoring.tf#L163) uses an absence
duration of 82,800 seconds (23 hours), while
[infra/prod/locals.tf:22](infra/prod/locals.tf#L22) schedules a daily download.
Healthy daily completion events therefore leave a gap longer than the threshold.
This is a configuration finding; actual incident behavior was not checked live.

Remediation: use a freshness calculation with a window longer than the schedule
plus processing grace, such as a suitable PromQL policy or last-success check.
Simply increasing this absence duration to 25 hours is invalid: Cloud Monitoring
[limits metric absence to 23.5 hours](https://docs.cloud.google.com/monitoring/alerts/metric-absence).

### 8. P3 — Release smoke check still references the retired gateway

[.github/workflows/release.yml:166](.github/workflows/release.yml#L166) still reads
`GC_API_KEY` and conditionally probes `API_GATEWAY_URL`. It currently skips when
the retired secret is absent. Remove the optional gateway branch to match the
frozen IAM-private IMDb contract; do not recreate the secret or gateway.

## Resolutions

| Finding | Fix and regression evidence |
|---|---|
| 1. IMDb row-floor bypass | Skip malformed/empty IDs and fields; reject duplicate titles across compact and overflow storage. Tests cover all duplicate orderings and snapshot retention. |
| 2. Unbounded responses | Cap decoded Rutor HTML at 4 MiB and TMDB JSON at 1 MiB, including trailing data. Cap streamed IMDb input at 64 MiB, total entries at 2.5 million, and overflow at 4,096 rows / 4 MiB of field data. Oversize, gzip-expansion, and index-growth tests pass. |
| 3. Refresh timeout | Derive a 10-second deadline from the caller for both refresh entry points; parsing observes cancellation and never publishes a cancelled snapshot. A channel serializes refresh work while allowing cancelled waiters to exit. Fake-clock tests verify that stalled storage times out and the old rating is served. |
| 4. Tracker challenge pages | Reject documents without known torrent-row structure. Structured pages filtered to zero remain valid; partial-source degradation is preserved. Challenge and pipeline tests pass. |
| 5. Timestamp regression | Retrying per-document Firestore transactions preserve the greatest stored/incoming discovery timestamp; writes are bounded to 20 concurrent transactions. Real-SDK/in-memory-RPC tests cover out-of-order writes, conflict retries, cancellation, and all stored fields. |
| 6. Partial rollback | Attempt every available restoration, collect failures, then return nonzero. All four offline workflow tests pass; the original failed two. |
| 7. Premature freshness alert | Sum native DELTA counts over 25 hours, use zero for absent data, and require 15 minutes of sustained absence. Seven tests against the real PromQL engine cover isolated successes, zero intervals, absence, and grace boundaries. |
| 8. Retired gateway probe | Remove the gateway branch and both retired variable/secret references from the release workflow. |

The Firestore fix adds one document read and transaction RPCs per movie, with
SDK retries on conflicts; it preserves the existing document contract. No new
Go modules were added. Existing gRPC/protobuf requirements are now classified as
direct because the offline Firestore tests import them.

Without a captured legitimate empty-result page, rowless tracker responses
conservatively fail the source rather than count as healthy. A verified fixture
is required before adding an explicit empty-results exception.

CI now runs the rollback tests and the freshness-policy tests. The latter uses
Promtool 3.15.1 downloaded from its official release with a pinned SHA-256.

## Version updates

| Component | Before | After |
|---|---|---|
| Go module and digest-pinned builder image | 1.26.10 | 1.27.3 |
| Dockerfile frontend, now digest-pinned | 1.18 | 1.28.0 |
| OpenTofu, including CI | 1.12.6 | 1.13.2 |
| Google and Google Beta providers, including lockfiles | 7.42.0 | 8.9.0 |
| golangci-lint | 2.12.2 in CI | 2.15.0 |
| govulncheck | 1.7.0 in CI | 1.9.0 |
| pre-commit-hooks | 5.0.0 | 6.0.0 |

GitHub actions were updated to current release tags: checkout 7.0.1, setup-go
7.0.0, golangci-lint-action 9.3.0, setup-buildx-action 4.4.1,
build-push-action 7.4.0, sbom-action 0.24.3, attest-build-provenance 4.2.2,
upload-artifact 7.0.2, and download-artifact 8.0.2. Auth 3, Trivy action 0.36.0,
and setup-opentofu 2.0.2 were already current.

`go get -u all`, `go mod tidy`, and a fresh `go list -m -u -json all` confirmed
that all explicitly required Go modules, including the six direct dependencies,
were already current. No Go dependency or checksum changes were necessary.
The existing distroless Debian 12 nonroot digest also matches its current tag.

Versions were checked against official release APIs, the Go module proxy, and
container registries. See [Go downloads](https://go.dev/dl/),
[OpenTofu 1.13.2](https://github.com/opentofu/opentofu/releases/tag/v1.13.2), and
the [Google provider 8 migration guide](https://github.com/hashicorp/terraform-provider-google/blob/v8.9.0/website/docs/guides/version_8_upgrade.html.markdown).

## Verification evidence before fixes

With Go 1.27.3 and temporary, checksum-verified tooling in `.cache/review-tools`:

```sh
PATH="$PWD/.cache/review-tools:$PATH" make test race cover vet lint vuln build
```

Exit 0. All seven packages passed unit/race tests; coverage:
`74.0% (minimum 65.0%)`; lint: `0 issues.`; vulnerability scan:
`Your code is affected by 0 vulnerabilities.` All three binaries built.

`govulncheck -show verbose ./...` found one module-only advisory,
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), concerning the unmaintained
`golang.org/x/crypto/openpgp` packages. None of those packages is imported;
there are zero imported-package or reachable-symbol findings. There is no fixed
version for that unused package. No suppression was added.

Additional checks:

- `go mod verify`: `all modules verified`.
- `gofmt -l cmd internal`: empty output; `git diff --check`: passed.
- `actionlint` 1.7.12: exit 0, no workflow diagnostics.
- Five-second fuzz smoke tests: catalog 144,072 executions; IMDb 14,552; both PASS.
- `TestIndexMemoryBudget`: 1,713,579 rows in 21.7 MiB, below the unchanged 40 MiB budget.
- `BenchmarkParseRatings`, 200,000 generated rows, three iterations on Apple M4:
  96.4 ms/op, 29,274,176 B/op, 799,884 allocations/op, including input generation.
  This is a local sample, not evidence of a performance improvement.
- Isolated copies of both infrastructure stacks: OpenTofu 1.13.2
  `init -backend=false -upgrade`, then `validate`: configuration valid.
- Provider signatures verified; lock checksums checked for darwin_arm64 and
  linux_amd64. `tofu fmt -check -recursive infra`: exit 0.
- `GOOS=linux GOARCH=amd64 make build`: exit 0; `file bin/*` confirmed all
  three outputs are statically linked Linux x86-64 ELF binaries.

Infrastructure validation covers schemas only. Provider 8 is a major upgrade;
an administrator must review a saved production plan before applying it.
Neither the real bootstrap directory nor production backend/state was initialized.
No IAM, secret, API, Firestore rule, or WIF-trust configuration was changed.
Container builds/scans and emulator-backed integration were not run locally;
no Docker executable was available. All executed application tests were offline.
Existing attestation/reviewer-plan limitations remain unchanged.

## Verification after fixes

```sh
PATH="$PWD/.cache/review-tools:$PATH" make test race cover vet lint vuln build
```

All seven package suites passed; coverage is now **77.7%** (minimum 65.0%),
lint reports **0 issues**, and govulncheck reports **0 reachable vulnerabilities**.
The unused OpenPGP module-only advisory above is unchanged.

`TestIndexMemoryBudget` still measures **21.7 MiB** for 1,713,579 rows under the
unchanged 40 MiB budget. A three-iteration, 200,000-row parse benchmark measured
**91.2 ms/op, 29,274,114 B/op, 799,883 allocations/op**. These local samples do
not establish an improvement over the earlier 96.4 ms/op measurement.

`python3 scripts/test_release_workflow.py`: four tests, OK.
`python3 scripts/test_imdb_freshness.py --promtool <promtool-3.15.1>`: seven cases
passed in the real PromQL engine. `actionlint`, OpenTofu formatting, isolated
production schema validation, `go mod tidy -diff`, and diff whitespace checks
pass. Emulator-backed Firestore regression is present and skips when
`FIRESTORE_EMULATOR_HOST` is absent; it was not executed against an emulator.
Five-second parser fuzz checks passed again: 63,512 catalog executions and
11,134 IMDb executions. Linux/amd64 builds passed for all three workloads;
`file bin/*` confirmed statically linked x86-64 ELF outputs.
