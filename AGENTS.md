# AGENTS.md

## Project
MoviesTracker backend monorepo with independent Go modules for Cloud Run services and scheduled jobs.

## Repository Layout
- `SERVICES/GetTorrentsForMovie`: HTTP service to return torrents for a movie query.
- `SERVICES/GetImdbRating`: HTTP service to fetch IMDb rating by `imdb_id`.
- `JOBS/GetMovies`: batch pipeline job that fetches torrents, enriches via TMDB, and stores to DB.
- `JOBS/DownloadImdbData`: batch job that downloads IMDb datasets.

## Development Rules
- Treat each module (`go.mod`) as a standalone unit.
- Keep changes scoped to the target module.
- Prefer small, testable functions and deterministic handlers.
- Never hardcode secrets; use env vars / secret managers.
- For request handlers, validate inputs and return explicit HTTP status codes.

## Local Commands
Run commands from the module directory you are changing.

### Services
- `go run ./cmd`
- `go test ./...`

### GetTorrentsForMovie Integration Test
- Standard tests: `go test ./...`
- Live test (network + credentials required):
  - `go test -tags=integration ./cmd -run TestHandleRequest_Integration_TheMummyReturns2001 -v`

## Environment Variables
### `SERVICES/GetTorrentsForMovie`
- `RUTOR_SEARCH_URL`
- `KZ_SEARCH_URL`
- `KZ_LOGIN`
- `KZ_PASSWORD`
- `TMDBAPIKEY`

Handler also supports `key` query param as TMDB API key override.

### `SERVICES/GetImdbRating`
- No mandatory env vars for basic startup.

### `JOBS/GetMovies`
- `TMDBAPIKEY`
- `RUTOR_URLS` / `RUTOR_HDR10_URLS` / `RUTOR_DV_URLS` (based on job argument)
- `MONGO_URI` (for `mongo` mode)
- `MONGO_URI` is required (`moviestracker-package v1.2.0` supports `mongo` only for `GetMovies`)

## Deployment
Each module has its own `gcloud_deploy.sh`. Keep deploy script changes minimal and verify required env/secret names match runtime code.

## Skills To Use In This Project
Use these skills when applicable:
- `backend-dev-guidelines`: default for backend code changes (handlers, pipelines, module structure).
- `testing-patterns`: adding/changing unit or integration tests.
- `systematic-debugging`: any failing test, runtime error, or behavior regression.
- `verification-before-completion`: before claiming work complete; run relevant tests/commands first.
- `security-review`: when touching auth, secrets, env handling, request inputs, or external integrations.
- `deployment-procedures`: when changing Dockerfiles, deploy scripts, or rollout behavior.
- `clean-code`: refactors to simplify logic without changing behavior.

## Completion Checklist
- Code compiles in affected module.
- Relevant tests pass in affected module.
- No secrets added to tracked files.
- Changes are limited to requested scope.
