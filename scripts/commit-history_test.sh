#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Changkun Ou
# SPDX-License-Identifier: Apache-2.0
#
# Exercises commit-history.sh against a local bare repository: a clean push,
# a push that needs a rebase, and a conflict on the same file.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.com GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.com

git init -q --bare -b history "$tmp/remote.git"
seed() { git clone -q "$tmp/remote.git" "$1" 2>/dev/null; git -C "$1" checkout -q -B history; }

seed "$tmp/seed"
echo '{"runs":[]}' >"$tmp/seed/day.json"
git -C "$tmp/seed" add -A && git -C "$tmp/seed" commit -q -m seed && git -C "$tmp/seed" push -q origin history

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1. Clean push.
seed "$tmp/a"
"$here/commit-history.sh" "$tmp/a" sh -c 'echo a >>day.json'
[ "$(git -C "$tmp/remote.git" rev-list --count history)" = 2 ] || fail "clean push"

# 2. Another run pushed a different file first: rebase, then push.
seed "$tmp/b"
seed "$tmp/c"
echo c >"$tmp/c/c.txt" && git -C "$tmp/c" add -A && git -C "$tmp/c" commit -q -m c && git -C "$tmp/c" push -q origin history
"$here/commit-history.sh" "$tmp/b" sh -c 'echo b >b.txt'
git -C "$tmp/remote.git" show history:b.txt >/dev/null || fail "rebase push lost b.txt"
git -C "$tmp/remote.git" show history:c.txt >/dev/null || fail "rebase push lost c.txt"

# 3. Another run changed the same file: the rebase conflicts, so the command
# runs again on top of the remote and both lines survive.
seed "$tmp/d"
seed "$tmp/e"
echo e >>"$tmp/e/day.json" && git -C "$tmp/e" add -A && git -C "$tmp/e" commit -q -m e && git -C "$tmp/e" push -q origin history
"$here/commit-history.sh" "$tmp/d" sh -c 'echo d >>day.json'
git -C "$tmp/remote.git" show history:day.json | grep -qx e || fail "conflict path lost the other run's line"
git -C "$tmp/remote.git" show history:day.json | grep -qx d || fail "conflict path lost this run's line"

# 4. Nothing changed: no commit.
before=$(git -C "$tmp/remote.git" rev-parse history)
seed "$tmp/f"
"$here/commit-history.sh" "$tmp/f" true
[ "$(git -C "$tmp/remote.git" rev-parse history)" = "$before" ] || fail "empty commit pushed"

echo "ok"
