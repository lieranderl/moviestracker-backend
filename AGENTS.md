# MoviesTracker Backend

Single Go module (`github.com/lieranderl/moviestracker-backend`) for three Cloud
Run workloads in GCP project `moviestracker-f07e2`, region `europe-west1`.

```
cmd/getmovies          Rutor -> TMDB -> Firestore catalog job (arg: collection)
cmd/getimdbrating      constant-time public IMDb rating HTTP service
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
- Client-side Firestore access is denied by security rules on purpose. The web
  app (`moviestracker-web`, from the `moviestracker-app` repository) uses
  Firestore server-side as `movies-web@` (`roles/datastore.user`).
  Authentication is the web app's concern, not this repo's.
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

- Internal `GET /getimdb?imdb_id=tt<digits>` returns JSON keys `Id`, `Rating`,
  `Votes` (all strings). Status codes: 200, 400 invalid id, 404 unknown id.
  The Cloud Run service is **public by decision** (`invoker_iam_disabled`):
  installed moviestracker-app clients (desktop, NAS, web) call it without
  credentials. Requiring a token would break every installed client.
- Because Cloud Run IAM is off, `POST /internal/refresh` authenticates in-app:
  it accepts only a Google ID token for the service URL issued to
  `IMDB_REFRESH_INVOKER` (`movies-imdb-downloader@`), and is not registered
  when that variable is unset. Never expose it without that check.
- API Gateway and `GC_API_KEY` are retired; do not restore the gateway or
  API-key authentication.
- Firestore collections `latesttorrentsmovies`, `hdr10movies`, `dvmovies` and
  the document field names in `internal/catalog.Movie` firestore tags,
  including the irregular `Year` key.

## Verified environment facts (checked against gcloud, not assumed)

These are counter-intuitive and have already caused wrong conclusions once.
Do not "clean them up" without re-verifying.

- **One TMDB secret, shared.** `TMDB_API_KEY` is read by all three
  `getmovies-*-job` workloads (as env `TMDBAPIKEY`) and by `moviestracker-web`.
  The former backend-only secret `TMDBAPIKEY` was deleted; do not recreate it.
- **The production Secret Manager inventory is five secrets** (checked
  2026-10-30): `AUTH_SECRET`, `GOOGLE_SECRET`, `JACRED_APIKEY`,
  `TMDB_API_KEY`, and `WEB_SESSION_KEY`. `AUTH_SECRET` belonged to the retired
  Qwik frontend and has no consumer. `GC_API_KEY`, Firebase configuration
  secrets, and the legacy Cloud Build GitHub OAuth token are retired and must
  not be recreated.
- **`/getimdb` is called by moviestracker-app clients**: the web server and
  installed desktop/NAS apps. Measure before adding response caching.
- **`min_instance_count` stays 0 by decision** (cost). Any freshness mechanism
  must work on the request path, not in a background goroutine.
- The frontend is `moviestracker-web` on `movies-web@`. The
  `moviestracker-app` repository's `web.yml` deploys its image and traffic as
  `movies-web-deployer@` through its own `moviestracker-web` identity pool;
  OpenTofu here owns its runtime settings, identity, and secrets. The Qwik
  service `moviestracker-app` and its `movies-app@` identity are retired.
  Legacy MongoDB, Firebase, API Gateway, and gateway-secret resources must
  converge to the retired state and must not be restored by a future apply.
- OpenTofu state was stale from 2026-09-07 until the reconciliation written on
  2026-10-30 was applied on 2026-10-31; a plan afterwards reported no changes.
  `.github/workflows/drift.yml` is meant to run a daily read-only plan, but its
  planner identity, `GCP_PLAN_*` variables, and `PROD_TFVARS_JSON` secret do
  not exist yet, so it fails at authentication. Until that is set up, run the
  read-only plan locally before each apply; once it works, treat a red drift
  run as a blocker for the next apply.
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
  requests) and `min_instance_count = 0`. Background goroutines are therefore
  **not reliably scheduled**. The downloader invokes authenticated
  `POST /internal/refresh` after publishing a generation; request-path refresh
  remains the coalesced fallback. Do not reintroduce a `time.Ticker` here
  without also disabling `cpu_idle` and accepting the cost.

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
- Operational commands use JSON `slog`. Log-based metrics in
  `infra/prod/monitoring.tf` match stable `jsonPayload.event` fields and catalog
  diagnostic fields. Update code and metric filters atomically when changing
  those event contracts.

## Delivery

- Production changes go through `infra/` (OpenTofu 1.13.2, google providers
  8.9.0, versions pinned with `=`) and the release-gated GitHub Actions
  workflow. Never apply infrastructure or deploy from a local task, and never
  run `gcloud run services update` / `jobs update` by hand.
- Images are deployed by digest only. `infra/prod/run.tf` intentionally sets
  `ignore_changes` on container images so releases, not OpenTofu, move them.
- **Build provenance is required.** The repository became public after v3.0.3,
  and v3.1.0 (2026-10-31) recorded a persisted attestation for every image, so
  the `attest-build-provenance` step no longer has `continue-on-error`. The
  `production` environment still has no required reviewer configured; that is a
  repository setting for the maintainer.
- **Releases move images only.** No workflow runs `tofu apply`; the maintainer
  applies `infra/prod` by hand. Code that depends on new infrastructure (an env
  var, IAM) must not be released before that apply: v3.1.0 first failed because
  `IMDB_REFRESH_INVOKER` from #31 had never been applied.
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
- The production reviewer gate is still unconfigured. Do not silently change
  adjacent delivery controls while doing an unrelated task; keep changes scoped
  and mention what you noticed.
- Prefer the standard library. The dependency set is deliberately small:
  Firestore, Cloud Storage, goquery, `golang.org/x/sync`, testify. Ask before
  adding a direct dependency.
- Report evidence, not intent. Quote the command you ran and its output.
