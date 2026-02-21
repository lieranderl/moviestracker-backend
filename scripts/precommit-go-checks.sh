#!/usr/bin/env bash
set -euo pipefail

# Runs go vet and go test only in modules affected by staged files.
ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

if [ "$#" -eq 0 ]; then
  echo "No files provided to pre-commit hook; skipping Go module checks."
  exit 0
fi

find_module_root() {
  local path="$1"
  local dir

  dir="$(dirname "$path")"
  while [ "$dir" != "." ] && [ "$dir" != "/" ]; do
    if [ -f "$dir/go.mod" ]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  return 1
}

module_list=""
for file in "$@"; do
  if [ ! -e "$file" ]; then
    continue
  fi

  if module_root="$(find_module_root "$file")"; then
    module_list+="$module_root"$'\n'
  fi
done

if [ -z "$module_list" ]; then
  echo "No Go modules touched; skipping Go module checks."
  exit 0
fi

# shellcheck disable=SC2016
unique_modules="$(printf '%s' "$module_list" | awk 'NF' | sort -u)"

while IFS= read -r module; do
  [ -z "$module" ] && continue

  echo "==> pre-commit checks for $module"
  (
    cd "$ROOT_DIR/$module"
    go vet ./...
    go test ./...
  )
done <<< "$unique_modules"
