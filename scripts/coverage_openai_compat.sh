#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root_dir"

gopath="$(go env GOPATH)"
if [[ -n "$gopath" ]]; then
  if [[ "$gopath" =~ ^[A-Za-z]:\\\\ ]]; then
    drive="${gopath:0:1}"
    rest="${gopath:2}"
    rest="${rest//\\\\//}"
    gopath="/${drive,,}/${rest}"
  fi
  export PATH="$gopath/bin:$PATH"
fi

if ! command -v go-bcov >/dev/null 2>&1; then
  echo "go-bcov not found. Install with: go install github.com/alx99/go-bcov@v1" >&2
  exit 1
fi

coverpkgs=(
  ./internal/config
  ./internal/api/handlers/management
  ./internal/watcher/diff
  ./internal/translator/openai/openai/responses
  ./internal/translator/openai/claude
  ./internal/runtime/executor
  ./sdk/api/handlers/openai
)

coverpkg=$(IFS=,; echo "${coverpkgs[*]}")

cover_out="$root_dir/coverage.out"
cover_xml="$root_dir/coverage-branch.xml"

go test ./... -coverpkg="$coverpkg" -coverprofile="$cover_out" -covermode=atomic

go-bcov -format sonar-cover-report < "$cover_out" > "$cover_xml"
branch_pct=$(python - <<'PY'
import sys
import xml.etree.ElementTree as ET

try:
    tree = ET.parse("coverage-branch.xml")
except FileNotFoundError:
    sys.exit("missing coverage-branch.xml")

total = 0
covered = 0
for line in tree.iter():
    if line.tag.endswith("lineToCover"):
        branches = line.attrib.get("branchesToCover")
        covered_branches = line.attrib.get("coveredBranches")
        if branches is None:
            continue
        try:
            b = int(branches)
            c = int(covered_branches or 0)
        except ValueError:
            continue
        total += b
        covered += c

if total == 0:
    sys.exit("no branch data in coverage report")

pct = covered / total * 100
print(f"{pct:.2f}")
PY
)

threshold=95
python - <<PY
import sys
branch = float("${branch_pct}")
threshold = float("${threshold}")
if branch < threshold:
    sys.exit(f"branch coverage {branch:.2f}% < {threshold:.2f}%")
print(f"branch coverage {branch:.2f}% >= {threshold:.2f}%")
PY
