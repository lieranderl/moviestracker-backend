# MoviesTracker Backend

Go-based backend monorepo for MoviesTracker, split into independent modules for deployable services and scheduled jobs.

## Architecture

### Services
- `SERVICES/GetTorrentsForMovie`: HTTP API that searches trackers and returns torrent candidates.
- `SERVICES/GetImdbRating`: HTTP API that returns IMDb rating data for an `imdb_id`.

### Jobs
- `JOBS/GetMovies`: Batch pipeline that fetches torrents, enriches with TMDB, and persists to DB.
- `JOBS/DownloadImdbData`: Batch job that downloads and stores IMDb ratings dataset.

## Prerequisites
- Go `1.25+`
- Docker (optional, for container builds)
- `gcloud` CLI (optional, for Cloud Run deploy scripts)

## Quick Start
1. Copy environment template and set values:
   - `cp .env.example .env`
2. Run tests for all modules:
   - `make test`
3. Build all modules:
   - `make build`

## Common Commands
- Format: `make fmt`
- Tidy modules: `make tidy`
- Unit tests: `make test`
- Race tests: `make race`
- Coverage: `make cover`
- Vet: `make vet`
- Lint (golangci-lint): `make lint`
- Static analysis: `make staticcheck`
- Security scan: `make gosec`
- Install pre-commit hooks: `make precommit-install`
- Run pre-commit checks manually: `make precommit-run`

## Pre-commit Checks
This repo includes `.pre-commit-config.yaml` with useful checks:
- merge conflict markers, YAML validation, EOF/newline/whitespace cleanup
- private key + large file detection
- `gofmt` for staged Go files
- `go vet` + `go test` for changed Go modules only

Setup:
1. Install `pre-commit` (for example: `pipx install pre-commit`)
2. Run `make precommit-install`

## Module-Specific Run Commands
- GetTorrentsForMovie:
  - `cd SERVICES/GetTorrentsForMovie && go run ./cmd`
- GetImdbRating:
  - `cd SERVICES/GetImdbRating && go run ./cmd`
- GetMovies job:
  - `cd JOBS/GetMovies && go run ./cmd latesttorrentsmovies mongo`
- DownloadImdbData job:
  - `cd JOBS/DownloadImdbData && go run ./cmd`

## Testing Notes
- Standard tests: `make test`
- Integration test for tracker flow (live external dependencies):
  - `cd SERVICES/GetTorrentsForMovie && go test -tags=integration ./cmd -run TestHandleRequest_Integration_TheMummyReturns2001 -v`

## Configuration
See `.env.example` for all variables.

High-impact variables:
- Tracker service: `RUTOR_SEARCH_URL`, `KZ_SEARCH_URL`, `KZ_LOGIN`, `KZ_PASSWORD`, `TMDBAPIKEY`
- Movies job (`mongo` only with `moviestracker-package v1.2.0`): `RUTOR_URLS`, `RUTOR_HDR10_URLS`, `RUTOR_DV_URLS`, `TMDBAPIKEY`, `MONGO_URI`, `JOB_TIMEOUT`
- IMDb rating service: `IMDB_DATA_PATH` (optional override)

## Deployment
Each module includes its own `gcloud_deploy.sh` script:
- `SERVICES/GetTorrentsForMovie/gcloud_deploy.sh`
- `SERVICES/GetImdbRating/gcloud_deploy.sh`
- `JOBS/GetMovies/gcloud_deploy.sh`
- `JOBS/DownloadImdbData/gcloud_deploy.sh`

## CI
GitHub Actions workflow: `.github/workflows/ci.yml`
- quality gates: format check, vet, test, race, staticcheck, gosec, golangci-lint
- build verification: binary build and Docker build per module
- coverage artifact upload per module

If you want README badges, replace `<OWNER>` and `<REPO>` in these URLs:
- `https://github.com/<OWNER>/<REPO>/actions/workflows/ci.yml/badge.svg`
- `https://github.com/<OWNER>/<REPO>/actions/workflows/ci.yml`
