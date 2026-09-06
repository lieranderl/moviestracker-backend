# MoviesTracker Backend

One Go module containing the active MoviesTracker backend workloads:

- `cmd/getmovies`: Rutor → TMDB → Firestore catalog job
- `cmd/getimdbrating`: constant-time IMDb lookup HTTP service
- `cmd/downloadimdbdata`: IMDb archive → Cloud Storage refresh job

Kinozal, MongoDB, Firebase Admin, Colly, the custom TMDB client, and the old
GetTorrentsForMovie service are intentionally removed. Qwik uses JacRed for
torrent search.

## Runtime architecture

- Go 1.26.8, context-aware `net/http`, goquery
- Firestore Native database `moviestracker` in `europe-west1`
- Cloud Storage generation-aware immutable IMDb index, streamed and refreshed by
  an authenticated downloader callback, with request-path refresh as fallback
- Cloud Run service/jobs with dedicated identities and scale-to-zero defaults
- OpenTofu 1.12.6 and Google providers 7.42.0
- GitHub release workflow with OIDC, image scans, SBOMs, digest deployments,
  fresh-data jobs, smoke checks, and automatic image rollback

## Known open issues

Tracked but not yet fixed, so the README does not overstate the current state:

- **Build provenance attestation does not work on this repository.** GitHub
  artifact attestations are unavailable for user-owned private repositories, and
  environment protection rules (required reviewers) are unavailable on the
  current billing plan. Releases therefore carry no cryptographic provenance and
  no human approval gate. Making the repository public, or moving it into an
  organisation on a supporting plan, would enable both.
- The IMDb ratings index holds ~1.71M rows in ~22 MiB and peaks at ~61 MiB
  across a refresh, against a 512 MiB Cloud Run limit. The service still
  requests 512 MiB, so there is room to lower it once real usage is observed;
  that needs an OpenTofu apply.
- The alias matcher and degraded-run alert require a production observation
  window before their resolution and false-positive rates can be compared with
  the checked-in corpus and previous production baseline.

## Local verification

```sh
make test
make race
make cover
make vet
make lint
make vuln
make build
```

Firestore and Cloud Storage use Application Default Credentials. Use emulators
for integration tests; no database credential is supported.

## Configuration

Copy `.env.example` to `.env` for local use. Required production secrets live
in Secret Manager and are attached only to the workload that needs them.
Catalog enrichment concurrency defaults to 10 and is capped at 20 through
`ENRICHER_CONCURRENCY`.

## Infrastructure and release

See [`infra/README.md`](infra/README.md). Production must not be changed with
the removed local `gcloud_deploy.sh` paths.
