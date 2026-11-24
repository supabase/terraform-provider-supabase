#!/usr/bin/env bash
# Chooses which acceptance tests CI should run for a change.
#
# Inputs (environment):
#   BASE        Commit to diff against. Empty or unknown means "run everything".
#   EVENT_NAME  The GitHub event name, such as pull_request or push.
#
# Outputs (appended to $GITHUB_OUTPUT, or printed when it is unset):
#   acceptance  "true" when any acceptance tests should run, otherwise "false".
#   run         The regular expression to pass to "go test -run".
#
# Rules:
#   - Changes only under tools/, docs/, assets/ or templates/, or to README.md,
#     run nothing.
#   - On a pull request, when every other changed file is a resource or data
#     source (or its test) in internal/provider, run only the acceptance tests
#     in the matching test file.
#   - Anything else runs every acceptance test. This includes shared helpers,
#     the provider itself, test fixtures and every push.
set -euo pipefail

output="${GITHUB_OUTPUT:-/dev/stdout}"
ignored='^((tools|docs|assets|templates)/|README\.md$)'
resource_file='^internal/provider/([a-z0-9_]+_(resource|data_source))(_test)?\.go$'

emit() {
  printf 'acceptance=%s\nrun=%s\n' "$1" "$2" >> "$output"
}

run_all() {
  emit true TestAcc
  exit 0
}

if [ -z "${BASE:-}" ] || ! git cat-file -e "${BASE}^{commit}" 2>/dev/null; then
  run_all
fi

# --no-renames lists both sides of a rename, so a moved resource selects both.
relevant=$(git diff --no-renames --name-only "$BASE" HEAD | grep -Ev "$ignored" || true)
if [ -z "$relevant" ]; then
  emit false TestAcc
  exit 0
fi

if [ "${EVENT_NAME:-}" != pull_request ]; then
  run_all
fi

names=()
while IFS= read -r file; do
  if [[ $file =~ $resource_file ]]; then
    test_file="internal/provider/${BASH_REMATCH[1]}_test.go"
    # A deleted resource has no test file left to select from.
    [ -f "$test_file" ] || continue
    while IFS= read -r name; do
      names+=("$name")
    done < <(sed -nE 's/^func (TestAcc[A-Za-z0-9_]*)\(.*/\1/p' "$test_file")
  else
    run_all
  fi
done <<< "$relevant"

if [ ${#names[@]} -eq 0 ]; then
  emit false TestAcc
  exit 0
fi

unique=$(printf '%s\n' "${names[@]}" | sort -u | paste -sd'|' -)
emit true "^(${unique})\$"
