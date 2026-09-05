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
- Cloud Storage generation-aware immutable IMDb index, refreshed every 15 min
- Cloud Run service/jobs with dedicated identities and scale-to-zero defaults
- OpenTofu 1.12.6 and Google providers 7.42.0
- GitHub release workflow with OIDC, image scans, SBOMs, attestations, reviewed
  saved plans, digest deployments, and smoke checks

## Known open issues

Tracked but not yet fixed, so the README does not overstate the current state:

- The IMDb ratings index peaks at ~479 MiB during refresh against a 512 MiB
  Cloud Run limit (measured, 1.71M rows).
- `get-imdb-rating-service` runs with CPU throttled outside requests, so the
  15-minute background refresh goroutine is not reliably scheduled.
- TMDB enrichment matches ~42% of the `latesttorrentsmovies` candidates because
  it only inspects the first search result and compares titles and years for
  exact equality.

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

## Infrastructure and release

See [`infra/README.md`](infra/README.md). Production must not be changed with
the removed local `gcloud_deploy.sh` paths.
