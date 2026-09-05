# Contributing

Use a feature branch and keep changes in the root Go module. Add deterministic,
offline tests for changed behavior and pass `make verify` before opening a PR.

Key rules:

- propagate caller contexts through all I/O and worker boundaries;
- return errors outside `main` instead of terminating the process;
- use ADC and dedicated service identities, never credential strings;
- preserve `GET /getimdb?imdb_id=tt…` response/error semantics;
- keep tracker parsing, TMDB enrichment, persistence, and orchestration separate;
- do not restore MongoDB, Kinozal, Firebase Admin, or GetTorrentsForMovie.

Production changes use a reviewed OpenTofu plan from a published GitHub release.
