#!/bin/sh
# Prints one "<actions> <address>" line per resource a saved infra/prod plan
# would change, sorted, and nothing else.
#
# The JSON plan carries attribute values, including sensitive ones, so it is
# only ever piped into jq here and never printed. The output is safe for a
# public log, and it is what the infrastructure workflow fingerprints to prove
# that the apply job applies the changes that were approved.
set -eu
tofu -chdir=infra/prod show -json "$1" | jq -r '
  [.resource_changes[]?
    | select(.change.actions != ["no-op"])
    | "\(.change.actions | join("+")) \(.address)"]
  | sort
  | .[]'
