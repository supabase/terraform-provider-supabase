#!/usr/bin/env bash
# Writes the acceptance test command and results to the job summary.
#
# The summary needs no token, so it also works for pull requests from forks.
#
# Inputs (environment):
#   COMMAND_FILE  File holding the command that was run.
#   LOG_FILE      File holding the combined test output.
#   OUTCOME       Outcome of the test step: success, failure, cancelled or skipped.
#
# Output is appended to $GITHUB_STEP_SUMMARY, or printed when it is unset.
set -euo pipefail

output="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
# GitHub limits a step summary to 1 MiB, so leave room for the rest of the body.
max_bytes=900000

if [ ! -f "$LOG_FILE" ] || [ ! -f "$COMMAND_FILE" ]; then
  echo "No test output was produced, so there is no summary to write."
  exit 0
fi

case "${OUTCOME:-}" in
  success) heading='✅ Acceptance tests passed' ;;
  failure) heading='❌ Acceptance tests failed' ;;
  cancelled) heading='⚪ Acceptance tests were cancelled' ;;
  *) heading='⚪ Acceptance tests did not run' ;;
esac

# At least six backticks, and always one more than the longest run in the output,
# so nothing inside the test output can end the block early.
longest=$(grep -o '`\+' "$LOG_FILE" | awk '{ if (length($0) > m) m = length($0) } END { print m + 0 }' || true)
fence_len=$(( longest >= 6 ? longest + 1 : 6 ))
fence=$(printf '`%.0s' $(seq "$fence_len"))

{
  echo "## $heading"
  echo
  echo "$fence"
  echo "\$ $(cat "$COMMAND_FILE")"
  echo
  if [ "$(wc -c < "$LOG_FILE")" -gt "$max_bytes" ]; then
    echo "[output truncated, showing the last part]"
    # Drop the first line, which the cut may have split.
    tail -c "$max_bytes" "$LOG_FILE" | sed 1d
  else
    cat "$LOG_FILE"
  fi
  echo "$fence"
} >> "$output"
