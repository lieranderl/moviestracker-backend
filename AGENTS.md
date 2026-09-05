# MoviesTracker Backend

Single Go 1.26.8 module for three Cloud Run workloads under `cmd/`. Shared code
lives under `internal/` by responsibility: `catalog`, `imdb`, and `platform`.

Rules:

- propagate caller `context.Context` through HTTP, workers, Firestore, and GCS;
- keep parsing, enrichment, persistence, and orchestration separate;
- use Firestore Native and ADC only; MongoDB/Firebase paths are retired;
- Rutor is the catalog source; Kinozal and GetTorrentsForMovie are retired;
- IMDb API compatibility is locked to `GET /getimdb?imdb_id=tt…` and JSON keys
  `Id`, `Rating`, `Votes`;
- tests are deterministic and offline; emulator tests must skip when unavailable;
- production changes use `infra/` and release-gated GitHub Actions only;
- never apply infrastructure or deploy from a local maintenance task.

Run `make test`, `make race`, `make cover`, `make vet`, `make lint`, `make vuln`,
and `make build` before completion.
