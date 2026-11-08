#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Changkun Ou
# SPDX-License-Identifier: Apache-2.0
#
# Usage: alert.sh STATUS HISTORY_DIR
#
# Makes the open issues labelled "outage" match the components that are down
# now: STATUS alerts decides, from HISTORY_DIR and the open issues, which to
# open and which to close, and this script applies that with gh. Needs
# GH_TOKEN with issues: write.
#
# ALERT_MENTION, when set, is a GitHub login each new issue mentions. GitHub
# notifies a mentioned user, and subscribes them to the issue, whether or
# not they watch the repository, so the closing comment reaches them too.
set -euo pipefail

status=$1
history=$2
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

gh label create outage --color B60205 --description "A component is down" --force >/dev/null
gh issue list --label outage --state open --limit 100 --json number,body >"$tmp/open.json"
"$status" alerts -history "$history" -open "$tmp/open.json" >"$tmp/actions.json"

n=$(jq length "$tmp/actions.json")
for ((i = 0; i < n; i++)); do
  jq ".[$i]" "$tmp/actions.json" >"$tmp/action.json"
  jq -r .body "$tmp/action.json" >"$tmp/body.md"
  case $(jq -r .action "$tmp/action.json") in
  open)
    if [[ -n ${ALERT_MENTION:-} ]]; then
      printf '\ncc @%s\n' "$ALERT_MENTION" >>"$tmp/body.md"
    fi
    gh issue create --label outage --title "$(jq -r .title "$tmp/action.json")" --body-file "$tmp/body.md"
    ;;
  close)
    gh issue close "$(jq -r .issue "$tmp/action.json")" --comment "$(cat "$tmp/body.md")"
    ;;
  esac
done
echo "$n alert change(s)"
