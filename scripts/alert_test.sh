#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Changkun Ou
# SPDX-License-Identifier: Apache-2.0
#
# Exercises alert.sh with a fake gh that records its calls: an outage opens
# an issue, a recovery closes it, and nothing changes while it lasts.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

(cd "$here/.." && go build -o "$tmp/status" ./cmd/status)
mkdir "$tmp/bin" "$tmp/history"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
echo "$*" >>"$GH_LOG"
case "$1 $2" in
"issue list") cat "$GH_OPEN" ;;
"issue create") cat "${@: -1}" >"$GH_LOG.body" ;;
esac
GH
chmod +x "$tmp/bin/gh"
export PATH="$tmp/bin:$PATH" GH_LOG="$tmp/gh.log" GH_OPEN="$tmp/open.json"

day() { # day STATE...: one run a minute ago per state, for website
  local runs="" i=0 ok
  for ok in "$@"; do
    runs+="${runs:+,}{\"time\":\"$(date -u -v-$((10 - i))M +%Y-%m-%dT%H:%M:00Z 2>/dev/null || date -u -d "-$((10 - i)) min" +%Y-%m-%dT%H:%M:00Z)\",\"results\":[{\"service\":\"website\",\"ok\":$ok,\"code\":502,\"ms\":90}]}"
    i=$((i + 1))
  done
  echo "{\"date\":\"$(date -u +%Y-%m-%d)\",\"runs\":[$runs]}" >"$tmp/history/$(date -u +%Y-%m-%d).json"
}

# 1. Down twice, no issue yet: one issue opens, with the marker.
day false false
echo '[]' >"$GH_OPEN"
: >"$GH_LOG"
"$here/alert.sh" "$tmp/status" "$tmp/history" >/dev/null
grep -q '^issue create --label outage --title changkun.de: Website is down' "$GH_LOG" || fail "no issue opened: $(cat "$GH_LOG")"
grep -q '<!-- status:component=website -->' "$GH_LOG.body" || fail "issue body lacks the marker"

# 2. Still down with the issue open: nothing changes.
printf '%s\n' '[{"number":12,"body":"x\n\n<!-- status:component=website -->"}]' >"$GH_OPEN"
: >"$GH_LOG"
"$here/alert.sh" "$tmp/status" "$tmp/history" >/dev/null
! grep -q '^issue \(create\|close\)' "$GH_LOG" || fail "changed issues during the outage: $(cat "$GH_LOG")"

# 3. Recovered: the issue closes with a comment.
day false false true
: >"$GH_LOG"
"$here/alert.sh" "$tmp/status" "$tmp/history" >/dev/null
grep -q '^issue close 12 --comment Checks pass again' "$GH_LOG" || fail "issue not closed: $(cat "$GH_LOG")"

echo "ok"
