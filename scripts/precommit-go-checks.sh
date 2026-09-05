#!/usr/bin/env bash
set -euo pipefail

root_dir="$(git rev-parse --show-toplevel)"
cd "$root_dir"

go vet ./...
go test ./...
tofu fmt -check -recursive infra
