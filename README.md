# MoviesTracker Backend

Go backend for MoviesTracker, running on Cloud Run in GCP.

| Workload | Purpose |
| --- | --- |
| `cmd/getmovies` | Job: scrapes Rutor, enriches with TMDB, writes the catalog to Firestore |
| `cmd/getimdbrating` | Public HTTP service: `GET /getimdb?imdb_id=tt…` returns an IMDb rating |
| `cmd/downloadimdbdata` | Job: refreshes the IMDb ratings dataset in Cloud Storage daily |

The web frontend (`moviestracker-web`) lives in the `moviestracker-app`
repository and reads the catalog from Firestore.

## Layout

```
cmd/            entry points for the three workloads
internal/       catalog pipeline, IMDb index, Firestore/GCS adapters
infra/          OpenTofu: bootstrap (CI identities) and prod (all resources)
.github/        CI, release, infrastructure apply, and drift workflows
```

## Development

Requires Go 1.27. Copy `.env.example` to `.env` for local runs; cloud access
uses Application Default Credentials.

```sh
make verify
```

Tests are offline and never call Rutor, TMDB, IMDb, Firestore, or Cloud Storage.

## Deployment

Production changes are made only through GitHub Actions:

- **Infrastructure**: changes to `infra/prod` merged to `master` are planned and,
  after approval, applied by `infra.yml`.
- **Code**: publishing a GitHub release builds, attests, and deploys images by
  digest via `release.yml`. Apply infrastructure before releasing code that
  depends on it.

See [infra/README.md](infra/README.md) for details.

## More

- [AGENTS.md](AGENTS.md): architecture rules, frozen contracts, and limits
- [CONTRIBUTING.md](CONTRIBUTING.md): contribution guide
- [SECURITY_REVIEW.md](SECURITY_REVIEW.md): security review and findings
