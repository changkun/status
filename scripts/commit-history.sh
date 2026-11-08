#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Changkun Ou
# SPDX-License-Identifier: Apache-2.0
#
# Usage: commit-history.sh DIR COMMAND [ARG...]
#
# DIR is a checkout of the history branch. Runs COMMAND (the probe, which
# writes into DIR), commits what changed, and pushes. If another run pushed
# first, it rebases onto that push and tries again. When the rebase itself
# conflicts, which happens when two runs edited the same day file, it
# discards its local commit, takes the remote branch as it is and runs
# COMMAND again. It never force-pushes, so no one's history is overwritten.
set -euo pipefail

dir=$1
shift
branch=${HISTORY_BRANCH:-history}
attempts=${HISTORY_ATTEMPTS:-5}

cd "$dir"
for ((i = 1; i <= attempts; i++)); do
  "$@"
  git add -A
  if git diff --cached --quiet; then
    echo "nothing to commit"
    exit 0
  fi
  git commit -q -m "history: $(date -u +%Y-%m-%dT%H:%MZ)"
  if git push -q origin "HEAD:refs/heads/$branch"; then
    exit 0
  fi
  echo "push rejected (attempt $i of $attempts)" >&2
  git fetch -q origin "$branch"
  if git rebase -q "origin/$branch" && git push -q origin "HEAD:refs/heads/$branch"; then
    exit 0
  fi
  git rebase --abort 2>/dev/null || true
  git reset -q --hard "origin/$branch"
done
echo "could not push history after $attempts attempts" >&2
exit 1
