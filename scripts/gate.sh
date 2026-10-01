#!/usr/bin/env bash
# The merge gate the escapement kernel runs for forge (escapement ADR-005):
# the checks .github/workflows/test.yml runs, in one place, so a kernel ship
# and CI agree on what GREEN means. Exit 0 is GREEN; anything else FAILED.
# A phase argument, if given, is ignored: forge's gate is one pass.
set -euo pipefail
cd "$(dirname "$0")/.."

stage() { echo "==> gate: $1"; }

stage fmt
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "not gofmt'd:"; echo "$unformatted"
  exit 1
fi

stage vet
go vet ./...

stage build
go build ./...

stage test
race=""
# -race needs cgo; CI has it, and so does minis (gcc).
if [ "$(go env CGO_ENABLED)" = "1" ] && command -v gcc >/dev/null; then race="-race"; fi
cover=$(mktemp)
trap 'rm -f "$cover"' EXIT
go test $race -count=1 -coverprofile="$cover" ./...

stage coverage
total=$(go tool cover -func="$cover" | awk '/^total:/ {sub("%", "", $3); print $3}')
echo "total coverage: ${total}% (floor 70%)"
awk -v c="$total" 'BEGIN { exit !(c >= 70.0) }' || { echo "coverage ${total}% is below the 70% floor"; exit 1; }

stage examples
for example in examples/*/; do
  echo "building ${example}"
  (cd "$example" && go build .)
done

echo "==> gate: GREEN"
