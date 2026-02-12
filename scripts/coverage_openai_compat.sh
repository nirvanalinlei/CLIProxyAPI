#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root_dir"

if ! command -v go-bcov >/dev/null 2>&1; then
  echo "go-bcov not found. Install with: go install github.com/ashanbrown/go-bcov@latest" >&2
  exit 1
fi

coverpkgs=(
  ./internal/config
  ./internal/translator/openai/openai/responses
  ./internal/translator/openai/claude
  ./internal/runtime/executor
  ./sdk/api/handlers/openai
)

coverpkg=$(IFS=,; echo "${coverpkgs[*]}")

go test ./... -coverpkg="$coverpkg" -coverprofile=coverage.out -covermode=atomic

branch_pct=$(
  {
    go-bcov -profile coverage.out -format json 2>/dev/null || go-bcov -profile coverage.out
  } | python - <<'PY'
import json
import re
import sys

text = sys.stdin.read().strip()
if not text:
    sys.exit("no coverage output")

def emit(value):
    if value <= 1:
        value *= 100
    print(f"{value:.2f}")

try:
    data = json.loads(text)
    if isinstance(data, dict):
        total = data.get("total") or data.get("summary") or data
        for key in ("branch", "branches", "branch_coverage", "branchCoverage"):
            if key in total:
                emit(float(total[key]))
                sys.exit(0)
    raise ValueError("branch coverage not found in json")
except Exception:
    match = re.search(r"branches?[^0-9]*([0-9]+(?:\.[0-9]+)?)%", text, re.IGNORECASE)
    if not match:
        match = re.search(r"branch[^0-9]*([0-9]+(?:\.[0-9]+)?)", text, re.IGNORECASE)
    if not match:
        sys.exit("failed to parse branch coverage from go-bcov output")
    emit(float(match.group(1)))
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
