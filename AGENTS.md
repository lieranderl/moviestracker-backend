# MoviesTracker Backend

Single Go module (`github.com/lieranderl/moviestracker-backend`) for three Cloud
Run workloads in GCP project `moviestracker-f07e2`, region `europe-west1`.

```
cmd/getmovies          Rutor -> TMDB -> Firestore catalog job (arg: collection)
cmd/getimdbrating      constant-time IMDb rating HTTP service (behind API Gateway)
cmd/downloadimdbdata   IMDb archive -> Cloud Storage refresh job
internal/catalog       scraping, parsing, TMDB enrichment, pipeline orchestration
internal/imdb          ratings index, snapshot manager, download, HTTP handler
internal/platform/...   Firestore and Cloud Storage adapters only
infra/bootstrap        state bucket + GitHub OIDC trust (LOCAL state - see below)
infra/prod             all production resources (GCS-backed state)
```

## Architecture rules

- Propagate the caller `context.Context` through HTTP, workers, Firestore, and
  Cloud Storage. Never create a background `context.Background()` in a request
  or job path.
- Keep the layers separate: parsing, enrichment, persistence, orchestration.
  `internal/platform` must not contain business logic; `internal/catalog` and
  `internal/imdb` must not import cloud SDKs beyond their adapter interfaces.
- Persistence is Firestore Native database `moviestracker` via
  `cloud.google.com/go/firestore` with Application Default Credentials.
  Do not add the Firebase Admin SDK, MongoDB, or any database credential.
- Client-side Firestore access is denied by security rules on purpose. The Qwik
  frontend reads Firestore server-side as `movies-app@` (`roles/datastore.viewer`).
  Firebase Auth / Identity Platform is the frontend's concern, not this repo's.
- Rutor is the only catalog source. Kinozal, Colly, and `GetTorrentsForMovie`
  are removed; the frontend uses JacRed for torrent search.
- TMDB enrichment scores **every** search result and takes the best above a
  confidence floor (`internal/catalog/match.go`). Never reduce this to
  `Results[0]` with exact title and year equality: that measured 42% on
  `latesttorrentsmovies`. Tracker years drift by one against TMDB release dates,
  titles differ by punctuation and abbreviation, and a film's canonical title is
  often in a third language. The `confidenceWeak` tier deliberately trusts TMDB's
  top-ranked result on an exact year, because TMDB has already matched the query
  against alternative titles that its response does not expose.

## Locked external contracts

Changing any of these breaks a live consumer. Treat as frozen unless the task
explicitly says otherwise.

- `GET /getimdb?imdb_id=tt<digits>` returning JSON keys `Id`, `Rating`, `Votes`
  (all strings). Status codes: 200, 400 invalid id, 404 unknown id.
- API Gateway `moviestracker-gw-eu-w1` fronts the service. Access needs an API
  key **and** a `Referer` of `moviestracker.net` or `localhost`; the key is
  browser-referrer restricted, so `curl` without `Referer` returns 403.
- Firestore collections `latesttorrentsmovies`, `hdr10movies`, `dvmovies` and
  the document field names in `internal/catalog.Movie` firestore tags,
  including the irregular `Year` key.

## Verified environment facts (checked against gcloud, not assumed)

These are counter-intuitive and have already caused wrong conclusions once.
Do not "clean them up" without re-verifying.

- **Two TMDB secrets exist and both are live.** `TMDBAPIKEY` is the *backend*
  key, consumed by all three `getmovies-*-job` workloads and granted to
  `movies-catalog-jobs@`. `TMDB_API_KEY` is the *frontend* key, consumed by the
  `moviestracker-app` Cloud Run service. They are not duplicates. Deleting
  either breaks a live workload.
- **`/getimdb` is called by Qwik SSR, not by browsers.** Response caching
  headers (`Cache-Control`, `ETag`) therefore buy nothing; do not add them as a
  performance measure.
- **`min_instance_count` stays 0 by decision** (cost). Any freshness mechanism
  must work on the request path, not in a background goroutine.
- **The frontend still mounts `MONGO_URI`** and runs as the *default compute*
  service account, which holds project-wide `roles/editor`. The `movies-app@`
  identity that `infra/prod/iam.tf` creates is currently unused. Do not delete
  the MongoDB or Firebase secrets, and do not assume `movies-app@` is wired up;
  both are frontend-repo concerns that must be coordinated first.
- Firestore queries order on `lasttimefound`. Other `Movie` fields are not
  filtered or ordered on, so they are candidates for single-field index
  exemptions.

## Hard resource limits (measured, not guessed)

- The IMDb dataset is ~1.71M rows. The index stores each row as a 12-byte
  `entry` (`uint32` id, `uint32` votes, `uint8` rating scaled by ten) in one
  sorted slice, costing **~22 MiB resident**. `Manager.Refresh` builds the
  replacement while the outgoing index still serves, peaking at **~61 MiB** in a
  512 MiB container. `TestIndexMemoryBudget` fails the build above 40 MiB for a
  single index; do not raise that budget to make a change fit.
- The compact encoding is only valid because the published data is regular:
  identifiers fit `uint32` (max observed `tt45115568`), ratings always carry
  exactly one decimal, and votes fit `uint32` (max observed 3,233,728). Rows that
  violate this fall back to an `overflow` map so bad input costs memory rather
  than correctness. Do not remove that fallback.
- `Index.Lookup` accepts only IMDb's canonical zero-padded form, matching the
  string-keyed map it replaced: `tt0133093` resolves and `tt133093` does not.
  Relaxing this changes the frozen HTTP contract.
- The ratings snapshot is **streamed** from Cloud Storage through gzip into the
  parser. Do not reintroduce an `io.ReadAll` of the decompressed export; that
  alone previously cost ~30 MiB per refresh.
- `get-imdb-rating-service` runs with `cpu_idle = true` (CPU only during
  requests) and `min_instance_count = 0`. Background goroutines such as the
  refresh ticker are therefore **not reliably scheduled** in production. Do not
  add background timers to this service without also changing that setting and
  accepting the cost, or moving the work onto the request path.

## Verification

Run before declaring work complete:

```sh
make test race cover vet lint vuln build
```

- `make cover` enforces a threshold (`COVERAGE_THRESHOLD`, currently 65).
- Tests are deterministic and offline. No test may reach Rutor, TMDB, IMDb,
  Firestore, or Cloud Storage. Emulator-backed tests must skip when the
  emulator is absent.
- If a change plausibly affects memory or throughput, add a benchmark or a
  measurement and report the numbers rather than asserting improvement.
- Log-based metrics in `infra/prod/monitoring.tf` match on log **text**
  (`"catalog job completed"`, `"IMDb ratings updated"`,
  `"IMDb index refresh failed"`). Changing those strings or moving to a JSON
  slog handler breaks the alerts. Update `monitoring.tf` in the same change.

## Delivery

- Production changes go through `infra/` (OpenTofu 1.12.6, google providers
  7.42.0, versions pinned with `=`) and the release-gated GitHub Actions
  workflow. Never apply infrastructure or deploy from a local task, and never
  run `gcloud run services update` / `jobs update` by hand.
- Images are deployed by digest only. `infra/prod/run.tf` intentionally sets
  `ignore_changes` on container images so releases, not OpenTofu, move them.
- **Two GitHub platform features the docs used to assume are unavailable here.**
  Artifact attestations fail with "Feature not available for user-owned private
  repositories", and environment protection rules (required reviewers) are
  rejected by the billing plan. The `attest-build-provenance` step is therefore
  `continue-on-error: true`. Do not present the release as cryptographically
  attested or human-gated until the repository is public or org-owned on a
  supporting plan.
- `infra/bootstrap` has **no remote backend**; its state is local and
  untracked. Do not run it opportunistically. Treat it as a break-glass
  administrator task and confirm with the maintainer first.
- Ask before: enabling or disabling a GCP API, changing IAM, creating or
  deleting a secret, touching Firestore security rules, changing the API
  Gateway config, or altering the WIF trust condition.

## Working style

- A catalog run degrades rather than aborting: a failed tracker page or a failed
  TMDB lookup costs that item, and the run only fails if every source failed or
  more than `maxFailureRatio` of lookups failed (`internal/catalog/runner.go`).
  Do not reintroduce `errgroup.WithContext` there; cancelling siblings on the
  first error is what previously turned one bad page into a lost run.
- This repo has known open defects that are documented but not yet fixed
  (IMDb refresh under `cpu_idle`, no alert on a partially degraded catalog run,
  GitHub attestations and reviewer gate unavailable on this plan). Do not
  silently "fix" adjacent code while doing an unrelated task; keep changes
  scoped and mention what you noticed.
- Prefer the standard library. The dependency set is deliberately small:
  Firestore, Cloud Storage, goquery, `golang.org/x/sync`, testify. Ask before
  adding a direct dependency.
- Report evidence, not intent. Quote the command you ran and its output.
