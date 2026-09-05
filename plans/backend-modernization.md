# Backend modernization implementation

The approved target is a single Go module, Firestore Native persistence, JWT
sessions in Qwik, generation-aware IMDb indexing, OpenTofu-managed GCP, and
release-gated GitHub Actions. MongoDB, Kinozal, Firebase Admin,
GetTorrentsForMovie, nested Go modules, and local production deploy scripts are
removed. The cutover is clean rather than dual-stack; only immediate release
rollback is retained.
