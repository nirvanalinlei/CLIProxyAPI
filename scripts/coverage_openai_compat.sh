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

coverpkg="${COVERPKG:-$(IFS=,; echo "${coverpkgs[*]}")}"

cover_out="$root_dir/coverage.out"
cover_xml="$root_dir/coverage-branch.xml"

go test ./... -coverpkg="$coverpkg" -coverprofile="$cover_out" -covermode=atomic

go-bcov -format sonar-cover-report < "$cover_out" > "$cover_xml"
branch_pct=$(python - <<'PY'
import sys
import xml.etree.ElementTree as ET
import subprocess
import re
import os

base_ref = os.environ.get("BASE_REF", "origin/main")

def git_env():
    env = os.environ.copy()
    if env.get("MSYSTEM") or env.get("MSYS2_PATH_TYPE") or env.get("MINGW_PREFIX"):
        return env
    git_file = os.path.join(os.getcwd(), ".git")
    if not os.path.isfile(git_file):
        return env
    try:
        with open(git_file, "r", encoding="utf-8") as fh:
            content = fh.read().strip()
    except OSError:
        return env
    if not content.startswith("gitdir:"):
        return env
    gitdir = content.split("gitdir:", 1)[1].strip()
    if re.match(r"^[A-Za-z]:[\\/]", gitdir):
        drive = gitdir[0].lower()
        rest = gitdir[2:].replace("\\", "/").lstrip("/")
        if os.path.isdir(f"/mnt/{drive}"):
            gitdir = f"/mnt/{drive}/{rest}"
        else:
            gitdir = f"/{drive}/{rest}"
    env["GIT_DIR"] = gitdir
    env["GIT_WORK_TREE"] = os.getcwd()
    return env

def git_diff():
    return subprocess.check_output(
        ["git", "diff", "-U0", "--no-color", f"{base_ref}...HEAD", "--", "*.go"],
        text=True,
        env=git_env(),
    )

try:
    diff = git_diff()
except subprocess.CalledProcessError as exc:
    sys.exit(f"git diff failed: {exc}")

if not diff.strip():
    print("SKIP")
    sys.exit(0)

changed = {}
new_files = set()
cur_file = None
is_new = False

for line in diff.splitlines():
    if line.startswith("diff --git "):
        cur_file = None
        is_new = False
        continue
    if line.startswith("new file mode"):
        is_new = True
        continue
    if line.startswith("+++ "):
        path = line[4:].strip()
        if path == "/dev/null":
            cur_file = None
            continue
        if path.startswith("b/"):
            path = path[2:]
        if path.endswith("_test.go"):
            cur_file = None
            is_new = False
            continue
        cur_file = path
        if is_new:
            new_files.add(path)
        continue
    if line.startswith("@@ "):
        if not cur_file:
            continue
        match = re.search(r"\+(\d+)(?:,(\d+))?", line)
        if not match:
            continue
        start = int(match.group(1))
        count = int(match.group(2) or "1")
        if count <= 0:
            continue
        lines = changed.setdefault(cur_file, set())
        lines.update(range(start, start + count))

scope_files = set(changed.keys()) | new_files
if not scope_files:
    print("SKIP")
    sys.exit(0)

try:
    tree = ET.parse("coverage-branch.xml")
except FileNotFoundError:
    sys.exit("missing coverage-branch.xml")

total = 0
covered = 0
seen = set()
for line in tree.iter():
    if not line.tag.endswith("file"):
        continue
    path = line.attrib.get("path", "")
    if not path:
        continue
    norm = path.replace("\\", "/")
    if norm not in scope_files:
        continue
    seen.add(norm)
    include_all = norm in new_files
    changed_lines = changed.get(norm, set())
    for child in line:
        if not child.tag.endswith("lineToCover"):
            continue
        branches = child.attrib.get("branchesToCover")
        if branches is None:
            continue
        try:
            b = int(branches)
            c = int(child.attrib.get("coveredBranches") or 0)
            line_no = int(child.attrib.get("lineNumber") or 0)
        except ValueError:
            continue
        if b <= 0:
            continue
        if include_all or line_no in changed_lines:
            total += b
            covered += c

missing = sorted(scope_files - seen)
if missing:
    sys.exit("missing coverage data for changed/new files: " + ", ".join(missing))

if total == 0:
    sys.exit("no branch data for changed/new files")

pct = covered / total * 100
print(f"{pct:.2f}")
PY
)

if [[ "$branch_pct" == "SKIP" ]]; then
  echo "no changed go files; skipping branch coverage gate"
  exit 0
fi

threshold=95
python - <<PY
import sys
branch = float("${branch_pct}")
threshold = float("${threshold}")
if branch < threshold:
    sys.exit(f"branch coverage {branch:.2f}% < {threshold:.2f}%")
print(f"branch coverage {branch:.2f}% >= {threshold:.2f}%")
PY
